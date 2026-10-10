//go:build !unix

package links

// lockFile is a no-op where flock isn't available; the in-process mutex
// still serialises edits from a single goli process.
func lockFile(string) (func(), error) { return func() {}, nil }
