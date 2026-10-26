package main

import (
	"bytes"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientLog(t *testing.T) {
	var out bytes.Buffer
	old := log.Writer()
	log.SetOutput(&out)
	defer log.SetOutput(old)
	clientLogs.Lock()
	clientLogs.since = time.Now()
	clientLogs.count = 0
	clientLogs.Unlock()
	call := func(body, origin string) int {
		r := httptest.NewRequest("POST", "http://localhost/api/log", strings.NewReader(body))
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handleClientLog(w, r)
		return w.Code
	}
	if got := call(`{"event":"mic_failure"}`, "http://evil"); got != 403 {
		t.Fatal(got)
	}
	if got := call(`{"event":"mic_failure","message":"`+strings.Repeat("x", 4096)+`"}`, ""); got != 400 {
		t.Fatal(got)
	}
	if got := call(`{"event":"mic_failure","stage":"getUserMedia","name":"NotReadableError","message":"hello\nworld\u0000","samples":[1,2]}`, ""); got != 204 {
		t.Fatal(got)
	}
	if !strings.Contains(out.String(), "hello world ") || strings.Contains(out.String(), "samples") {
		t.Fatal(out.String())
	}
	if got := call(`{"event":"mic_failure","stage":"chunk","n":1024,"validSamples":1000,"maxAbs":1.02,"samples":[1]}`, ""); got != 204 {
		t.Fatal(got)
	}
	if !strings.Contains(out.String(), "n=1024 validSamples=1000 maxAbs=1.02") || strings.Contains(out.String(), "samples=") {
		t.Fatal(out.String())
	}
	if len([]rune(clientPrintable(strings.Repeat("中", 300)))) != 200 {
		t.Fatal("truncate")
	}
	for i := 2; i < 30; i++ {
		if call(`{"event":"mic_start"}`, "") != 204 {
			t.Fatal(i)
		}
	}
	if call(`{"event":"mic_start"}`, "") != 429 {
		t.Fatal("limit")
	}
}
