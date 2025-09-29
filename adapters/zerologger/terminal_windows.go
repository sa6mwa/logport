//go:build windows

package zerologger

import (
	"io"

	"golang.org/x/sys/windows"
)

type fdWriter interface {
	Fd() uintptr
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(fdWriter)
	if !ok {
		return false
	}
	handle := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return false
	}
	return true
}
