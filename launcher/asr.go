package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
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

type asrRecording struct {
	token     string
	id        uint32
	cancelled atomic.Bool
}

var asrCurrent atomic.Pointer[asrRecording]
var asrSerial uint32
var asrLast time.Time

// Slot owns both the pipe and session state. Never reuse a failed/timed-out pipe.
func handleASR(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	responseSession := ""
	errorReply := func(message string, status int) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]any{"kind": "error", "session": responseSession, "revision": 0, "message": message})
	}
	if r.Method != "POST" {
		errorReply("POST required", 405)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
		errorReply("origin", 403)
		return
	}
	var b struct {
		Action       string    `json:"action"`
		Session      string    `json:"session"`
		Samples      []float32 `json:"samples"`
		ValidSamples int       `json:"validSamples"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 131072)).Decode(&b); err != nil {
		errorReply("invalid JSON", 400)
		return
	}
	responseSession = b.Session
	if b.Action != "start" && b.Action != "chunk" && b.Action != "cancel" && b.Action != "finish" {
		errorReply("action", 400)
		return
	}
	if b.Action == "chunk" && (len(b.Samples) == 0 || len(b.Samples) > 4096 || len(b.Samples)%512 != 0) {
		errorReply("samples", 400)
		return
	}
	if b.Action == "chunk" && (b.ValidSamples < 1 || b.ValidSamples > len(b.Samples)) {
		errorReply("validSamples", 400)
		return
	}
	if b.Action != "chunk" && (len(b.Samples) != 0 || b.ValidSamples != 0) {
		errorReply("control payload", 400)
		return
	}
	for _, v := range b.Samples {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || v < -1 || v > 1 {
			errorReply("samples", 400)
			return
		}
	}
	if !asrReady.Load() {
		errorReply("ASR unavailable", 503)
		return
	}
	// Cancellation invalidates a pending finish before waiting for its pipe owner.
	if b.Action == "cancel" {
		current := asrCurrent.Load()
		if current == nil || current.token != b.Session {
			errorReply("stale session", 409)
			return
		}
		current.cancelled.Store(true)
		select {
		case asrSlot <- struct{}{}:
			defer func() { <-asrSlot }()
		case <-time.After(12 * time.Second):
			errorReply("ASR busy", 429)
			return
		}
	} else {
		select {
		case asrSlot <- struct{}{}:
			defer func() { <-asrSlot }()
		default:
			errorReply("ASR busy", 429)
			return
		}
	}
	if !asrReady.Load() {
		errorReply("ASR unavailable", 503)
		return
	}
	if b.Action == "start" {
		if asrCurrent.Load() != nil && time.Since(asrLast) < 35*time.Second {
			errorReply("microphone busy", 409)
			return
		}
		var token [16]byte
		if _, err := rand.Read(token[:]); err != nil {
			errorReply("session", 500)
			return
		}
		asrSerial++
		if asrSerial == 0 {
			asrSerial++
		}
		asrCurrent.Store(&asrRecording{token: hex.EncodeToString(token[:]), id: asrSerial})
	} else if current := asrCurrent.Load(); current == nil || b.Session != current.token || (current.cancelled.Load() && b.Action != "cancel") {
		errorReply("stale session", 409)
		return
	}
	current := asrCurrent.Load()
	responseSession = current.token
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
		var frame bytes.Buffer
		command := map[string]uint32{"start": 1, "chunk": 2, "finish": 3, "cancel": 4}[b.Action]
		for _, word := range []uint32{0x35524c58, 1, command, current.id, uint32(len(samples)), uint32(b.ValidSamples)} {
			_ = binary.Write(&frame, binary.LittleEndian, word)
		}
		_ = binary.Write(&frame, binary.LittleEndian, samples)
		_, err := asrIn.Write(frame.Bytes())
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
		errorReply("ASR failed", 503)
	}
	select {
	case v := <-done:
		if v.err != nil {
			log.Println("ASR pipe:", v.err)
			fail()
			return
		}
		var out map[string]any
		if json.Unmarshal(v.data, &out) != nil || out["session"] != float64(current.id) || out["revision"] == nil ||
			(out["kind"] != "ack" && out["kind"] != "partial" && out["kind"] != "final" && out["kind"] != "error") ||
			(b.Action == "finish" && out["kind"] != "final") || (b.Action != "finish" && out["kind"] != "ack" && out["kind"] != "partial") {
			fail()
			return
		}
		revision, validRevision := out["revision"].(float64)
		if !validRevision || revision < 1 || revision > math.MaxUint32 || revision != math.Trunc(revision) {
			fail()
			return
		}
		if current.cancelled.Load() && b.Action != "cancel" {
			errorReply("cancelled session", 409)
			return
		}
		out["session"] = current.token
		if b.Action == "cancel" || out["kind"] == "final" {
			asrCurrent.CompareAndSwap(current, nil)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	case <-time.After(10 * time.Second):
		fail()
	}
}
