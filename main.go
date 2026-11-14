package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/shu-go/gli/v2"
	"github.com/shu-go/hcstatus/osc7501"
)

var version = "dev"

type globalCmd struct {
	Working workingCmd `cli:"working,w" help:"report that work is in progress" usage:"hcstatus working [--progress N] [MESSAGE...]"`
	Blocked blockedCmd `cli:"blocked,b" help:"report that user action is needed" usage:"hcstatus blocked [--kind permission|question|auth] [MESSAGE...]"`
	Idle    idleCmd    `cli:"idle,i" help:"report waiting for the next instruction" usage:"hcstatus idle [MESSAGE...]"`
	Done    doneCmd    `cli:"done,d" help:"report finished work the user has not seen yet" usage:"hcstatus done [MESSAGE...]"`
	Error   errorCmd   `cli:"error,e" help:"report a failure" usage:"hcstatus error [MESSAGE...]"`
	Clear   clearCmd   `cli:"clear,c" help:"remove a record and its children (all records without --id); under herdr, also reset the tab name" usage:"hcstatus clear [--id ID]"`
	Exec    runCmd     `cli:"run,r" help:"run a command, reporting working -> done/error (idle if interrupted)" usage:"hcstatus run [OPTIONS] -- COMMAND [ARGS...]\nExits with the status of COMMAND."`

	Out     string `cli:"out,o=DEST" default:"tty" help:"where to write: tty (default; bypasses redirection), stdout, stderr"`
	BEL     bool   `cli:"bel" help:"terminate with BEL instead of ESC \\"`
	Tmux    string `cli:"tmux=MODE" default:"auto" help:"wrap in tmux passthrough: auto (when $TMUX is set), on, off"`
	Verbose bool   `cli:"v,verbose" help:"print diagnostics to stderr"`
}

func (g *globalCmd) Before() error {
	switch g.Out {
	case "tty", "stdout", "stderr":
	default:
		return fmt.Errorf("--out must be tty, stdout or stderr (got %q)", g.Out)
	}
	switch g.Tmux {
	case "auto", "on", "off":
	default:
		return fmt.Errorf("--tmux must be auto, on or off (got %q)", g.Tmux)
	}
	return nil
}

func (g *globalCmd) passthroughTmux() bool {
	switch g.Tmux {
	case "on":
		return true
	case "off":
		return false
	default:
		return os.Getenv("TMUX") != ""
	}
}

func (g *globalCmd) verbosef(format string, a ...any) {
	if g.Verbose {
		fmt.Fprintf(os.Stderr, format, a...)
	}
}

// common holds keys shared by all report commands.
type common struct {
	ID    string `cli:"id=ID" help:"record path such as build/test (default: root record)"`
	App   string `cli:"app,a=NAME" help:"stable program name [A-Za-z0-9_.+-]{1,32}"`
	Title string `cli:"title,t=TEXT" help:"short label, typically for child records (under herdr, i.e. $HC_TERMINAL_ID is set, also sent as the window title)"`
	Msg   string `cli:"msg,m=TEXT" help:"one-line message (default: the arguments joined by spaces)"`
	Tab   bool   `cli:"tab" help:"under herdr, also rename the herdr-app tab to --title (errors are ignored)"`
}

// report builds a Report from the common options. Free text is sanitized
// (line breaks -> spaces, other control characters removed, truncated)
// because terminals silently drop reports containing control characters.
func (c common) report(state osc7501.State, args []string) osc7501.Report {
	msg := c.Msg
	if msg == "" {
		msg = strings.Join(args, " ")
	}
	return osc7501.Report{
		State: state,
		ID:    c.ID,
		App:   c.App,
		Title: osc7501.SanitizeText(c.Title, osc7501.MaxTitleDecoded),
		Msg:   osc7501.SanitizeText(msg, osc7501.MaxMsgDecoded),
	}
}

type workingCmd struct {
	common
	Progress *int `cli:"progress,p=PERCENT" help:"progress 0..100 (omit if unknown)"`
}

func (c workingCmd) Run(g *globalCmd, args []string) error {
	r := c.report(osc7501.StateWorking, args)
	r.Progress = c.Progress
	return newEmitter(g, c.Tab).emit(r)
}

type blockedCmd struct {
	common
	Kind     string `cli:"kind,k=KIND" help:"what is needed: permission, question or auth"`
	Progress *int   `cli:"progress,p=PERCENT" help:"progress 0..100 (omit if unknown)"`
}

func (c blockedCmd) Run(g *globalCmd, args []string) error {
	r := c.report(osc7501.StateBlocked, args)
	r.Kind = osc7501.Kind(c.Kind)
	r.Progress = c.Progress
	return newEmitter(g, c.Tab).emit(r)
}

type idleCmd struct{ common }

func (c idleCmd) Run(g *globalCmd, args []string) error {
	return newEmitter(g, c.Tab).emit(c.report(osc7501.StateIdle, args))
}

type doneCmd struct{ common }

func (c doneCmd) Run(g *globalCmd, args []string) error {
	return newEmitter(g, c.Tab).emit(c.report(osc7501.StateDone, args))
}

type errorCmd struct{ common }

func (c errorCmd) Run(g *globalCmd, args []string) error {
	return newEmitter(g, c.Tab).emit(c.report(osc7501.StateError, args))
}

type clearCmd struct {
	ID string `cli:"id=ID" help:"record path to remove together with its children (default: all records)"`
}

func (c clearCmd) Run(g *globalCmd, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("clear takes no arguments (got %q)", args)
	}
	return newEmitter(g, false).emit(osc7501.Report{State: osc7501.StateClear, ID: c.ID})
}

// exitCode is set by commands that must propagate a specific exit status.
var exitCode = 0

func main() {
	app := gli.NewWith(&globalCmd{})
	app.Name = "hcstatus"
	app.Desc = "report program status to the terminal via OSC 7501"
	app.Version = version
	app.Copyright = "(C) 2026 Shuhei Kubota"

	if err := app.Run(os.Args); err != nil {
		// gli has already printed the error.
		if exitCode == 0 {
			exitCode = 2
		}
	}
	os.Exit(exitCode)
}
