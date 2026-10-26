package main

import (
	"crypto/sha256"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"
)

// Set by release ldflags. Local builds identify the executable bytes once.
var buildID string
var buildOnce sync.Once

func serverBuild() string {
	buildOnce.Do(func() {
		if buildID != "" {
			return
		}
		exe, err := os.Executable()
		if err != nil {
			panic(err)
		}
		f, err := os.Open(exe)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			panic(err)
		}
		buildID = fmt.Sprintf("%x", h.Sum(nil)[:12])
	})
	return buildID
}
func webHandler() http.Handler {
	sub, _ := fs.Sub(webFS, "web")
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// FileServer canonicalizes /index.html to /; serve both without a cached shell.
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			b, _ := webFS.ReadFile("web/index.html")
			body := strings.ReplaceAll(string(b), "__XIAOLI_BUILD__", html.EscapeString(serverBuild()))
			if r.Method != "HEAD" {
				io.WriteString(w, body)
			}
			return
		}
		// Scripts must also revalidate across updates on the same origin.
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
}
