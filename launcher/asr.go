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
var asrPartial atomic.Bool
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
	defer asrPartial.Store(false)
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
	go func() {
		s, e := reader.ReadString('\n')
		asrPartial.Store(e == nil && s == "READY partial=1\n")
		hello <- e == nil && (s == "READY\n" || s == "READY partial=1\n")
	}()
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
	log.Println("ASR ready (SenseVoice int8 + Silero; optional streaming Zipformer, CPU)")
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
	revision  uint32
}

var asrCurrent atomic.Pointer[asrRecording]
var asrSerial uint32
var asrLast time.Time

// Slot owns both the pipe and session state. Never reuse a failed/timed-out pipe.
func handleASR(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	responseSession := ""
	action := ""
	validation := false
	errorReply := func(message string, status int) {
		if status >= 400 && status < 500 {
			log.Printf("ASR request action=%q status=%d reason=%q", action, status, message)
			if validation {
				releaseInvalidASR(responseSession)
			}
		}
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
		ValidSamples *int      `json:"validSamples"`
	}
	// Decode the envelope first so type errors still identify the current session.
	var raw json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 131072)).Decode(&raw); err != nil {
		errorReply("invalid JSON", 400)
		return
	}
	var envelope struct {
		Action  string `json:"action"`
		Session string `json:"session"`
	}
	_ = json.Unmarshal(raw, &envelope)
	action, responseSession = envelope.Action, envelope.Session
	validation = true
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	_, hasValidSamples := fields["validSamples"]
	if envelope.Action == "chunk" && !hasValidSamples {
		errorReply("client outdated; reload page", 426)
		return
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		errorReply("invalid JSON", 400)
		return
	}
	responseSession = b.Session
	validSamples := 0
	if b.ValidSamples != nil {
		validSamples = *b.ValidSamples
	}
	if b.Action != "start" && b.Action != "chunk" && b.Action != "cancel" && b.Action != "finish" {
		errorReply("action", 400)
		return
	}
	if b.Action == "chunk" && (len(b.Samples) == 0 || len(b.Samples) > 4096 || len(b.Samples)%512 != 0) {
		errorReply("samples", 400)
		return
	}
	if b.Action == "chunk" && (validSamples < 1 || validSamples > len(b.Samples)) {
		errorReply("validSamples", 400)
		return
	}
	if b.Action != "chunk" && (len(b.Samples) != 0 || validSamples != 0) {
		errorReply("control payload", 400)
		return
	}
	for _, v := range b.Samples {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || v < -1 || v > 1 {
			errorReply("samples", 400)
			return
		}
	}
	validation = false
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
		for _, word := range []uint32{0x35524c58, 1, command, current.id, uint32(len(samples)), uint32(validSamples)} {
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
		if !validRevision || revision <= float64(current.revision) || revision > math.MaxUint32 || revision != math.Trunc(revision) {
			fail()
			return
		}
		if current.cancelled.Load() && b.Action != "cancel" {
			errorReply("cancelled session", 409)
			return
		}
		current.revision = uint32(revision)
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

// Validation errors cancel only the supplied current token. Slot ownership keeps
// the worker exchange serialized with in-flight chunks/finish and new starts.
func releaseInvalidASR(token string) {
	current := asrCurrent.Load()
	if token == "" || current == nil || current.token != token {
		return
	}
	current.cancelled.Store(true)
	select {
	case asrSlot <- struct{}{}:
	case <-time.After(12 * time.Second):
		asrReady.Store(false)
		asrCurrent.CompareAndSwap(current, nil)
		if asrCmd != nil {
			asrCmd.Process.Kill()
		}
		return
	}
	defer func() { <-asrSlot }()
	if asrCurrent.Load() != current {
		return
	}
	defer asrCurrent.CompareAndSwap(current, nil)
	if !asrReady.Load() {
		return
	}
	done := make(chan error, 1)
	go func() {
		var frame bytes.Buffer
		for _, word := range []uint32{0x35524c58, 1, 4, current.id, 0, 0} {
			_ = binary.Write(&frame, binary.LittleEndian, word)
		}
		_, err := asrIn.Write(frame.Bytes())
		var n uint32
		if err == nil {
			err = binary.Read(asrOut, binary.LittleEndian, &n)
		}
		if err == nil && (n == 0 || n > 16384) {
			err = fmt.Errorf("invalid ASR cancel length %d", n)
		}
		var out struct {
			Kind    string `json:"kind"`
			Session uint32 `json:"session"`
		}
		if err == nil {
			data := make([]byte, n)
			_, err = io.ReadFull(asrOut, data)
			if err == nil {
				err = json.Unmarshal(data, &out)
			}
			if err == nil && (out.Kind != "ack" || out.Session != current.id) {
				err = fmt.Errorf("invalid ASR cancel reply")
			}
		}
		done <- err
	}()
	var err error
	select {
	case err = <-done:
	case <-time.After(10 * time.Second):
		err = fmt.Errorf("ASR cancel timeout")
	}
	if err != nil {
		log.Println("ASR validation cancel:", err)
		asrReady.Store(false)
		if asrCmd != nil {
			asrCmd.Process.Kill()
		}
	}
}
