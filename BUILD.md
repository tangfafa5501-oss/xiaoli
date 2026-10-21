# 小离 MVP v0.1 build notes
- launcher/: Go launcher (UI embedded via go:embed, persona.txt default). Build: cd launcher && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H windowsgui -s -w" -o ../xiaoli.exe .
- Icon/manifest: ~/go/bin/go-winres simply --icon ../icon.png --manifest gui (rsrc_windows_amd64.syso)
- stage/小离 = package layout; dist/XiaoLi-v0.1-Windows-x64.zip = output
- dl/: llama.cpp b11799 (win-cpu-x64, ubuntu-x64), unsloth Qwen3-1.7B-Q4_K_M.gguf, VC++ runtime DLLs from vc_redist.x64.exe
- Linux test: lintest/ (./xiaoli) + e2e.py with playwright (.venv)
