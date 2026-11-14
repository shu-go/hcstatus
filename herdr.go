package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// herdrTerminalEnv is set by herdr-core in terminals it runs.
const herdrTerminalEnv = "HC_TERMINAL_ID"

// herdrTimeout bounds each herdr command so that hcstatus never hangs on them.
const herdrTimeout = 3 * time.Second

func herdrTerminal() string {
	return os.Getenv(herdrTerminalEnv)
}

// renameHerdrTab names the herdr-app tab that holds terminal to name.
// An empty name resets the tab to its number.
//
// The tab id is looked up explicitly: tab.rename without an id would rename
// the focused tab, which need not be ours.
func renameHerdrTab(terminal, name string) error {
	out, err := runHerdr("hc", "get", terminal, "--json")
	if err != nil {
		return fmt.Errorf("hc get %s: %w", terminal, err)
	}
	tab, err := tabIDFromHCGet(out)
	if err != nil {
		return err
	}

	if _, err := runHerdr("herdr-app", tabRenameArgs(tab, name)...); err != nil {
		return fmt.Errorf("herdr-app tab.rename: %w", err)
	}
	return nil
}

// tabRenameArgs passes the arguments as JSON so that an empty name is sent as
// "" (reset to its number). tab.rename without a name asks the person, so
// the name key is always present.
func tabRenameArgs(tab, name string) []string {
	args, _ := json.Marshal(struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}{tab, name})
	return []string{"do", "tab.rename", string(args)}
}

// tabIDFromHCGet extracts the herdr-app tab id from `hc get --json` output.
func tabIDFromHCGet(out []byte) (string, error) {
	var t struct {
		Labels map[string]string `json:"labels"`
	}
	if err := json.Unmarshal(out, &t); err != nil {
		return "", fmt.Errorf("hc get: %w", err)
	}
	tab := t.Labels["herdr.tab"]
	if tab == "" {
		return "", errors.New("terminal is not in a herdr-app tab (no herdr.tab label)")
	}
	return tab, nil
}

func runHerdr(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), herdrTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := errors.AsType[*exec.ExitError](err); ok && len(ee.Stderr) != 0 {
			return nil, fmt.Errorf("%w: %s", err, ee.Stderr)
		}
		if len(out) != 0 {
			// herdr-app reports errors as JSON on stdout.
			return nil, fmt.Errorf("%w: %s", err, out)
		}
		return nil, err
	}
	return out, nil
}
