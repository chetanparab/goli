//go:build darwin || linux

package secret

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// toolTimeout bounds each call to a keyring tool, so a locked or missing
// keyring daemon can't hang goli.
const toolTimeout = 5 * time.Second

// run runs a keyring tool with a timeout and returns its stdout.
func run(stdin string, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin) // secrets never go on a command line
	}
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return out, fmt.Errorf("%s timed out (is your keyring locked?)", filepath.Base(name))
	}
	return out, err
}
