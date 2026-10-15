//go:build !darwin && !linux && !windows

package secret

// Elsewhere (BSD) secrets use the 0600 file in goli's config directory.
func systemBackend() backend { return nil }
