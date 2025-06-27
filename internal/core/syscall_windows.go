//go:build windows
// +build windows

package core

func getSystemFDLimit() int {
	// Windows doesn't have the same FD limits as Unix systems
	// Return a reasonable default for Windows
	return 10000
}
