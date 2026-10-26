package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"testing"
)

func audioBody(session string, pcm []float32, valid any) string {
	b, _ := json.Marshal(map[string]any{"action": "chunk", "session": session, "samples": pcm, "validSamples": valid})
	return string(b)
}
func TestASRAudioFraming(t *testing.T) {
	for _, n := range []int{1, 128, 1000, 1024, 4096, 4097, 5000, 16384} {
		for _, valid := range []int{n, 1, n - 1, 0, -1, n + 100} {
			t.Run(fmt.Sprintf("n%d-valid%d", n, valid), func(t *testing.T) {
				real := valid
				if real <= 0 || real > n {
					real = n
				}
				var revision uint32
				var frames int
				var received []float32
				fakeASRFrames(t, func(h [6]uint32, pcm []float32) []byte {
					revision++
					if h[2] == 2 {
						frames++
						if h[4] < 512 || h[4] > 4096 || h[4]%512 != 0 || h[5] < 1 || h[5] > h[4] {
							t.Errorf("bad frame %v", h)
						}
						received = append(received, pcm[:h[5]]...)
						for _, v := range pcm[h[5]:] {
							if v != 0 {
								t.Error("nonzero padding")
							}
						}
					}
					b, _ := json.Marshal(map[string]any{"kind": "partial", "session": h[3], "revision": revision, "text": fmt.Sprint(revision)})
					return b
				})
				session := startFake(t)
				pcm := make([]float32, n)
				for i := range pcm {
					pcm[i] = float32(i%7) / 8
				}
				w := asrHTTP(audioBody(session, pcm, valid))
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
				if len(received) != real || frames != (real+4095)/4096 {
					t.Fatal(len(received), real, frames)
				}
				for i, v := range received {
					if v != pcm[i] {
						t.Fatalf("sample %d: %g != %g", i, v, pcm[i])
					}
				}
				var out map[string]any
				json.Unmarshal(w.Body.Bytes(), &out)
				if out["revision"] != float64(frames+1) || out["text"] != fmt.Sprint(frames+1) {
					t.Fatal(w.Body.String())
				}
			})
		}
	}
}
func TestASRSanitizeAndBodyLimit(t *testing.T) {
	var revision uint32
	var seen []float32
	fakeASRFrames(t, func(h [6]uint32, pcm []float32) []byte {
		revision++
		if h[2] == 2 {
			seen = append(seen, pcm[:h[5]]...)
		}
		b, _ := json.Marshal(map[string]any{"kind": "ack", "session": h[3], "revision": revision})
		return b
	})
	session := startFake(t)
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)
	pcm := []float32{1.0000001, -1.02, 1.3, -1.5, 0, .5}
	for i := 0; i < 2; i++ {
		if w := asrHTTP(audioBody(session, pcm, len(pcm))); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for i, want := range []float32{1, -1, 1, -1, 0, .5} {
		if seen[i] != want {
			t.Fatal(seen)
		}
	}
	if strings.Count(logs.String(), "ASR chunk sanitized") != 1 || !strings.Contains(logs.String(), "clamped=4 nonfinite=0 min=-1.5 max=1.3") {
		t.Fatal(logs.String())
	}
	t.Log(strings.TrimSpace(logs.String()))
	seen = nil
	pcm = make([]float32, 16384)
	for i := range pcm {
		pcm[i] = .12345679
	}
	body := audioBody(session, pcm, 99999)
	if len(body) <= 131072 {
		t.Fatal("body does not exercise old cap", len(body))
	}
	if w := asrHTTP(body); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(seen) != 16384 {
		t.Fatal(len(seen))
	}
	seen = nil
	if w := asrHTTP(`{"action":"chunk","session":"` + session + `","samples":[null,0.5],"validSamples":null}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(seen) != 2 || seen[0] != 0 || seen[1] != .5 {
		t.Fatal(seen)
	}
	body = `{"action":"start","padding":"` + strings.Repeat("x", asrMaxBody) + `"}`
	if w := asrHTTP(body); w.Code != 400 {
		t.Fatal(w.Code)
	}
}
func TestASRNonfiniteSanitizer(t *testing.T) {
	pcm := []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1)), 2, -3, .5}
	stats := sanitizeASR(pcm)
	if stats.nonfinite != 3 || stats.outOfRange != 2 {
		t.Fatal(stats)
	}
	for i, want := range []float32{0, 0, 0, 1, -1, .5} {
		if pcm[i] != want {
			t.Fatal(pcm)
		}
	}
}
func TestASRLimitLogAndIdempotentCancel(t *testing.T) {
	fakeASR(t, fakeReply)
	session := startFake(t)
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)
	body := audioBody(session, make([]float32, 16385), 16385)
	w := asrHTTP(body)
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, part := range []string{`action="chunk" status=400 reason="samples" n=16385 valid=16385`, fmt.Sprintf("bytes=%d", len(body)), "min=0 max=0 nonfinite=0 outOfRange=0"} {
		if !strings.Contains(logs.String(), part) {
			t.Fatal(logs.String())
		}
	}
	t.Log(strings.TrimSpace(logs.String()))
	for i := 0; i < 2; i++ {
		if w := asrHTTP(`{"action":"cancel","session":"` + session + `"}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"kind":"ack"`) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	next := startFake(t)
	if w := asrHTTP(`{"action":"cancel","session":"` + session + `"}`); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if asrCurrent.Load().token != next {
		t.Fatal("old cancel touched current")
	}
	if w := asrHTTP(`{"action":"cancel","session":"unknown"}`); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := asrHTTP(`{"action":"finish","session":"` + next + `"}`); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := asrHTTP(`{"action":"cancel","session":"` + next + `"}`); w.Code != 200 {
		t.Fatal(w.Code)
	}
	asrReady.Store(false)
	if w := asrHTTP(`{"action":"cancel","session":"` + session + `"}`); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := asrHTTP(`{"action":"cancel","session":"unknown"}`); w.Code != 409 {
		t.Fatal(w.Code)
	}

}
func TestASRSplitRejectsIntermediateRevision(t *testing.T) {
	var calls int
	fakeASR(t, func(h [6]uint32) []byte {
		calls++
		rev := calls
		if calls == 3 {
			rev = 2
		}
		b, _ := json.Marshal(map[string]any{"kind": "ack", "session": h[3], "revision": rev})
		return b
	})
	session := startFake(t)
	if w := asrHTTP(audioBody(session, make([]float32, 16384), 16384)); w.Code != 503 || asrReady.Load() || calls != 3 {
		t.Fatal(w.Code, w.Body.String(), calls)
	}
}
