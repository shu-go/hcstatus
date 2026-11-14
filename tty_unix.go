//go:build !windows

package main

import "os"

// openTTY opens the controlling terminal so that sequences bypass any
// redirection of stdout/stderr.
func openTTY() (*os.File, func(), error) {
	f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { f.Close() }, nil
}
