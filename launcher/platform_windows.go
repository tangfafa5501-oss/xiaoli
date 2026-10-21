//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const serverExe = "llama-server.exe"

var jobHandle windows.Handle
var mutexHandle windows.Handle

func userDataDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "XiaoLi")
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

// attachToJob makes Windows kill the brain automatically if this launcher dies for any reason.
func attachToJob(cmd *exec.Cmd) {
	j, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(j, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(j)
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	if windows.AssignProcessToJobObject(j, h) == nil {
		jobHandle = j
	}
}

func otherInstancePort() (int, bool) {
	name, _ := windows.UTF16PtrFromString("Local\\XiaoLiMVP_SingleInstance")
	h, err := windows.CreateMutex(nil, false, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		b, _ := os.ReadFile(filepath.Join(userDataDir(), "port.txt"))
		p, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		return p, true
	}
	mutexHandle = h
	return 0, false
}

func findBrowser() string {
	var cands []string
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LOCALAPPDATA"} {
		if b := os.Getenv(env); b != "" {
			cands = append(cands, filepath.Join(b, `Microsoft\Edge\Application\msedge.exe`))
		}
	}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		if b := os.Getenv(env); b != "" {
			cands = append(cands, filepath.Join(b, `Google\Chrome\Application\chrome.exe`))
		}
	}
	for _, c := range cands {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func openWindow(url string) {
	if b := findBrowser(); b != "" {
		cmd := exec.Command(b,
			"--app="+url,
			"--user-data-dir="+filepath.Join(userDataDir(), "window"),
			"--window-size=480,860",
			"--no-first-run",
			"--no-default-browser-check",
			"--disable-sync",
			"--disable-extensions",
		)
		if cmd.Start() == nil {
			return
		}
	}
	exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func showError(msg string) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	mb := user32.NewProc("MessageBoxW")
	t, _ := windows.UTF16PtrFromString(msg)
	c, _ := windows.UTF16PtrFromString("小离")
	mb.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x40)
}
