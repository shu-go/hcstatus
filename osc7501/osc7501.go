// Package osc7501 builds and validates OSC 7501 (program status) escape sequences.
//
// Spec: https://gist.githubusercontent.com/mitchellh/7acae3abd8355c1c00287d67e96c913a/raw
//
// Terminals silently discard a whole report when it breaks any rule, so this
// package validates everything up front and reports what is wrong.
package osc7501

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// State is the value of the required "state" key.
type State string

const (
	StateIdle    State = "idle"
	StateWorking State = "working"
	StateDone    State = "done"
	StateBlocked State = "blocked"
	StateError   State = "error"
	StateClear   State = "clear"
)

// Kind is the value of the "kind" key. It is meaningful only with StateBlocked.
type Kind string

const (
	KindNone       Kind = ""
	KindPermission Kind = "permission"
	KindQuestion   Kind = "question"
	KindAuth       Kind = "auth"
)

// Terminator selects the string terminator of the sequence.
type Terminator int

const (
	// ST is ESC \ (0x1B 0x5C).
	ST Terminator = iota
	// BEL is 0x07.
	BEL
)

// Limits defined by the spec.
const (
	MaxSequenceBytes   = 4096
	MaxMsgDecoded      = 2048
	MaxMsgEncoded      = 2732
	MaxTitleDecoded    = 192
	MaxTitleEncoded    = 256
	MaxAppBytes        = 32
	MaxIDBytes         = 128
	MaxIDSegmentBytes  = 32
	MaxIDDepth         = 8
	MinProgress        = 0
	MaxProgress        = 100
	oscIntroducer      = "\x1b]"
	oscNumber          = "7501"
	terminatorST       = "\x1b\\"
	terminatorBEL      = "\x07"
	tmuxPassthroughPre = "\x1bPtmux;"
)

// Report is one status report. Each report fully replaces the record
// addressed by ID; omitted fields are cleared on the terminal side.
type Report struct {
	State State
	// ID is a "/"-separated path. Empty means the root record.
	ID   string
	Kind Kind
	// Progress is 0..100. nil means unknown. Only valid with working/blocked.
	Progress *int
	App      string
	Title    string
	Msg      string
}

// Validate checks r against the spec.
func (r Report) Validate() error {
	var errs []error

	switch r.State {
	case StateIdle, StateWorking, StateDone, StateBlocked, StateError, StateClear:
	case "":
		errs = append(errs, errors.New("state is required"))
	default:
		errs = append(errs, fmt.Errorf("unknown state %q", r.State))
	}

	if r.ID != "" {
		if err := validateID(r.ID); err != nil {
			errs = append(errs, err)
		}
	}

	switch r.Kind {
	case KindNone:
	case KindPermission, KindQuestion, KindAuth:
		if r.State != StateBlocked {
			errs = append(errs, fmt.Errorf("kind is only valid with state=blocked (state=%s)", r.State))
		}
	default:
		errs = append(errs, fmt.Errorf("unknown kind %q", r.Kind))
	}

	if r.Progress != nil {
		if *r.Progress < MinProgress || *r.Progress > MaxProgress {
			errs = append(errs, fmt.Errorf("progress must be %d..%d (got %d)", MinProgress, MaxProgress, *r.Progress))
		}
		if r.State != StateWorking && r.State != StateBlocked {
			errs = append(errs, fmt.Errorf("progress is only valid with state=working or blocked (state=%s)", r.State))
		}
	}

	if r.App != "" {
		if len(r.App) > MaxAppBytes {
			errs = append(errs, fmt.Errorf("app is %d bytes (max %d)", len(r.App), MaxAppBytes))
		}
		if !isToken(r.App) {
			errs = append(errs, fmt.Errorf("app %q contains characters outside [A-Za-z0-9_.+-]", r.App))
		}
	}

	if err := validateText("title", r.Title, MaxTitleDecoded); err != nil {
		errs = append(errs, err)
	}
	if err := validateText("msg", r.Msg, MaxMsgDecoded); err != nil {
		errs = append(errs, err)
	}

	if len(errs) != 0 {
		return errors.Join(errs...)
	}

	if n := len(r.sequence(ST)); n > MaxSequenceBytes {
		return fmt.Errorf("sequence is %d bytes (max %d)", n, MaxSequenceBytes)
	}
	return nil
}

// Body returns the pairs part ("state=...:msg=...") after validating r.
func (r Report) Body() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return r.body(), nil
}

// Encode returns the complete escape sequence after validating r.
func (r Report) Encode(t Terminator) ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return r.sequence(t), nil
}

func (r Report) body() string {
	pairs := []string{"state=" + string(r.State)}
	if r.ID != "" {
		pairs = append(pairs, "id="+r.ID)
	}
	if r.Kind != KindNone {
		pairs = append(pairs, "kind="+string(r.Kind))
	}
	if r.Progress != nil {
		pairs = append(pairs, "progress="+strconv.Itoa(*r.Progress))
	}
	if r.App != "" {
		pairs = append(pairs, "app="+r.App)
	}
	if r.Title != "" {
		pairs = append(pairs, "title="+base64.StdEncoding.EncodeToString([]byte(r.Title)))
	}
	if r.Msg != "" {
		pairs = append(pairs, "msg="+base64.StdEncoding.EncodeToString([]byte(r.Msg)))
	}
	return strings.Join(pairs, ":")
}

func (r Report) sequence(t Terminator) []byte {
	return wrap(r.body(), t)
}

// Query returns the support query sequence (OSC 7501 ; ? ST).
// A supporting terminal replies with the same body "?".
func Query(t Terminator) []byte {
	return wrap("?", t)
}

func wrap(body string, t Terminator) []byte {
	term := terminatorST
	if t == BEL {
		term = terminatorBEL
	}
	return []byte(oscIntroducer + oscNumber + ";" + body + term)
}

// WrapTmux wraps seq in a tmux DCS passthrough so that tmux forwards it to
// the outer terminal (requires `set -g allow-passthrough on`).
// Every ESC inside seq is doubled as tmux requires.
func WrapTmux(seq []byte) []byte {
	inner := strings.ReplaceAll(string(seq), "\x1b", "\x1b\x1b")
	return []byte(tmuxPassthroughPre + inner + terminatorST)
}

func validateID(id string) error {
	if len(id) > MaxIDBytes {
		return fmt.Errorf("id is %d bytes (max %d)", len(id), MaxIDBytes)
	}
	segs := strings.Split(id, "/")
	if len(segs) > MaxIDDepth {
		return fmt.Errorf("id has %d levels (max %d)", len(segs), MaxIDDepth)
	}
	for _, s := range segs {
		if s == "" {
			return fmt.Errorf("id %q has an empty segment", id)
		}
		if len(s) > MaxIDSegmentBytes {
			return fmt.Errorf("id segment %q is %d bytes (max %d)", s, len(s), MaxIDSegmentBytes)
		}
		if !isToken(s) {
			return fmt.Errorf("id segment %q contains characters outside [A-Za-z0-9_.+-]", s)
		}
	}
	return nil
}

// validateText checks free text (msg/title). The encoded limit follows from the
// decoded limit (base64 of N bytes is 4*ceil(N/3)), so only decoded is checked.
func validateText(name, s string, maxDecoded int) error {
	if s == "" {
		return nil
	}
	if len(s) > maxDecoded {
		return fmt.Errorf("%s is %d bytes (max %d)", name, len(s), maxDecoded)
	}
	if !utf8.ValidString(s) {
		return fmt.Errorf("%s is not valid UTF-8", name)
	}
	for i, c := range s {
		if IsControl(c) {
			return fmt.Errorf("%s contains control character U+%04X at byte %d", name, c, i)
		}
	}
	return nil
}

// IsControl reports whether c is forbidden in msg/title
// (U+0000–U+001F, U+007F, U+0080–U+009F).
func IsControl(c rune) bool {
	return c <= 0x1f || (0x7f <= c && c <= 0x9f)
}

func isToken(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case c == '_', c == '.', c == '+', c == '-':
		default:
			return false
		}
	}
	return true
}

// SanitizeText makes s usable as msg/title: tabs and line breaks become single
// spaces, other control characters are removed, invalid UTF-8 is replaced,
// and the result is truncated to maxBytes on a rune boundary.
func SanitizeText(s string, maxBytes int) string {
	s = strings.ToValidUTF8(s, "�")
	var b strings.Builder
	prevSpace := false
	for _, c := range s {
		switch {
		case c == '\t' || c == '\n' || c == '\r':
			if !prevSpace {
				b.WriteByte(' ')
			}
			prevSpace = true
			continue
		case IsControl(c):
			continue
		}
		prevSpace = c == ' '
		b.WriteRune(c)
	}
	return Truncate(strings.TrimSpace(b.String()), maxBytes)
}

// Truncate cuts s to at most maxBytes without splitting a rune.
func Truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
