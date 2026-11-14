//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// openTTY opens the console screen buffer (CONOUT$) so that sequences bypass
// any redirection of stdout/stderr. Virtual terminal processing is enabled
// while writing so that the console passes escape sequences on instead of
// printing them literally; the original mode is restored on close.
func openTTY() (*os.File, func(), error) {
	name, err := windows.UTF16PtrFromString("CONOUT$")
	if err != nil {
		return nil, nil, err
	}
	h, err := windows.CreateFile(name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, nil, err
	}

	restore := func() {}
	var mode uint32
	if windows.GetConsoleMode(h, &mode) == nil {
		want := mode | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING | windows.ENABLE_PROCESSED_OUTPUT
		if want != mode && windows.SetConsoleMode(h, want) == nil {
			restore = func() { windows.SetConsoleMode(h, mode) }
		}
	}

	f := os.NewFile(uintptr(h), "CONOUT$")
	return f, func() { restore(); f.Close() }, nil
}
