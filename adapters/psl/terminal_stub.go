//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !windows

package psl

import "io"

func isTerminal(io.Writer) bool { return false }
