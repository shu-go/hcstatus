package main

import (
	"slices"
	"testing"
)

func TestTabIDFromHCGet(t *testing.T) {
	out := []byte(`{"address":"t2t","labels":{"herdr.seen":"5","herdr.space":"sp1","herdr.tab":"tb41ca","where":"x / 1"},"status":null}`)
	got, err := tabIDFromHCGet(out)
	if err != nil || got != "tb41ca" {
		t.Errorf("got %q, %v", got, err)
	}

	for _, bad := range []string{`{"labels":{}}`, `{"address":"t1"}`, `not json`} {
		if got, err := tabIDFromHCGet([]byte(bad)); err == nil {
			t.Errorf("%s: want error, got %q", bad, got)
		}
	}
}

func TestTabRenameArgs(t *testing.T) {
	tests := []struct {
		tab, name string
		want      []string
	}{
		{"tb1", "fix login", []string{"do", "tab.rename", `{"id":"tb1","name":"fix login"}`}},
		// Empty name must be sent explicitly (reset), never omitted (asks the person).
		{"tb1", "", []string{"do", "tab.rename", `{"id":"tb1","name":""}`}},
		{"tb1", `a"b\c 日本`, []string{"do", "tab.rename", `{"id":"tb1","name":"a\"b\\c 日本"}`}},
	}
	for _, tt := range tests {
		if got := tabRenameArgs(tt.tab, tt.name); !slices.Equal(got, tt.want) {
			t.Errorf("tabRenameArgs(%q, %q) = %q, want %q", tt.tab, tt.name, got, tt.want)
		}
	}
}
