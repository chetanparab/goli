//go:build !darwin && !linux

package secret

// Elsewhere (Windows, BSD) secrets use the 0600 file in goli's config directory.
func systemBackend() backend { return nil }
