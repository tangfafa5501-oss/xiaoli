package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
)

var ttsStop = make(chan struct{})
var ttsDone = make(chan struct{})
var ttsReady atomic.Bool
var ttsSlot = make(chan struct{}, 1)
var ttsIn io.WriteCloser
var ttsOut *bufio.Reader
var ttsCmd *exec.Cmd

// One persistent worker, no CGO in launcher; a failed worker never blocks chat.
func startTTS() {
	defer close(ttsDone)
	defer ttsReady.Store(false)
	name := "xiaoli-tts"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	cmd := exec.Command(filepath.Join(appDir, "runtime", "tts", name))
	cmd.Dir = filepath.Join(appDir, "models", "tts")
	lf, err := os.Create(filepath.Join(dataDir, "tts.log"))
	if err != nil {
		log.Println("TTS log:", err)
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
		log.Println("TTS unavailable:", err)
		return
	}
	attachToJob(cmd)
	ttsCmd = cmd
	defer cmd.Process.Kill()
	exited := make(chan struct{})
	go func() { err := cmd.Wait(); ttsReady.Store(false); log.Println("TTS exited:", err); close(exited) }()
	reader := bufio.NewReader(out)
	hello := make(chan bool, 1)
	go func() { s, e := reader.ReadString('\n'); hello <- e == nil && s == "READY\n" }()
	select {
	case ok := <-hello:
		if !ok {
			log.Println("TTS invalid handshake")
			return
		}
	case <-ttsStop:
		return
	case <-time.After(90 * time.Second):
		log.Println("TTS startup timeout")
		return
	}
	ttsIn = in
	ttsOut = reader
	ttsReady.Store(true)
	log.Println("TTS ready (Melo sid=0, CPU, 2 threads)")
	select {
	case <-exited:
	case <-ttsStop:
	}
}

var spokenLinks = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
var spokenTags = regexp.MustCompile("<[^>]*>|https?://[^\\s]+|`{1,3}|[*_#>~～|\\[\\]{}]")

func spokenText(s string) string {
	s = spokenLinks.ReplaceAllString(s, "$1")
	s = spokenTags.ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r) || strings.ContainsRune("。，！？!?；;：:、,.…（）()'-", r) {
			return r
		}
		return -1
	}, s)
	return strings.TrimSpace(s)
}

func handleTTS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	// No cross-origin synthesis or credentials; UI and worker are loopback only.
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
		http.Error(w, "origin", 403)
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	text := spokenText(body.Text)
	if text == "" {
		w.WriteHeader(204)
		return
	}
	if len([]rune(text)) > 240 {
		http.Error(w, "sentence too long", 400)
		return
	}
	if !ttsReady.Load() {
		http.Error(w, "TTS unavailable", 503)
		return
	}
	select {
	case ttsSlot <- struct{}{}:
		defer func() { <-ttsSlot }()
	default:
		http.Error(w, "TTS busy", 429)
		return
	}
	if !ttsReady.Load() {
		http.Error(w, "TTS unavailable", 503)
		return
	}
	started := time.Now()
	type result struct {
		wav []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		b := []byte(text)
		err := binary.Write(ttsIn, binary.LittleEndian, uint32(len(b)))
		if err == nil {
			_, err = ttsIn.Write(b)
		}
		var n uint32
		if err == nil {
			err = binary.Read(ttsOut, binary.LittleEndian, &n)
		}
		if err == nil && (n < 44 || n > 5000000) {
			err = fmt.Errorf("invalid WAV length %d", n)
		}
		var wav []byte
		if err == nil {
			wav = make([]byte, n)
			_, err = io.ReadFull(ttsOut, wav)
		}
		done <- result{wav, err}
	}()
	select {
	case v := <-done:
		if v.err != nil {
			ttsReady.Store(false)
			if ttsCmd != nil {
				ttsCmd.Process.Kill()
			}
			log.Println("TTS failed:", v.err)
			http.Error(w, "TTS failed", 503)
			return
		}
		log.Printf("TTS text=%q elapsed_ms=%d bytes=%d", text, time.Since(started).Milliseconds(), len(v.wav))
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Server-Timing", fmt.Sprintf("synthesis;dur=%d", time.Since(started).Milliseconds()))
		w.Write(v.wav)
	case <-time.After(30 * time.Second):
		// Kill before releasing the slot: never reuse an out-of-sync binary pipe.
		ttsReady.Store(false)
		if ttsCmd != nil {
			ttsCmd.Process.Kill()
		}
		http.Error(w, "TTS timeout", 503)
	}
}
