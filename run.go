package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/shu-go/hcstatus/osc7501"
)

type runCmd struct {
	common
	DoneMsg  string `cli:"done-msg=TEXT" help:"message on success (default: \"done: COMMAND\")"`
	ErrorMsg string `cli:"error-msg=TEXT" help:"message on failure (default: \"failed (exit N): COMMAND\")"`
}

func (c runCmd) Run(g *globalCmd, args []string) error {
	if len(args) == 0 {
		return errors.New("run needs a command: hcstatus run [OPTIONS] -- COMMAND [ARGS...]")
	}
	e := emitter{g}

	cmdline := osc7501.SanitizeText(strings.Join(args, " "), osc7501.MaxMsgDecoded)
	if c.App == "" {
		c.App = defaultApp(args[0])
	}
	if c.Msg == "" {
		c.Msg = cmdline
	}

	// Validate everything before starting the command so that a bad option
	// fails fast instead of after a long run.
	working := c.report(osc7501.StateWorking, nil)
	if err := working.Validate(); err != nil {
		return err
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	// The child shares the terminal and receives Ctrl+C itself. We keep
	// running so that we can report "idle" after it exits.
	var interrupted atomic.Bool
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigs)

	if err := e.emit(working); err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		exitCode = 127
		r := c.report(osc7501.StateError, nil)
		r.Msg = osc7501.SanitizeText("failed to start: "+cmdline, osc7501.MaxMsgDecoded)
		if emitErr := e.emit(r); emitErr != nil {
			g.verbosef("hcstatus: %v\n", emitErr)
		}
		return err
	}

	go func() {
		for s := range sigs {
			interrupted.Store(true)
			if s == syscall.SIGTERM {
				// Not delivered to the child automatically; forward it.
				// (Unsupported on Windows; the error is ignored.)
				_ = cmd.Process.Signal(s)
			}
		}
	}()

	waitErr := cmd.Wait()
	code := 0
	if waitErr != nil {
		ee, ok := errors.AsType[*exec.ExitError](waitErr)
		if !ok {
			code = 1
		} else if code = ee.ExitCode(); code < 0 {
			code = 1 // killed by a signal
		}
	}
	exitCode = code

	var r osc7501.Report
	switch {
	case interrupted.Load():
		r = c.report(osc7501.StateIdle, nil)
		r.Msg = osc7501.SanitizeText("interrupted: "+cmdline, osc7501.MaxMsgDecoded)
	case code == 0:
		r = c.report(osc7501.StateDone, nil)
		r.Msg = pick(c.DoneMsg, "done: "+cmdline)
	default:
		r = c.report(osc7501.StateError, nil)
		r.Msg = pick(c.ErrorMsg, fmt.Sprintf("failed (exit %d): %s", code, cmdline))
	}
	if err := e.emit(r); err != nil {
		g.verbosef("hcstatus: %v\n", err)
	}
	return nil
}

func pick(custom, def string) string {
	if custom == "" {
		custom = def
	}
	return osc7501.SanitizeText(custom, osc7501.MaxMsgDecoded)
}

// defaultApp derives a valid app name from the command path
// ("C:\bin\make.exe" -> "make").
func defaultApp(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	var b strings.Builder
	for i := 0; i < len(base) && b.Len() < osc7501.MaxAppBytes; i++ {
		ch := base[i]
		switch {
		case 'a' <= ch && ch <= 'z', 'A' <= ch && ch <= 'Z', '0' <= ch && ch <= '9',
			ch == '_', ch == '.', ch == '+', ch == '-':
			b.WriteByte(ch)
		}
	}
	return b.String()
}
