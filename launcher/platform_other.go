//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const serverExe = "llama-server"

func userDataDir() string          { return filepath.Join(os.TempDir(), "xiaoli") }
func hideWindow(cmd *exec.Cmd)     {}
func attachToJob(cmd *exec.Cmd)    {}
func otherInstancePort() (int, bool) { return 0, false }
func openWindow(url string)        { fmt.Println("OPEN", url) }
func showError(msg string)         { fmt.Fprintln(os.Stderr, msg) }
