//go:build linux || darwin || freebsd || netbsd || openbsd

package zerologger

import (
	"io"

	"golang.org/x/term"
)

type fdWriter interface {
	Fd() uintptr
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(fdWriter)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
