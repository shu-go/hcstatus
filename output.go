package main

import (
	"fmt"
	"os"

	"github.com/shu-go/hcstatus/osc7501"
)

// emitter writes reports to the destination chosen by global options.
//
// Under herdr (HC_TERMINAL_ID is set), a report's title is also sent as the
// window title (OSC 2), because herdr-app's agents list shows the terminal
// title but not the OSC 7501 title/msg. With tab set, the herdr-app tab is
// renamed to the title as well (once per emitter; failures are ignored).
type emitter struct {
	g   *globalCmd
	tab bool

	tabRenamed bool
}

func newEmitter(g *globalCmd, tab bool) *emitter {
	return &emitter{g: g, tab: tab}
}

func (e *emitter) emit(r osc7501.Report) error {
	terminal := herdrTerminal()
	seq, err := e.build(r, terminal != "")
	if err != nil {
		return err
	}

	if err := e.write(seq); err != nil {
		return err
	}

	if terminal != "" && !e.tabRenamed {
		switch {
		case r.State == osc7501.StateClear:
			// clear resets the tab name (to its number) without --tab.
			e.renameTab(terminal, "")
		case e.tab && r.Title != "":
			// Never an empty name here: that would reset the tab.
			e.renameTab(terminal, r.Title)
		}
	}
	return nil
}

func (e *emitter) renameTab(terminal, name string) {
	e.tabRenamed = true
	if err := renameHerdrTab(terminal, name); err != nil {
		e.g.verbosef("hcstatus: tab not renamed: %v\n", err)
	}
}

func (e *emitter) write(seq []byte) error {
	switch e.g.Out {
	case "stdout":
		_, err := os.Stdout.Write(seq)
		return err
	case "stderr":
		_, err := os.Stderr.Write(seq)
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
func (e *emitter) build(r osc7501.Report, withTitle bool) ([]byte, error) {
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
