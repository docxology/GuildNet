//go:build !linux

package main

import "syscall"

// setParentDeathSignal is a no-op on non-Linux platforms.
func setParentDeathSignal(sig syscall.Signal) error { return nil }
