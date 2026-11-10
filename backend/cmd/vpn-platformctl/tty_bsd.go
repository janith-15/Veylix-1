//go:build darwin || freebsd || netbsd || openbsd

package main

import "syscall"

// TIOCGETA / TIOCSETA are the BSD spelling of the same request pair.
const (
	ioctlGetTermios = syscall.TIOCGETA
	ioctlSetTermios = syscall.TIOCSETA
)
