//go:build unix

package platform

// WritePID is only needed on Windows, where there's no service manager to
// stop the background server.
func WritePID() {}
