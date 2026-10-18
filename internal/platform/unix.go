//go:build unix

package platform

import (
	"fmt"
	"os/exec"
	"strings"
)

// HostsPath is the system hosts file.
func HostsPath() string { return "/etc/hosts" }

// HostsInstructions is the command a user runs (it needs their password) to add lines.
func HostsInstructions(lines []string) string {
	body := strings.ReplaceAll(strings.Join(lines, "\\n"), "\t", " ")
	return fmt.Sprintf("printf '\\n# goli: go/ short links\\n%s\\n' | sudo tee -a /etc/hosts >/dev/null", body)
}

// hostsAppendScript is the one small shell command that runs with admin
// rights when the user says yes to adding lines. Names are DNS labels, so
// nothing here needs quoting.
func hostsAppendScript(lines []string) string {
	body := strings.ReplaceAll(strings.Join(lines, "\\n"), "\t", " ")
	return fmt.Sprintf("printf '\\n# goli: go/ short links\\n%s\\n' >> /etc/hosts", body)
}

// EditCommand runs $EDITOR (which may include arguments) on path.
func EditCommand(editor, path string) *exec.Cmd {
	if editor == "" {
		editor = "vi"
	}
	return exec.Command("sh", "-c", editor+` "$1"`, "sh", path)
}
