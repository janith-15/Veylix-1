//go:build linux

package main

import "syscall"

// TCGETS reads the termios struct and TCSETS writes it back, on Linux.
const (
	ioctlGetTermios = syscall.TCGETS
	ioctlSetTermios = syscall.TCSETS
)
