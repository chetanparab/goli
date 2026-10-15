//go:build darwin

package platform

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// Name is a human name for this OS.
const Name = "macOS"

// DefaultListen binds all interfaces: macOS only lets a normal user bind
// port 80 that way (goli's guard still refuses non-local clients).
const DefaultListen = ":80"

var hostsLines = []string{"127.0.0.1\tgo", "::1\tgo"}

// OpenURL opens u in the default browser.
func OpenURL(u string) error { return exec.Command("open", u).Run() }

// FlushDNS makes a hosts-file change take effect.
func FlushDNS() {
	_ = exec.Command("dscacheutil", "-flushcache").Run()
	_ = exec.Command("killall", "-HUP", "mDNSResponder").Run()
}

// TailscaleBinary returns the Tailscale CLI path.
func TailscaleBinary() string {
	if p, err := exec.LookPath("tailscale"); err == nil {
		return p
	}
	return "/Applications/Tailscale.app/Contents/MacOS/Tailscale"
}

// KeepAwake prevents idle sleep while on power, for as long as pid lives.
// Closing the lid still sleeps the Mac unless an external display is attached.
func KeepAwake(pid int) error {
	cmd := exec.Command("/usr/bin/caffeinate", "-s", "-w", strconv.Itoa(pid))
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// ── LaunchAgent ──────────────────────────────────────────────────────────

const label = "dev.goli.server"

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
}

// LogLocation tells the user where the background server logs.
func LogLocation() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Logs", "goli.log")
}

func domain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

// ServiceInstalled reports whether goli starts at login.
func ServiceInstalled() bool {
	_, err := os.Stat(plistPath())
	return err == nil
}

// InstallService starts `bin serve` now and at every login.
func InstallService(bin string, env map[string]string) error {
	path := plistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, ok := env["PATH"]; !ok {
		env["PATH"] = "/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin"
	}
	envXML := ""
	for k, v := range env {
		envXML += fmt.Sprintf("\t\t<key>%s</key><string>%s</string>\n", html.EscapeString(k), html.EscapeString(v))
	}
	log := html.EscapeString(LogLocation())
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key>
	<array><string>%s</string><string>serve</string></array>
	<key>EnvironmentVariables</key>
	<dict>
%s	</dict>
	<key>RunAtLoad</key><true/>
	<key>KeepAlive</key><true/>
	<key>ProcessType</key><string>Background</string>
	<key>Umask</key><integer>63</integer>
	<key>StandardOutPath</key><string>%s</string>
	<key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, label, html.EscapeString(bin), envXML, log, log)
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return err
	}
	_ = exec.Command("launchctl", "bootout", domain()+"/"+label).Run()
	if out, err := exec.Command("launchctl", "bootstrap", domain(), path).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w: %s", err, out)
	}
	return nil
}

// UninstallService stops goli and removes it from login items.
func UninstallService() error {
	_ = exec.Command("launchctl", "bootout", domain()+"/"+label).Run()
	if err := os.Remove(plistPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// RestartService restarts the background server (e.g. after an upgrade).
func RestartService() error {
	if out, err := exec.Command("launchctl", "kickstart", "-k", domain()+"/"+label).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl kickstart: %w: %s", err, out)
	}
	return nil
}

// Detach is a no-op outside Windows.
func Detach() {}
