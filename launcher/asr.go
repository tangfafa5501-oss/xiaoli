package main

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"
)

var asrStop = make(chan struct{})
var asrDone = make(chan struct{})
var asrReady atomic.Bool
var asrSlot = make(chan struct{}, 1)
var asrIn io.WriteCloser
var asrOut *bufio.Reader
var asrCmd *exec.Cmd

// One persistent worker, no CGO in launcher; a failed worker never blocks chat.
func startASR() {
	defer close(asrDone)
	defer asrFinished.Store(true)
	defer asrReady.Store(false)
	name := "xiaoli-asr"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	cmd := exec.Command(filepath.Join(appDir, "runtime", "tts", name))
	cmd.Dir = filepath.Join(appDir, "models", "asr")
	lf, err := os.Create(filepath.Join(dataDir, "asr.log"))
	if err != nil {
		log.Println("ASR log:", err)
		return
	}
	defer lf.Close()
	cmd.Stderr = lf
	out, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	hideWindow(cmd)
	if err = cmd.Start(); err != nil {
		log.Println("ASR unavailable:", err)
		return
	}
	attachToJob(cmd)
	asrCmd = cmd
	defer cmd.Process.Kill()
	exited := make(chan struct{})
	go func() { err := cmd.Wait(); asrReady.Store(false); log.Println("ASR exited:", err); close(exited) }()
	reader := bufio.NewReader(out)
	hello := make(chan bool, 1)
	go func() { s, e := reader.ReadString('\n'); hello <- e == nil && s == "READY\n" }()
	select {
	case ok := <-hello:
		if !ok {
			log.Println("ASR invalid handshake")
			return
		}
	case <-asrStop:
		return
	case <-time.After(90 * time.Second):
		log.Println("ASR startup timeout")
		return
	}
	asrIn = in
	asrOut = reader
	asrReady.Store(true)
	log.Println("ASR ready (SenseVoice int8 + Silero, CPU, 4 threads)")
	select {
	case <-exited:
	case <-asrStop:
	}
}

var asrFinished atomic.Bool
var asrSession string
var asrLast time.Time

// Slot owns both the pipe and session state. Never reuse a failed/timed-out pipe.
func handleASR(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
		http.Error(w, "origin", 403)
		return
	}
	var b struct {
		Action  string    `json:"action"`
		Session string    `json:"session"`
		Samples []float32 `json:"samples"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 131072)).Decode(&b); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if b.Action != "start" && b.Action != "chunk" && b.Action != "cancel" {
		http.Error(w, "action", 400)
		return
	}
	if b.Action == "chunk" && (len(b.Samples) == 0 || len(b.Samples) > 4096 || len(b.Samples)%512 != 0) {
		http.Error(w, "samples", 400)
		return
	}
	for _, v := range b.Samples {
		if v < -1 || v > 1 {
			http.Error(w, "samples", 400)
			return
		}
	}
	if !asrReady.Load() {
		http.Error(w, "ASR unavailable", 503)
		return
	}
	select {
	case asrSlot <- struct{}{}:
		defer func() { <-asrSlot }()
	default:
		http.Error(w, "ASR busy", 429)
		return
	}
	if !asrReady.Load() {
		http.Error(w, "ASR unavailable", 503)
		return
	}
	if b.Action == "start" {
		if asrSession != "" && time.Since(asrLast) < 35*time.Second {
			http.Error(w, "microphone busy", 409)
			return
		}
		var token [16]byte
		if _, err := rand.Read(token[:]); err != nil {
			http.Error(w, "session", 500)
			return
		}
		asrSession = hex.EncodeToString(token[:])
	} else if asrSession == "" || b.Session != asrSession {
		http.Error(w, "stale session", 409)
		return
	}
	asrLast = time.Now()
	samples := b.Samples
	if b.Action != "chunk" {
		samples = nil
	}
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		err := binary.Write(asrIn, binary.LittleEndian, uint32(len(samples)))
		if err == nil && len(samples) > 0 {
			err = binary.Write(asrIn, binary.LittleEndian, samples)
		}
		var n uint32
		if err == nil {
			err = binary.Read(asrOut, binary.LittleEndian, &n)
		}
		if err == nil && (n == 0 || n > 16384) {
			err = fmt.Errorf("invalid ASR length %d", n)
		}
		var data []byte
		if err == nil {
			data = make([]byte, n)
			_, err = io.ReadFull(asrOut, data)
		}
		done <- result{data, err}
	}()
	fail := func() {
		asrReady.Store(false)
		if asrCmd != nil {
			asrCmd.Process.Kill()
		}
		http.Error(w, "ASR failed", 503)
	}
	select {
	case v := <-done:
		if v.err != nil {
			log.Println("ASR pipe:", v.err)
			fail()
			return
		}
		var out map[string]any
		if json.Unmarshal(v.data, &out) != nil {
			fail()
			return
		}
		out["session"] = asrSession
		if b.Action == "cancel" || out["done"] == true {
			asrSession = ""
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	case <-time.After(10 * time.Second):
		fail()
	}
}
