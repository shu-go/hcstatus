package main

import (
	"testing"

	"github.com/shu-go/hcstatus/osc7501"
)

func TestBuild(t *testing.T) {
	r := osc7501.Report{State: osc7501.StateWorking, App: "appname", Title: "title", Msg: "msg"}
	report := "\x1b]7501;state=working:app=appname:title=dGl0bGU=:msg=bXNn\x1b\\"

	tests := []struct {
		name      string
		g         globalCmd
		r         osc7501.Report
		withTitle bool
		want      string
	}{
		{"no herdr", globalCmd{Tmux: "off"}, r, false, report},
		{"herdr", globalCmd{Tmux: "off"}, r, true, "\x1b]2;title\x1b\\" + report},
		{"herdr bel", globalCmd{Tmux: "off", BEL: true}, r, true,
			"\x1b]2;title\x07\x1b]7501;state=working:app=appname:title=dGl0bGU=:msg=bXNn\x07"},
		{"herdr without title", globalCmd{Tmux: "off"}, osc7501.Report{State: osc7501.StateDone}, true,
			"\x1b]7501;state=done\x1b\\"},
		{"herdr tmux", globalCmd{Tmux: "on"}, osc7501.Report{State: osc7501.StateIdle, Title: "t"}, true,
			"\x1bPtmux;\x1b\x1b]2;t\x1b\x1b\\\x1b\\" + "\x1bPtmux;\x1b\x1b]7501;state=idle:title=dA==\x1b\x1b\\\x1b\\"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := emitter{&tt.g}.build(tt.r, tt.withTitle)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestBuildInvalidSendsNothing(t *testing.T) {
	g := globalCmd{Tmux: "off"}
	if got, err := (emitter{&g}).build(osc7501.Report{State: "bad", Title: "t"}, true); err == nil || got != nil {
		t.Errorf("want error and no bytes, got %q, %v", got, err)
	}
}
