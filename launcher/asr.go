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
	"sync"
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
	sanitized bool
}

var asrCurrent atomic.Pointer[asrRecording]
var asrSerial uint32
var asrLast time.Time

// Bound memory and worker latency per request; JSON float32 values fit in 1 MiB.
const asrMaxSamples = 16384
const asrMaxBody = 1 << 20

// A bounded history makes cleanup retries harmless, including after validation
// release. Independent locking also permits cancellation while the slot is busy.
var asrReleased struct {
	sync.Mutex
	tokens []string
}

func rememberASR(token string) {
	if token == "" {
		return
	}
	asrReleased.Lock()
	defer asrReleased.Unlock()
	asrReleased.tokens = append(asrReleased.tokens, token)
	if len(asrReleased.tokens) > 32 {
		asrReleased.tokens = asrReleased.tokens[len(asrReleased.tokens)-32:]
	}
}
func releasedASR(token string) bool {
	asrReleased.Lock()
	defer asrReleased.Unlock()
	for _, t := range asrReleased.tokens {
		if token != "" && t == token {
			return true
		}
	}
	return false
}

type asrSampleStats struct {
	min, max              float32
	nonfinite, outOfRange int
}

func sanitizeASR(samples []float32) (s asrSampleStats) {
	haveFinite := false
	for i, v := range samples {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			s.nonfinite++
			samples[i] = 0
			continue
		}
		if !haveFinite || v < s.min {
			s.min = v
		}
		if !haveFinite || v > s.max {
			s.max = v
		}
		haveFinite = true
		if v < -1 {
			samples[i] = -1
			s.outOfRange++
		} else if v > 1 {
			samples[i] = 1
			s.outOfRange++
		}
	}
	return
}

// Slot owns both the pipe and session state. Never reuse a failed/timed-out pipe.
func handleASR(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	responseSession := ""
	action := ""
	validation := false
	n, validSamples, bodyBytes := 0, 0, 0
	stats := asrSampleStats{}
	errorReply := func(message string, status int) {
		if status >= 400 && status < 500 {
			log.Printf("ASR request action=%q status=%d reason=%q n=%d valid=%d bytes=%d min=%g max=%g nonfinite=%d outOfRange=%d", action, status, message, n, validSamples, bodyBytes, stats.min, stats.max, stats.nonfinite, stats.outOfRange)
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
	raw, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, asrMaxBody))
	bodyBytes = len(raw)
	if readErr != nil || !json.Valid(raw) {
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
	decodeErr := json.Unmarshal(raw, &b)
	n = len(b.Samples)
	stats = sanitizeASR(b.Samples)
	if b.ValidSamples != nil {
		validSamples = *b.ValidSamples
	}
	if envelope.Action == "chunk" && !hasValidSamples {
		errorReply("client outdated; reload page", 426)
		return
	}
	if decodeErr != nil {
		errorReply("invalid JSON", 400)
		return
	}
	responseSession = b.Session
	if b.Action != "start" && b.Action != "chunk" && b.Action != "cancel" && b.Action != "finish" {
		errorReply("action", 400)
		return
	}
	if b.Action == "chunk" {
		if n == 0 || n > asrMaxSamples {
			errorReply("samples", 400)
			return
		}
		// null/nonpositive means all supplied samples; over-large counts stop at n.
		// A missing field remains a protocol-version error (426) above.
		if validSamples <= 0 || validSamples > n {
			validSamples = n
		}
	} else if n != 0 || validSamples != 0 {
		errorReply("control payload", 400)
		return
	}
	validation = false
	ackCancel := func() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"kind": "ack", "session": b.Session, "revision": 0})
	}
	if b.Action == "cancel" && releasedASR(b.Session) {
		ackCancel()
		return
	}
	// Token validity is independent of worker availability for cleanup requests.
	if b.Action == "cancel" {
		current := asrCurrent.Load()
		if current == nil || current.token != b.Session {
			if releasedASR(b.Session) {
				ackCancel()
				return
			}
			errorReply("stale session", 409)
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
			if releasedASR(b.Session) {
				ackCancel()
				return
			}
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
	if b.Action == "cancel" && releasedASR(b.Session) {
		ackCancel()
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
		if old := asrCurrent.Load(); old != nil {
			rememberASR(old.token)
		}
		asrCurrent.Store(&asrRecording{token: hex.EncodeToString(token[:]), id: asrSerial})
	} else if current := asrCurrent.Load(); current == nil || b.Session != current.token || (current.cancelled.Load() && b.Action != "cancel") {
		errorReply("stale session", 409)
		return
	}
	current := asrCurrent.Load()
	responseSession = current.token
	asrLast = time.Now()
	if b.Action == "chunk" && !current.sanitized && (stats.outOfRange > 0 || stats.nonfinite > 0) {
		current.sanitized = true
		log.Printf("ASR chunk sanitized session=%s clamped=%d nonfinite=%d min=%g max=%g", current.token, stats.outOfRange, stats.nonfinite, stats.min, stats.max)
	}
	samples := b.Samples
	if b.Action != "chunk" {
		samples = nil
	}
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	// One HTTP exchange may contain several worker frames. Consume and validate
	// every reply, then expose the last ack/partial and its worker revision.
	revision := current.revision
	go func() {
		var data []byte
		var err error
		command := map[string]uint32{"start": 1, "chunk": 2, "finish": 3, "cancel": 4}[b.Action]
		remaining := validSamples
		for offset := 0; ; {
			count := remaining
			if count > 4096 {
				count = 4096
			}
			var pcm []float32
			if b.Action == "chunk" {
				pcm = make([]float32, (count+511)/512*512)
				copy(pcm, samples[offset:offset+count])
			}
			var frame bytes.Buffer
			for _, word := range []uint32{0x35524c58, 1, command, current.id, uint32(len(pcm)), uint32(count)} {
				_ = binary.Write(&frame, binary.LittleEndian, word)
			}
			_ = binary.Write(&frame, binary.LittleEndian, pcm)
			_, err = asrIn.Write(frame.Bytes())
			var size uint32
			if err == nil {
				err = binary.Read(asrOut, binary.LittleEndian, &size)
			}
			if err == nil && (size == 0 || size > 16384) {
				err = fmt.Errorf("invalid ASR length %d", size)
			}
			if err == nil {
				data = make([]byte, size)
				_, err = io.ReadFull(asrOut, data)
			}
			if err == nil {
				var out map[string]any
				if json.Unmarshal(data, &out) != nil {
					err = fmt.Errorf("invalid ASR JSON")
				} else {
					rev, ok := out["revision"].(float64)
					kind := out["kind"]
					if out["session"] != float64(current.id) || !ok || rev <= float64(revision) || rev > math.MaxUint32 || rev != math.Trunc(rev) ||
						(b.Action == "finish" && kind != "final") || (b.Action != "finish" && kind != "ack" && kind != "partial") {
						err = fmt.Errorf("invalid ASR reply")
					} else {
						revision = uint32(rev)
					}
				}
			}
			if err != nil || b.Action != "chunk" {
				break
			}
			offset += count
			remaining -= count
			if remaining == 0 {
				break
			}
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
			rememberASR(current.token)
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
	rememberASR(current.token)
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
