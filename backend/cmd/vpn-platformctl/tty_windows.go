//go:build windows

package main

import "os"

// stdinIsTTY reports whether stdin is a console.
//
// This uses the stdlib rather than golang.org/x/sys/windows GetConsoleMode:
// x/sys is only an INDIRECT dependency of this module, and importing it
// directly would promote it in go.mod for one boolean. On Windows a console
// handle reports a non-regular file mode; a redirected stdin reports a name, a
// pipe or a regular file. That is the same distinction the ioctl draws elsewhere.
func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	const namedPipe = os.ModeNamedPipe
	return fi.Mode()&os.ModeCharDevice != 0 && fi.Mode()&(os.ModeNamedPipe|namedPipe) == 0 &&
		fi.Mode()&os.ModeDir == 0
}
