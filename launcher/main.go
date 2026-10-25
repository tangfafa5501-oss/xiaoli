// 小离 launcher: serves the chat UI, starts the local brain (llama-server),
// proxies chat requests to it, and shuts everything down when the window closes.
package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed web/*
var webFS embed.FS

//go:embed persona.txt
var defaultPersona string

var (
	ready     atomic.Bool
	failed    atomic.Bool
	failMsg   atomic.Value
	brainPort int
	appDir    string // folder containing the exe
	dataDir   string // per-user writable folder for logs / browser profile

	aliveMu    sync.Mutex
	aliveConns int
	lastSeen   = time.Now()
	everSeen   bool
)

func freePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 18080
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func persona() string {
	if b, err := os.ReadFile(filepath.Join(appDir, "runtime", "persona.txt")); err == nil && len(bytes.TrimSpace(b)) > 0 {
		return string(b)
	}
	return defaultPersona
}

func setFail(msg string) {
	failMsg.Store(msg)
	failed.Store(true)
}

func startBrain() (*exec.Cmd, error) {
	rt := filepath.Join(appDir, "runtime")
	model := filepath.Join(rt, "model.gguf")
	if _, err := os.Stat(model); err != nil {
		return nil, fmt.Errorf("找不到小离的“大脑”文件（runtime\\model.gguf）。请把压缩包完整解压后再打开。")
	}
	brainPort = freePort()
	// Reserve CPU capacity for the two-thread TTS session during generation.
	threads := runtime.NumCPU() - 2
	if threads < 1 {
		threads = 1
	}
	if threads > 6 {
		threads = 6
	}
	log.Printf("CPU budget: logical=%d llama=%d TTS=2", runtime.NumCPU(), threads)
	args := []string{
		"--threads", fmt.Sprint(threads),
		"--threads-batch", fmt.Sprint(threads),
		"-m", model,
		"--host", "127.0.0.1",
		"--port", fmt.Sprint(brainPort),
		"-c", "4096",
		"-np", "1",
		"--reasoning", "off",
		"--chat-template-kwargs", `{"enable_thinking":false}`,
		"--no-webui",
		"--cache-ram", "256",
	}
	cmd := exec.Command(filepath.Join(rt, serverExe), args...)
	cmd.Dir = rt
	logf, _ := os.Create(filepath.Join(dataDir, "brain.log"))
	if logf != nil {
		cmd.Stdout = logf
		cmd.Stderr = logf
	}
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("小离没能醒来：%v", err)
	}
	attachToJob(cmd)
	return cmd, nil
}

func waitReady(cmd *exec.Cmd) {
	exited := make(chan struct{})
	go func() { cmd.Wait(); close(exited) }()
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			if !ready.Load() {
				setFail("小离没能醒来。可能是电脑缺少必要的系统组件，或者内存不够。可以重启电脑后再试一次。")
			} else {
				setFail("小离刚才不小心睡着了，请关掉窗口重新打开。")
			}
			ready.Store(false)
			return
		default:
		}
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", brainPort))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				ready.Store(true)
				log.Println("brain ready on port", brainPort)
				<-exited
				ready.Store(false)
				setFail("小离刚才不小心睡着了，请关掉窗口重新打开。")
				return
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	setFail("小离醒得太慢了，请关掉窗口再试一次。")
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", 405)
		return
	}
	if !ready.Load() {
		http.Error(w, "not ready", 503)
		return
	}
	var in struct {
		Messages []chatMsg `json:"messages"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	// keep only the most recent turns so we stay well inside the context window
	msgs := in.Messages
	if len(msgs) > 20 {
		msgs = msgs[len(msgs)-20:]
	}
	clean := []chatMsg{{Role: "system", Content: persona()}}
	for _, m := range msgs {
		if (m.Role == "user" || m.Role == "assistant") && strings.TrimSpace(m.Content) != "" {
			c := m.Content
			if len([]rune(c)) > 2000 {
				c = string([]rune(c)[:2000])
			}
			clean = append(clean, chatMsg{m.Role, c})
		}
	}
	body, _ := json.Marshal(map[string]any{
		"messages":             clean,
		"stream":               true,
		"temperature":          0.7,
		"top_p":                0.8,
		"top_k":                20,
		"min_p":                0,
		"presence_penalty":     1.0,
		"max_tokens":           320,
		"chat_template_kwargs": map[string]any{"enable_thinking": false},
	})
	req, _ := http.NewRequestWithContext(r.Context(), "POST",
		fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", brainPort), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "brain error", 502)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(resp.StatusCode)
	fl, _ := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			if fl != nil {
				fl.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	msg, _ := failMsg.Load().(string)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ready": ready.Load(), "failed": failed.Load(), "message": msg, "tts_ready": ttsReady.Load(), "asr_ready": asrReady.Load(), "asr_finished": asrFinished.Load()})
}

// /api/alive is a long-lived event stream; while at least one window holds it open we stay running.
func handleAlive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	fl, _ := w.(http.Flusher)
	aliveMu.Lock()
	aliveConns++
	everSeen = true
	aliveMu.Unlock()
	defer func() {
		aliveMu.Lock()
		aliveConns--
		lastSeen = time.Now()
		aliveMu.Unlock()
	}()
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	fmt.Fprint(w, ": hi\n\n")
	if fl != nil {
		fl.Flush()
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-t.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			if fl != nil {
				fl.Flush()
			}
		}
	}
}

// shouldQuit: no window connected for a while after one had connected, or nobody ever connected.
func watchWindows(start time.Time, quit chan<- struct{}) {
	for {
		time.Sleep(2 * time.Second)
		aliveMu.Lock()
		conns, seen, last := aliveConns, everSeen, lastSeen
		aliveMu.Unlock()
		if seen && conns == 0 && time.Since(last) > 20*time.Second {
			log.Println("window closed, quitting")
			close(quit)
			return
		}
		if !seen && time.Since(start) > 3*time.Minute {
			log.Println("no window ever connected, quitting")
			close(quit)
			return
		}
	}
}

func main() {
	exe, _ := os.Executable()
	appDir = filepath.Dir(exe)
	dataDir = userDataDir()
	os.MkdirAll(dataDir, 0o755)
	if lf, err := os.Create(filepath.Join(dataDir, "launcher.log")); err == nil {
		log.SetOutput(io.MultiWriter(lf, os.Stderr))
	}

	// Only one copy at a time: if already running, just bring up its window again.
	if port, running := otherInstancePort(); running {
		if port > 0 {
			openWindow(fmt.Sprintf("http://127.0.0.1:%d/", port))
		}
		return
	}

	// Reuse the UI origin across launches so localStorage mute preferences persist.
	previousPort, _ := os.ReadFile(filepath.Join(dataDir, "ui-port.txt"))
	port, _ := strconv.Atoi(strings.TrimSpace(string(previousPort)))
	if port < 1024 || port > 65535 {
		port = 0
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		showError("小离打不开窗口：" + err.Error())
		return
	}
	uiPort := ln.Addr().(*net.TCPAddr).Port
	os.WriteFile(filepath.Join(dataDir, "ui-port.txt"), []byte(fmt.Sprint(uiPort)), 0o644)
	os.WriteFile(filepath.Join(dataDir, "port.txt"), []byte(fmt.Sprint(uiPort)), 0o644)

	cmd, err := startBrain()
	if err != nil {
		setFail(err.Error())
		log.Println(err)
	} else {
		go waitReady(cmd)
	}

	go startTTS()
	go startASR()

	sub, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/chat", handleChat)
	mux.HandleFunc("/api/tts", handleTTS)
	mux.HandleFunc("/api/asr", handleASR)
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/alive", handleAlive)
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	url := fmt.Sprintf("http://127.0.0.1:%d/", uiPort)
	log.Println("UI at", url)
	openWindow(url)

	quit := make(chan struct{})
	go watchWindows(time.Now(), quit)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	srv.Shutdown(ctx)
	cancel()
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
	}
	close(asrStop)
	select {
	case <-asrDone:
	case <-time.After(2 * time.Second):
	}
	close(ttsStop)
	select {
	case <-ttsDone:
	case <-time.After(2 * time.Second):
	}
	os.Remove(filepath.Join(dataDir, "port.txt"))
}
