// Package output renders engine plans, apply results, and diagnostics
// for terminal and JSON consumption.
package output

import (
	"io"
	"os"
)

// ANSI escape codes.
const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiBold   = "\033[1m"
)

func green(s string, color bool) string {
	if !color {
		return s
	}
	return ansiGreen + s + ansiReset
}

func red(s string, color bool) string {
	if !color {
		return s
	}
	return ansiRed + s + ansiReset
}

func yellow(s string, color bool) string {
	if !color {
		return s
	}
	return ansiYellow + s + ansiReset
}

func bold(s string, color bool) string {
	if !color {
		return s
	}
	return ansiBold + s + ansiReset
}

func boldRed(s string, color bool) string {
	if !color {
		return s
	}
	return ansiBold + ansiRed + s + ansiReset
}

// ShouldColor returns true when w is a TTY and the NO_COLOR env var is not set.
func ShouldColor(w io.Writer) bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeCharDevice != 0
}
