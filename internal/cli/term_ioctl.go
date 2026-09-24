//go:build darwin || linux

package cli

import (
	"os"
	"syscall"
	"unsafe"
)

// ttyColumns is the width of the terminal f is attached to, or 0.
func ttyColumns(f *os.File) int {
	var ws struct{ Row, Col, X, Y uint16 }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return 0
	}
	return int(ws.Col)
}
