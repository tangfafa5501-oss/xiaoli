package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildStatusAndPage(t *testing.T) {
	w := httptest.NewRecorder()
	handleStatus(w, httptest.NewRequest("GET", "/api/status", nil))
	var status map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	build, ok := status["build"].(string)
	if !ok || build == "" || status["version"] == nil {
		t.Fatal(status)
	}
	if serverBuild() != build {
		t.Fatal("unstable build")
	}
	for _, path := range []string{"/", "/index.html"} {
		w = httptest.NewRecorder()
		webHandler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-cache" ||
			!strings.Contains(w.Body.String(), `<meta name="xiaoli-build" content="`+build+`">`) || strings.Contains(w.Body.String(), "__XIAOLI_BUILD__") {
			t.Fatal(w.Code, w.Header())
		}
	}
}
