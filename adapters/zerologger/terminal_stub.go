//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !windows

package zerologger

import "io"

func isTerminal(io.Writer) bool { return false }
