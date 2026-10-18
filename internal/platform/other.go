//go:build unix && !darwin && !linux

package platform

import (
	"errors"
	"os/exec"
)

// Name is a human name for this OS.
const Name = "this OS"

// DefaultListen binds port 80 on all interfaces.
const DefaultListen = ":80"

var hostsAddrs = []string{"127.0.0.1", "::1"}

var errUnsupported = errors.New("not supported on this OS yet; run `goli serve` under your service manager")

// OpenURL opens u in the default browser.
func OpenURL(u string) error { return exec.Command("xdg-open", u).Run() }

// FlushDNS is a no-op here.
func FlushDNS() {}

// TailscaleBinary returns the Tailscale CLI path.
func TailscaleBinary() string { return "tailscale" }

// KeepAwake isn't supported here.
func KeepAwake(int) error { return errUnsupported }

// LogLocation tells the user where the server logs.
func LogLocation() string { return "stderr of `goli serve`" }

// ServiceInstalled reports whether goli starts at login.
func ServiceInstalled() bool { return false }

// InstallService isn't supported here.
func InstallService(string, map[string]string) error { return errUnsupported }

// UninstallService isn't supported here.
func UninstallService() error { return errUnsupported }

// RestartService isn't supported here.
func RestartService() error { return errUnsupported }

// Detach is a no-op.
func Detach() {}
