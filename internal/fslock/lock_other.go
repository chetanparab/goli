//go:build !unix

// Package fslock serialises goli processes that edit the same file.
package fslock

// Lock is a no-op where flock isn't available; each process's own mutex
// still serialises its edits.
func Lock(string) (func(), error) { return func() {}, nil }
