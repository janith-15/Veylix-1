//go:build windows

package main

import (
	"bufio"
	"os"
	"strings"
	"syscall"
)

// kernel32 is loaded lazily: SetConsoleMode is not in the Go stdlib's syscall
// package, and pulling in golang.org/x/sys/windows for one call would promote an
// indirect dependency in go.mod.
var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
)

// readSecretRaw reads one line from stdin with echo disabled.
//
// Windows has no termios; the equivalent is SetConsoleMode clearing
// ENABLE_ECHO_INPUT on the console input handle. The previous mode is restored in
// a defer so a failure cannot leave the operator typing invisibly.
func readSecretRaw() (string, error) {
	h := syscall.Handle(os.Stdin.Fd())
	var mode uint32
	if err := syscall.GetConsoleMode(h, &mode); err != nil {
		// Not a console: a piped password is legitimate, and echo is already off.
		return readLinePlain()
	}
	const enableEchoInput = 0x0004
	if r, _, _ := procSetConsoleMode.Call(uintptr(h), uintptr(mode&^enableEchoInput)); r == 0 {
		return readLinePlain()
	}
	defer func() {
		// Best effort: the password is already read and there is nothing useful
		// to do if restoring the mode fails.
		_, _, _ = procSetConsoleMode.Call(uintptr(h), uintptr(mode))
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
