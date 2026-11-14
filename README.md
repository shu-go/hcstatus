# hcstatus

Report program status to the terminal via [OSC 7501](https://mitchellh.com/writing/program-status-osc7501).

Terminals that do not support OSC 7501 ignore the sequences.

## Install

```
go install github.com/shu-go/hcstatus@latest
```

## Usage

```
hcstatus working [--progress N] [MESSAGE...]
hcstatus blocked [--kind permission|question|auth] [MESSAGE...]
hcstatus idle | done | error [MESSAGE...]
hcstatus clear [--id ID]                 # without --id: all records
hcstatus run [OPTIONS] -- COMMAND [ARGS...]
```

Common options: `--id build/test`, `--app NAME`, `--title TEXT`, `--msg TEXT`.

Global options go **before** the subcommand:

| option | |
|---|---|
| `--out tty\|stdout\|stderr` | default `tty` (`/dev/tty` / `CONOUT$`), so redirecting stdout does not mix sequences into data. When there is no terminal, the report is silently skipped. |
| `--bel` | terminate with BEL instead of `ESC \` |
| `--tmux auto\|on\|off` | wrap in tmux DCS passthrough (`auto`: when `$TMUX` is set; needs `allow-passthrough on`) |
| `-v` | diagnostics on stderr |

### run

```
hcstatus run -- make -j4 all
```

Reports `working`, then `done` (exit 0), `error` (non-zero), or `idle` (interrupted).
Exits with the command's status (127 if it could not be started).
`--` is required when the command has options.

### Examples

```sh
hcstatus working -p 40 Syncing photos
hcstatus --out stdout done "Build finished" | od -c   # inspect the bytes
```

## Notes

- Messages are sanitized: line breaks become spaces, other control characters are removed, text is truncated to the spec limits. Other violations (bad `--id`, `--app`, `--progress`, ...) are errors, because terminals silently drop invalid reports.
- herdr: when `HC_TERMINAL_ID` is set, `--title` is also sent as the window title (OSC 2) before the report, because herdr-app's agents list shows the terminal title but not the OSC 7501 `title`/`msg`.
  With `--tab` (all subcommands except `clear`), the herdr-app tab holding this terminal is also renamed to `--title` (`hc get` for the tab id, then `herdr-app do tab.rename`); any failure is ignored (`-v` shows why). A named tab shows its name instead of the window title in the agents list. `clear` resets the tab name (to its number) whenever `HC_TERMINAL_ID` is set, without `--tab`.
- Claude Code hooks cannot send OSC 7501 (not in the `terminalSequence` allowlist, and hooks have no controlling terminal).
- The `osc7501` package can be used on its own.
