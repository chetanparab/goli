//go:build windows

package secret

import (
	"os"
	"path/filepath"
)

// On Windows secrets go in %LocalAppData%\goli\secrets: unlike goli's config
// folder in %AppData% (Roaming), it isn't copied to other machines by
// roaming profiles. The folder is private to the user account.
func systemBackend() backend {
	dir := os.Getenv("LOCALAPPDATA")
	if dir == "" {
		return nil
	}
	return fileBackend{dir: filepath.Join(dir, "goli", "secrets")}
}
