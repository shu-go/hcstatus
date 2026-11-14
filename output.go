package main

import (
	"fmt"
	"os"

	"github.com/shu-go/hcstatus/osc7501"
)

// herdrTerminalEnv is set by herdr-core in terminals it runs. herdr-app's
// agents list does not show OSC 7501 title/msg, but it does show the terminal
// title (OSC 0/2), so there we also send --title as the window title.
const herdrTerminalEnv = "HC_TERMINAL_ID"

// emitter writes reports to the destination chosen by global options.
type emitter struct {
	g *globalCmd
}

func (e emitter) emit(r osc7501.Report) error {
	seq, err := e.build(r, os.Getenv(herdrTerminalEnv) != "")
	if err != nil {
		return err
	}

	switch e.g.Out {
	case "stdout":
		_, err = os.Stdout.Write(seq)
		return err
	case "stderr":
		_, err = os.Stderr.Write(seq)
		return err
	case "tty", "":
		f, closeFn, err := openTTY()
		if err != nil {
			// No terminal (CI, cron, hooks, ...): status reporting is best
			// effort, so do not break the caller.
			e.g.verbosef("hcstatus: no terminal, report skipped: %v\n", err)
			return nil
		}
		defer closeFn()
		_, err = f.Write(seq)
		return err
	default:
		return fmt.Errorf("--out must be tty, stdout or stderr (got %q)", e.g.Out)
	}
}

// build returns the bytes to write: the OSC 7501 report, preceded by an
// OSC 2 window title when withTitle is set and r has a title.
func (e emitter) build(r osc7501.Report, withTitle bool) ([]byte, error) {
	term := osc7501.ST
	if e.g.BEL {
		term = osc7501.BEL
	}
	report, err := r.Encode(term)
	if err != nil {
		return nil, err
	}

	var seqs [][]byte
	if withTitle && r.Title != "" {
		// r.Title has passed Validate: no control characters, so it cannot
		// terminate the sequence early.
		seqs = append(seqs, windowTitle(r.Title, term))
	}
	seqs = append(seqs, report)

	var out []byte
	for _, s := range seqs {
		if e.g.passthroughTmux() {
			s = osc7501.WrapTmux(s)
		}
		out = append(out, s...)
	}
	return out, nil
}

// windowTitle returns OSC 2 (set window title).
func windowTitle(title string, t osc7501.Terminator) []byte {
	end := "\x1b\\"
	if t == osc7501.BEL {
		end = "\x07"
	}
	return []byte("\x1b]2;" + title + end)
}
