//go:build linux

package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Name is a human name for this OS.
const Name = "Linux"

// DefaultListen binds port 80 on all interfaces; goli's guard refuses
// non-local clients. Binding needs `setcap cap_net_bind_service=+ep`.
const DefaultListen = ":80"

var hostsAddrs = []string{"127.0.0.1", "::1"}

// AddHostsElevated appends lines to /etc/hosts, asking for the user's
// password through polkit's dialog (pkexec) or, without a desktop, sudo in
// this terminal. Only that one shell command is elevated.
func AddHostsElevated(lines []string) error {
	script := hostsAppendScript(lines)
	var cmd *exec.Cmd
	if pk, err := exec.LookPath("pkexec"); err == nil && os.Getenv("DISPLAY")+os.Getenv("WAYLAND_DISPLAY") != "" {
		cmd = exec.Command(pk, "sh", "-c", script)
	} else {
		cmd = exec.Command("sudo", "sh", "-c", script)
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("couldn't edit /etc/hosts: %w", err)
	}
	FlushDNS()
	return nil
}

// OpenURL opens u in the default browser.
func OpenURL(u string) error { return exec.Command("xdg-open", u).Run() }

// FlushDNS makes a hosts-file change take effect where a resolver caches it.
func FlushDNS() { _ = exec.Command("resolvectl", "flush-caches").Run() }

// TailscaleBinary returns the Tailscale CLI path.
func TailscaleBinary() string {
	if p, err := exec.LookPath("tailscale"); err == nil {
		return p
	}
	return "/usr/bin/tailscale"
}

// KeepAwake holds a systemd sleep/idle inhibitor for as long as pid lives.
func KeepAwake(pid int) error {
	if _, err := exec.LookPath("systemd-inhibit"); err != nil {
		return fmt.Errorf("keep_awake needs systemd-inhibit: %w", err)
	}
	cmd := exec.Command("systemd-inhibit", "--what=sleep:idle", "--who=goli", "--why=Serving go/ links",
		"--mode=block", "sh", "-c", "while kill -0 "+strconv.Itoa(pid)+" 2>/dev/null; do sleep 30; done")
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	// systemd-inhibit exits at once when it can't take the lock (no logind,
	// no D-Bus session); report that instead of claiming the machine stays awake.
	select {
	case err := <-done:
		if err == nil {
			err = errors.New("exited immediately")
		}
		return fmt.Errorf("systemd-inhibit: %w", err)
	case <-time.After(time.Second):
		return nil
	}
}

// ── systemd user service ─────────────────────────────────────────────────

func unitPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "systemd", "user", "goli.service")
}

// LogLocation tells the user where the background server logs.
func LogLocation() string { return "journalctl --user -u goli" }

// ServiceInstalled reports whether goli starts at login.
func ServiceInstalled() bool {
	_, err := os.Stat(unitPath())
	return err == nil
}

func systemctl(args ...string) error {
	out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl --user %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return nil
}

// InstallService starts `bin serve` now and at every login (systemd --user).
func InstallService(bin string, env map[string]string) error {
	path := unitPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var envLines strings.Builder
	for k, v := range env {
		fmt.Fprintf(&envLines, "Environment=%s=%s\n", k, strconv.Quote(v))
	}
	unit := fmt.Sprintf(`[Unit]
Description=goli go/ short links
After=network.target

[Service]
ExecStart=%s serve
%sRestart=on-failure
UMask=0077

[Install]
WantedBy=default.target
`, strconv.Quote(bin), envLines.String())
	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
		return err
	}
	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	return systemctl("enable", "--now", "goli.service")
}

// UninstallService stops goli and removes it from login.
func UninstallService() error {
	_ = systemctl("disable", "--now", "goli.service")
	if err := os.Remove(unitPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return systemctl("daemon-reload")
}

// RestartService restarts the background server (e.g. after an upgrade).
func RestartService() error { return systemctl("restart", "goli.service") }

// Detach is a no-op outside Windows.
func Detach() {}
