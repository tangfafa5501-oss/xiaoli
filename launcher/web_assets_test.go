package main

import (
 "bytes"
 "context"
 "io"
 "io/fs"
 "net/http"
 "net/http/httptest"
 "net/url"
 "strconv"
 "strings"
 "testing"
 "time"
)

func TestEmbeddedMotionAssets(t *testing.T) {
 sub, err := fs.Sub(webFS, "web")
 if err != nil {
  t.Fatal(err)
 }
 handler := http.FileServer(http.FS(sub))
 for _, asset := range []string{"/chat-ui.mjs", "/chat-session.mjs", "/avatar.mjs", "/motion/rig.mjs", "/motion/motion.mjs", "/motion/vendor/three.module.min.js", "/motion/vendor/three.core.min.js"} {
  w := httptest.NewRecorder()
  handler.ServeHTTP(w, httptest.NewRequest("GET", asset, nil))
  if w.Code != 200 || w.Body.Len() == 0 { t.Errorf("%s missing: %d", asset, w.Code) }
  if !strings.Contains(w.Header().Get("Content-Type"), "javascript") { t.Errorf("%s wrong module MIME: %s", asset, w.Header().Get("Content-Type")) }
 }
}

// Tests the real existing Go proxy against an explicit local fake brain, not an LLM.
func TestChatCancellationReachesLocalBrain(t *testing.T) {
 reached, cancelled := make(chan struct{}), make(chan struct{})
 brain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  io.Copy(io.Discard, r.Body)
  close(reached)
  select {
  case <-r.Context().Done():
   close(cancelled)
  case <-time.After(5 * time.Second):
  }
 }))
 defer brain.Close()
 u, _ := url.Parse(brain.URL)
 previousPort, previousReady, previousDir := brainPort, ready.Load(), appDir
 brainPort, _ = strconv.Atoi(u.Port())
 ready.Store(true)
 appDir = t.TempDir()
 defer func(){ brainPort=previousPort;ready.Store(previousReady);appDir=previousDir }()
 proxy := httptest.NewServer(http.HandlerFunc(handleChat))
 defer proxy.Close()
 ctx, cancel := context.WithCancel(context.Background())
 defer cancel()
 req, _ := http.NewRequestWithContext(ctx, "POST", proxy.URL, bytes.NewBufferString(`{"messages":[{"role":"user","content":"fixture"}]}`))
 completed := make(chan struct{})
 go func(){ resp, err := http.DefaultClient.Do(req);if err==nil {io.Copy(io.Discard,resp.Body);resp.Body.Close()};close(completed) }()
 select {case <-reached: case <-time.After(3*time.Second): t.Fatal("local brain not reached")}
 cancel()
 select {case <-cancelled: case <-time.After(3*time.Second): t.Fatal("cancellation did not reach brain")}
 select {case <-completed: case <-time.After(3*time.Second): t.Fatal("proxy request remained blocked")}
}
