//go:build linux || darwin || freebsd || netbsd || openbsd

package main

import (
	"os"
	"syscall"
	"unsafe"
)

// stdinIsTTY reports whether stdin is a terminal.
//
// The ioctl is the portable POSIX answer, but syscall's constants and the
// ioctl(2) call signature differ between platforms, so this file is split by
// build tag rather than pretending one signature fits all. On Linux,
// TCGETS wants a termios struct; on BSD it is TIOCGETA.
func stdinIsTTY() bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL,
		os.Stdin.Fd(),
		uintptr(ioctlGetTermios),
		uintptr(unsafe.Pointer(&t)),
		0, 0, 0,
	)
	return errno == 0
}
