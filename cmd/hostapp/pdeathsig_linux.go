//go:build linux

package main

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// setParentDeathSignal requests that the kernel deliver the given signal
// to this process if its parent process exits. This helps ensure the
// server terminates when the launching terminal is closed.
func setParentDeathSignal(sig syscall.Signal) error {
	return unix.Prctl(unix.PR_SET_PDEATHSIG, uintptr(sig), 0, 0, 0)
}
