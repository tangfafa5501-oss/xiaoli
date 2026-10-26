package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"
)

var clientLogs struct {
	sync.Mutex
	since time.Time
	count int
}
var clientFields = []string{"event", "stage", "name", "message", "status", "action", "muted", "ttsState", "ttsRate", "ttsPlaying", "micRate", "chatMode", "ua"}

func clientPrintable(s string) string {
	r := []rune(strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return ' '
		}
		return r
	}, s))
	if len(r) > 200 {
		r = r[:200]
	}
	return string(r)
}
func handleClientLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
		http.Error(w, "origin", 403)
		return
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	var b map[string]any
	if err := d.Decode(&b); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		http.Error(w, "invalid JSON", 400)
		return
	}
	event, _ := b["event"].(string)
	if event != "mic_failure" && event != "mic_start" {
		http.Error(w, "event", 400)
		return
	}
	clientLogs.Lock()
	if time.Since(clientLogs.since) >= time.Minute {
		clientLogs.since = time.Now()
		clientLogs.count = 0
	}
	if clientLogs.count >= 30 {
		clientLogs.Unlock()
		http.Error(w, "rate limit", 429)
		return
	}
	clientLogs.count++
	clientLogs.Unlock()
	line := "CLIENT " + event
	for _, k := range clientFields[1:] {
		v, ok := b[k]
		if !ok {
			continue
		}
		var s string
		switch x := v.(type) {
		case string:
			s = clientPrintable(x)
		case float64, bool:
			raw, _ := json.Marshal(x)
			s = string(raw)
		default:
			continue
		}
		line += " " + k + "=" + s
	}
	log.Print(line)
	w.WriteHeader(204)
}
