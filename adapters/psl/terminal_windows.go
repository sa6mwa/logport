//go:build windows

package psl

import (
	"io"
	"syscall"
)

type syscallWriter interface {
	Fd() uintptr
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(syscallWriter)
	if !ok {
		return false
	}
	var st uint32
	if syscall.GetConsoleMode(syscall.Handle(f.Fd()), &st) != nil {
		return false
	}
	return true
}
