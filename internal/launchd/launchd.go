// Package launchd installs goli as a macOS LaunchAgent so the server starts at login.
package launchd

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
)

// Label is the LaunchAgent identifier.
const Label = "dev.goli.server"

// PlistPath returns ~/Library/LaunchAgents/dev.goli.server.plist.
func PlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist"), nil
}

// LogPath is where the agent's stdout/stderr go.
func LogPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Logs", "goli.log")
}

func domain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

// Install writes the plist for binary and (re)loads it.
func Install(binary string, env map[string]string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("launchd is macOS-only; on Linux run `goli serve` under systemd (see README)")
	}
	path, err := PlistPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	envXML := ""
	for k, v := range env {
		envXML += fmt.Sprintf("\t\t<key>%s</key><string>%s</string>\n", html.EscapeString(k), html.EscapeString(v))
	}
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
`, Label, html.EscapeString(binary), envXML, html.EscapeString(LogPath()), html.EscapeString(LogPath()))
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return err
	}
	_ = exec.Command("launchctl", "bootout", domain()+"/"+Label).Run()
	if out, err := exec.Command("launchctl", "bootstrap", domain(), path).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w: %s", err, out)
	}
	return nil
}

// Uninstall unloads and removes the agent.
func Uninstall() error {
	path, err := PlistPath()
	if err != nil {
		return err
	}
	_ = exec.Command("launchctl", "bootout", domain()+"/"+Label).Run()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
