package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Real HTTP handler and binary pipe, without model files or network services.
func fakeASR(t *testing.T, respond func([6]uint32) []byte) {
	t.Helper()
	input, writer := io.Pipe()
	output, reader := io.Pipe()
	asrIn = writer
	asrOut = bufio.NewReader(output)
	asrCmd = nil
	asrCurrent.Store(nil)
	asrReady.Store(true)
	t.Cleanup(func() {
		writer.Close()
		input.Close()
		reader.Close()
		output.Close()
		asrReady.Store(false)
		asrCurrent.Store(nil)
	})
	go func() {
		for {
			var h [6]uint32
			if binary.Read(input, binary.LittleEndian, &h) != nil {
				return
			}
			if h[0] != 0x35524c58 || h[1] != 1 || h[2] < 1 || h[2] > 4 || h[3] == 0 {
				t.Errorf("bad header %v", h)
				return
			}
			pcm := make([]float32, h[4])
			if binary.Read(input, binary.LittleEndian, pcm) != nil {
				return
			}
			data := respond(h)
			if binary.Write(reader, binary.LittleEndian, uint32(len(data))) != nil {
				return
			}
			if _, err := reader.Write(data); err != nil {
				return
			}
		}
	}()
}
func asrHTTP(body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "http://localhost/api/asr", strings.NewReader(body))
	handleASR(w, r)
	return w
}
func startFake(t *testing.T) string {
	t.Helper()
	w := asrHTTP(`{"action":"start"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	return out["session"].(string)
}
func fakeReply(h [6]uint32) []byte {
	kind := "ack"
	if h[2] == 3 {
		kind = "final"
	}
	b, _ := json.Marshal(map[string]any{"kind": kind, "session": h[3], "revision": h[2], "text": "完整文字"})
	return b
}
func TestASRCommands(t *testing.T) {
	var commands []uint32
	fakeASR(t, func(h [6]uint32) []byte { commands = append(commands, h[2]); return fakeReply(h) })
	session := startFake(t)
	samples := make([]float32, 512)
	pcm, _ := json.Marshal(samples)
	for _, action := range []string{"chunk", "finish"} {
		body := map[string]any{"action": action, "session": session}
		if action == "chunk" {
			body["samples"] = json.RawMessage(pcm)
			body["validSamples"] = 17
		}
		b, _ := json.Marshal(body)
		w := asrHTTP(string(b))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := asrHTTP(`{"action":"chunk","session":"` + session + `","samples":` + string(pcm) + `,"validSamples":512}`); w.Code != 409 {
		t.Fatal(w.Code)
	}
	session = startFake(t)
	if w := asrHTTP(`{"action":"cancel","session":"` + session + `"}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(commands) != 5 || commands[0] != 1 || commands[1] != 2 || commands[2] != 3 || commands[3] != 1 || commands[4] != 4 {
		t.Fatal(commands)
	}
}
func TestASRCancelDiscardsLateFinal(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	fakeASR(t, func(h [6]uint32) []byte {
		if h[2] == 3 {
			close(entered)
			<-release
		}
		return fakeReply(h)
	})
	session := startFake(t)
	finished := make(chan *httptest.ResponseRecorder, 1)
	cancelled := make(chan *httptest.ResponseRecorder, 1)
	go func() { finished <- asrHTTP(`{"action":"finish","session":"` + session + `"}`) }()
	<-entered
	go func() { cancelled <- asrHTTP(`{"action":"cancel","session":"` + session + `"}`) }()
	deadline := time.After(time.Second)
	for !asrCurrent.Load().cancelled.Load() {
		select {
		case <-deadline:
			t.Fatal("cancel not invalidated")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(release)
	if w := <-finished; w.Code != 409 {
		t.Fatal("late final escaped", w.Code, w.Body.String())
	}
	if w := <-cancelled; w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if asrCurrent.Load() != nil {
		t.Fatal("session survived cancel")
	}
}
func TestASRBadRequestsAndFrames(t *testing.T) {
	fakeASR(t, fakeReply)
	for _, body := range []string{`{"action":"reset"}`, `{"action":"chunk","samples":[0],"validSamples":1}`, `{"action":"finish","samples":[0]}`, `{"action":"chunk","samples":[],"validSamples":-1}`, `{"action":"start","samples":[2]}`} {
		if w := asrHTTP(body); w.Code != 400 {
			t.Fatal(body, w.Code)
		}
	}
	r := httptest.NewRequest("POST", "http://localhost/api/asr", strings.NewReader(`{"action":"start"}`))
	r.Header.Set("Origin", "http://evil.invalid")
	w := httptest.NewRecorder()
	handleASR(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestASRBadWorkerFrame(t *testing.T) {
	for _, payload := range []string{"", `{`, `{"kind":"ack","session":999,"revision":1}`, `{"kind":"bogus","session":1,"revision":1}`, `{"kind":"ack","session":1,"revision":"bad"}`, `{"kind":"ack","session":1,"revision":0.5}`} {
		t.Run(payload, func(t *testing.T) {
			fakeASR(t, func(h [6]uint32) []byte { return []byte(payload) })
			w := asrHTTP(`{"action":"start"}`)
			if w.Code != 503 || asrReady.Load() {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}

func TestASRPartialRevisionAndFallback(t *testing.T) {
	for _, partial := range []bool{true, false} {
		t.Run(map[bool]string{true: "partial", false: "missing-model-final-only"}[partial], func(t *testing.T) {
			var revision uint32
			fakeASR(t, func(h [6]uint32) []byte {
				revision++
				kind := "ack"
				if h[2] == 2 && partial {
					kind = "partial"
				}
				if h[2] == 3 {
					kind = "final"
				}
				data, _ := json.Marshal(map[string]any{"kind": kind, "session": h[3], "revision": revision, "text": "中英 Excel"})
				return data
			})
			session := startFake(t)
			pcm, _ := json.Marshal(make([]float32, 512))
			for i := 0; i < 3; i++ {
				w := asrHTTP(`{"action":"chunk","session":"` + session + `","samples":` + string(pcm) + `,"validSamples":512}`)
				var out map[string]any
				json.Unmarshal(w.Body.Bytes(), &out)
				expected := "ack"
				if partial {
					expected = "partial"
				}
				if w.Code != 200 || out["kind"] != expected || out["revision"] != float64(i+2) {
					t.Fatal(w.Code, w.Body.String())
				}
			}
			w := asrHTTP(`{"action":"finish","session":"` + session + `"}`)
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"kind":"final"`) {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}
func TestASRCancelDiscardsLatePartial(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	fakeASR(t, func(h [6]uint32) []byte {
		if h[2] != 2 {
			return fakeReply(h)
		}
		close(entered)
		<-release
		data, _ := json.Marshal(map[string]any{"kind": "partial", "session": h[3], "revision": 2, "text": "迟到"})
		return data
	})
	session := startFake(t)
	pcm, _ := json.Marshal(make([]float32, 512))
	result := make(chan *httptest.ResponseRecorder, 1)
	cancel := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		result <- asrHTTP(`{"action":"chunk","session":"` + session + `","samples":` + string(pcm) + `,"validSamples":512}`)
	}()
	<-entered
	go func() { cancel <- asrHTTP(`{"action":"cancel","session":"` + session + `"}`) }()
	deadline := time.Now().Add(time.Second)
	for !asrCurrent.Load().cancelled.Load() {
		if time.Now().After(deadline) {
			t.Fatal("cancel timeout")
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	if w := <-result; w.Code != 409 || strings.Contains(w.Body.String(), "迟到") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := <-cancel; w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestASRRejectsRegressingRevision(t *testing.T) {
	fakeASR(t, func(h [6]uint32) []byte {
		data, _ := json.Marshal(map[string]any{"kind": "ack", "session": h[3], "revision": 1})
		return data
	})
	session := startFake(t)
	if w := asrHTTP(`{"action":"cancel","session":"` + session + `"}`); w.Code != 503 || asrReady.Load() {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestASRVoiceActivityBackwardCompatible(t *testing.T) {
	fakeASR(t, func(h [6]uint32) []byte {
		var out map[string]any
		json.Unmarshal(fakeReply(h), &out)
		out["voiceActive"] = true
		data, _ := json.Marshal(out)
		return data
	})
	session := startFake(t)
	pcm, _ := json.Marshal(make([]float32, 512))
	w := asrHTTP(`{"action":"chunk","session":"` + session + `","samples":` + string(pcm) + `,"validSamples":512}`)
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || out["voiceActive"] != true || out["session"] != session {
		t.Fatal(w.Code, w.Body.String())
	}
}
