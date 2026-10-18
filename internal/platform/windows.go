//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// Name is a human name for this OS.
const Name = "Windows"

// DefaultListen keeps goli on loopback, so Windows Firewall never asks about
// it. Set listen: ":80" in config.yaml to reach goli from other devices.
const DefaultListen = "127.0.0.1:80"

// No ::1 line: a browser trying go over IPv6 would find nothing on [::1]:80.
var hostsAddrs = []string{"127.0.0.1"}

// HostsPath is the system hosts file.
func HostsPath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "drivers", "etc", "hosts")
}

// HostsInstructions is the command to run in an Administrator PowerShell.
func HostsInstructions(lines []string) string {
	body := strings.ReplaceAll(strings.Join(lines, "`r`n"), "\t", " ")
	return `Add-Content -Path "$env:SystemRoot\System32\drivers\etc\hosts" -Value "` + "`r`n# goli: go/ short links`r`n" + body + `"; ipconfig /flushdns` +
		"\n  (run it in PowerShell opened with \"Run as administrator\")"
}

// OpenURL opens u in the default browser.
func OpenURL(u string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Run()
}

// EditCommand opens path in $EDITOR, or Notepad.
func EditCommand(editor, path string) *exec.Cmd {
	f := strings.Fields(editor)
	if len(f) == 0 {
		return exec.Command("notepad", path)
	}
	return exec.Command(f[0], append(f[1:], path)...)
}

// FlushDNS makes a hosts-file change take effect.
func FlushDNS() { _ = exec.Command("ipconfig", "/flushdns").Run() }

// TailscaleBinary returns the Tailscale CLI path.
func TailscaleBinary() string {
	if p, err := exec.LookPath("tailscale"); err == nil {
		return p
	}
	return `C:\Program Files\Tailscale\tailscale.exe`
}

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
	procFreeConsole             = kernel32.NewProc("FreeConsole")
)

// KeepAwake asks Windows not to sleep while goli runs. The request belongs
// to the calling thread, so a locked goroutine holds it for the process lifetime.
func KeepAwake(int) error {
	const esContinuous, esSystemRequired = 0x80000000, 0x00000001
	errc := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		r, _, err := procSetThreadExecutionState.Call(uintptr(esContinuous | esSystemRequired))
		if r == 0 {
			errc <- fmt.Errorf("SetThreadExecutionState: %w", err)
			return
		}
		errc <- nil
		select {} // keep this thread (and its request) alive
	}()
	return <-errc
}

// Detach hides the console window when goli is started at login.
func Detach() { _, _, _ = procFreeConsole.Call() }

// ── start at login (HKCU Run key; no admin rights needed) ─────────────────

const runKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

// LogLocation tells the user where the background server logs.
func LogLocation() string { return filepath.Join(StateDir(), "goli.log") }

func pidPath() string { return filepath.Join(StateDir(), "goli.pid") }

func serveArgs() []string { return []string{"serve", "-background", "-log", LogLocation()} }

// ServiceInstalled reports whether goli starts at login.
func ServiceInstalled() bool {
	return exec.Command("reg", "query", runKey, "/v", "goli").Run() == nil
}

// InstallService starts goli now and at every login.
func InstallService(bin string, env map[string]string) error {
	if len(env) > 0 {
		// The Run key can't carry environment variables; GOLI_HOME must be set
		// for the user account instead.
		for k := range env {
			if k == "GOLI_HOME" {
				fmt.Fprintln(os.Stderr, "note: set GOLI_HOME as a user environment variable so goli finds it at login")
			}
		}
	}
	if err := os.MkdirAll(StateDir(), 0o700); err != nil {
		return err
	}
	cmdline := syscall.EscapeArg(bin) + " " + strings.Join(escapeAll(serveArgs()), " ")
	if out, err := exec.Command("reg", "add", runKey, "/v", "goli", "/t", "REG_SZ", "/d", cmdline, "/f").CombinedOutput(); err != nil {
		return fmt.Errorf("reg add: %w: %s", err, out)
	}
	return RestartService()
}

// UninstallService stops goli and removes it from login.
func UninstallService() error {
	stop()
	if out, err := exec.Command("reg", "delete", runKey, "/v", "goli", "/f").CombinedOutput(); err != nil && ServiceInstalled() {
		return fmt.Errorf("reg delete: %w: %s", err, out)
	}
	return nil
}

// RestartService stops the background server (if running) and starts it detached.
func RestartService() error {
	stop()
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	const detachedProcess, createNoWindow = 0x00000008, 0x08000000
	cmd := exec.Command(bin, serveArgs()...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | createNoWindow, HideWindow: true}
	return cmd.Start()
}

// WritePID records the background server's process ID so it can be stopped.
// The returned func removes the file again when the server exits cleanly.
func WritePID() (remove func()) {
	_ = os.WriteFile(pidPath(), []byte(strconv.Itoa(os.Getpid())), 0o600)
	return func() { _ = os.Remove(pidPath()) }
}

func stop() {
	data, err := os.ReadFile(pidPath())
	if err != nil {
		return
	}
	if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
		// A stale pid file (after a crash or reboot) may name an unrelated
		// process that reused the number: only kill it if it's goli.
		image := "goli.exe"
		if bin, err := os.Executable(); err == nil {
			image = filepath.Base(bin)
		}
		_ = exec.Command("taskkill", "/F", "/FI", "PID eq "+strconv.Itoa(pid), "/FI", "IMAGENAME eq "+image).Run()
	}
	_ = os.Remove(pidPath())
}

// escapeAll quotes args the way Windows programs parse their command line.
func escapeAll(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = syscall.EscapeArg(a)
	}
	return out
}
