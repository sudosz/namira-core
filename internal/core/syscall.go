//go:build !windows
// +build !windows

package core

import "syscall"

func getSystemFDLimit() int {
	var rlim syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlim)
	if err != nil {
		return 10000
	}

	return min(int(rlim.Cur), 100000)
}
