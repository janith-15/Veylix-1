//go:build linux || darwin || freebsd || netbsd || openbsd

package main

import (
	"bufio"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// readSecretRaw reads one line from stdin with terminal echo disabled.
//
// Echo is turned off with the same ioctl used to detect a TTY, and restored with
// a defer, so a panic or an early return cannot leave the operator's terminal in
// a state where nothing they type is visible - including the next command they
// try to type.
func readSecretRaw() (string, error) {
	fd := os.Stdin.Fd()
	var old syscall.Termios
	if _, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd,
		uintptr(ioctlGetTermios), uintptr(unsafe.Pointer(&old)), 0, 0, 0); errno != 0 {
		// Not a terminal: read plainly rather than refuse. A password piped in
		// from a secret manager is a legitimate use, and echo is already off.
		return readLinePlain()
	}

	quiet := old
	quiet.Lflag &^= syscall.ECHO
	quiet.Lflag |= syscall.ICANON | syscall.ISIG
	if _, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd,
		uintptr(ioctlSetTermios), uintptr(unsafe.Pointer(&quiet)), 0, 0, 0); errno != 0 {
		return readLinePlain()
	}
	defer func() {
		// Best effort: there is nothing useful to do if this fails, and the
		// password has already been read.
		_, _, _ = syscall.Syscall6(syscall.SYS_IOCTL, fd,
			uintptr(ioctlSetTermios), uintptr(unsafe.Pointer(&old)), 0, 0, 0)
	}()

	return readLinePlain()
}

// readLinePlain reads one line from stdin.
func readLinePlain() (string, error) {
	line, err := bufio.NewReader(os.Stdin).ReadString(newlineByte)
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
