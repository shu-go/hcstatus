package osc7501

import (
	"strings"
	"testing"
)

func TestEncode(t *testing.T) {
	tests := []struct {
		name string
		r    Report
		term Terminator
		want string
	}{
		{"minimal", Report{State: StateIdle}, ST, "\x1b]7501;state=idle\x1b\\"},
		{"bel", Report{State: StateDone}, BEL, "\x1b]7501;state=done\x07"},
		{
			"all keys",
			Report{State: StateBlocked, ID: "build/test", Kind: KindPermission, Progress: new(40), App: "terraform", Title: "t", Msg: "Syncing photos"},
			ST,
			"\x1b]7501;state=blocked:id=build/test:kind=permission:progress=40:app=terraform:title=dA==:msg=U3luY2luZyBwaG90b3M=\x1b\\",
		},
		{"utf8 msg", Report{State: StateWorking, Msg: "同期中"}, ST, "\x1b]7501;state=working:msg=5ZCM5pyf5Lit\x1b\\"},
		{"progress 0", Report{State: StateWorking, Progress: new(0)}, ST, "\x1b]7501;state=working:progress=0\x1b\\"},
		{"clear all", Report{State: StateClear}, ST, "\x1b]7501;state=clear\x1b\\"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.r.Encode(tt.term)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestValidateErrors(t *testing.T) {
	tests := []struct {
		name string
		r    Report
		want string // substring of the error
	}{
		{"no state", Report{}, "state is required"},
		{"bad state", Report{State: "running"}, "unknown state"},
		{"kind without blocked", Report{State: StateWorking, Kind: KindQuestion}, "only valid with state=blocked"},
		{"bad kind", Report{State: StateBlocked, Kind: "other"}, "unknown kind"},
		{"progress too big", Report{State: StateWorking, Progress: new(101)}, "progress must be"},
		{"progress negative", Report{State: StateWorking, Progress: new(-1)}, "progress must be"},
		{"progress with done", Report{State: StateDone, Progress: new(10)}, "only valid with state=working or blocked"},
		{"app too long", Report{State: StateIdle, App: strings.Repeat("a", 33)}, "app is 33 bytes"},
		{"app bad char", Report{State: StateIdle, App: "my app"}, "outside [A-Za-z0-9_.+-]"},
		{"id empty seg", Report{State: StateIdle, ID: "a//b"}, "empty segment"},
		{"id trailing slash", Report{State: StateIdle, ID: "a/"}, "empty segment"},
		{"id bad char", Report{State: StateIdle, ID: "a:b"}, "outside"},
		{"id seg too long", Report{State: StateIdle, ID: strings.Repeat("a", 33)}, "segment"},
		{"id too deep", Report{State: StateIdle, ID: "a/b/c/d/e/f/g/h/i"}, "9 levels"},
		{"id too long", Report{State: StateIdle, ID: strings.Repeat(strings.Repeat("a", 32)+"/", 4) + "a"}, "id is 133 bytes"},
		{"msg newline", Report{State: StateIdle, Msg: "a\nb"}, "control character U+000A"},
		{"msg C1", Report{State: StateIdle, Msg: "a\u0085b"}, "control character U+0085"},
		{"msg DEL", Report{State: StateIdle, Msg: "a\x7fb"}, "control character U+007F"},
		{"msg invalid utf8", Report{State: StateIdle, Msg: "a\xffb"}, "not valid UTF-8"},
		{"msg too long", Report{State: StateIdle, Msg: strings.Repeat("a", 2049)}, "msg is 2049 bytes"},
		{"title too long", Report{State: StateIdle, Title: strings.Repeat("a", 193)}, "title is 193 bytes"},
		{"title control", Report{State: StateIdle, Title: "a\tb"}, "control character U+0009"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.r.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not contain %q", err, tt.want)
			}
		})
	}
}

func TestLimitsAccepted(t *testing.T) {
	r := Report{
		State:    StateBlocked,
		ID:       strings.Repeat(strings.Repeat("a", 31)+"/", 3) + strings.Repeat("a", 32), // 128 bytes, 4 levels
		Kind:     KindAuth,
		Progress: new(100),
		App:      strings.Repeat("a", 32),
		Title:    strings.Repeat("t", MaxTitleDecoded),
		Msg:      strings.Repeat("m", MaxMsgDecoded),
	}
	if len(r.ID) != MaxIDBytes {
		t.Fatalf("test setup: id len %d", len(r.ID))
	}
	seq, err := r.Encode(ST)
	if err != nil {
		t.Fatalf("max-size report rejected: %v", err)
	}
	if len(seq) > MaxSequenceBytes {
		t.Errorf("sequence %d bytes exceeds max", len(seq))
	}
	body := string(seq)
	if i := strings.Index(body, "msg="); i < 0 || len(strings.TrimSuffix(body[i+4:], "\x1b\\")) != MaxMsgEncoded {
		t.Errorf("encoded msg length mismatch")
	}
	if i := strings.Index(body, "title="); i < 0 || len(body[i+6:strings.Index(body, ":msg=")]) != MaxTitleEncoded {
		t.Errorf("encoded title length mismatch")
	}
}

func TestQuery(t *testing.T) {
	if got := string(Query(ST)); got != "\x1b]7501;?\x1b\\" {
		t.Errorf("got %q", got)
	}
}

func TestWrapTmux(t *testing.T) {
	got := string(WrapTmux([]byte("\x1b]7501;state=idle\x1b\\")))
	want := "\x1bPtmux;\x1b\x1b]7501;state=idle\x1b\x1b\\\x1b\\"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestSanitizeText(t *testing.T) {
	tests := []struct {
		in   string
		max  int
		want string
	}{
		{"a\r\nb\tc", 100, "a b c"},
		{"  x\n\n\ny  ", 100, "x y"},
		{"a\x1b[31mb", 100, "a[31mb"},
		{"a\xffb", 100, "a�b"},
		{"あいう", 7, "あい"}, // 3 bytes each; must not split
		{"abc", 3, "abc"},
	}
	for _, tt := range tests {
		got := SanitizeText(tt.in, tt.max)
		if got != tt.want {
			t.Errorf("SanitizeText(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
		}
		if err := validateText("msg", got, tt.max); err != nil {
			t.Errorf("sanitized text still invalid: %v", err)
		}
	}
}
