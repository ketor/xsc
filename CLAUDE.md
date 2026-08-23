# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

XSC is a Go-based SSH session manager and SFTP file manager suite with two TUI tools:
- **xssh** — SSH session manager with Bubble Tea TUI and CLI interface
- **xftp** — dual-panel SFTP file manager with Bubble Tea TUI

Sessions are YAML files stored in `~/.xsc/sessions/`; the directory hierarchy becomes the tree structure in the TUI. Both tools support importing and decrypting SecureCRT, Xshell, and MobaXterm sessions.

## Build & Development Commands

```bash
make build          # Build xssh + xftp to ./build/
make build-xftp     # Build only xftp
make test           # Run all tests: go test -v ./...
make fmt            # Format code: go fmt ./...
make vet            # Static analysis: go vet ./...
make run            # Run xssh TUI mode
make run-xftp       # Run xftp TUI mode
make install        # Install both to /usr/local/bin
make deps           # Download dependencies without modifying go.mod/go.sum
```

Run a specific test:
```bash
go test -v ./internal/securecrt/... -run TestDecryptPasswordV2Synthetic
```

Full quality check: `make fmt && make vet && make test`

## Architecture

**Entry points**:
- `cmd/xssh/main.go` — xssh command dispatcher: `tui`, `list`, `connect`, `import-securecrt`, `import-xshell`, `import-mobaxterm`, `help`
- `cmd/xftp/main.go` — xftp command dispatcher: `tui`, `connect`, `help`

**Core packages** (all under `internal/`):

- **tui/** — xssh Bubble Tea model-view-update loop. Single file `tui.go` (~1300 LOC) containing the full TUI: multiple modes (normal, search, command, help, error), Vim-style keybindings defined in `KeyMap`/`DefaultKeyMap()`, virtual scrolling, tree rendering, details panel. Styles are defined at the top of the file. SSH connections launch via `tea.Exec()` which pauses the TUI.

- **xftp/** — xftp Bubble Tea TUI with dual-panel file manager. Key files:
  - `model.go` — main Model with modes (Selector, Normal, Search, Command, Help, Error, TransferResult, OverwriteConfirm, Confirm, Input)
  - `selector.go` — session selector (shared session tree with xssh TUI), supports `:pw` toggle, Vim navigation
  - `filepanel.go` — FilePanel for local/remote file listing with cursor, selection, filtering, Vim-style scrolling (j/k, Ctrl+u/d, Ctrl+f/b, gg/G)
  - `operations.go` — file operations: yank, paste (with overwrite confirmation), delete, mkdir, rename
  - `transfer.go` — TransferManager for async SFTP file transfers with progress tracking and stats
  - `keymap.go` — KeyMap with all keybindings
  - `messages.go` — Bubble Tea messages (ConnectedMsg, TransferCompleteMsg, TransferResultMsg, etc.)
  - `styles.go` — Gruvbox-themed lipgloss styles

- **session/** — `session.go` defines the `Session` struct (YAML-serialized) with three `AuthType` values: `password`, `key`, `agent`. Supports `AuthMethod` list for multi-auth (SecureCRT). `PasswordSource` field distinguishes decryption backends ("securecrt", "xshell", "mobaxterm"). `tree.go` implements `SessionNode` for hierarchical tree organization with expand/collapse, filtering, `LoadSecureCRTSessions()`, `LoadXShellSessions()`, and `LoadMobaXtermSessions()`.

- **ssh/** — Pure Go SSH client (`golang.org/x/crypto/ssh`) with multi-auth fallback: password, key, SSH Agent, keyboard-interactive. TOFU (Trust On First Use) host key verification using `knownhosts.KeyError`. Auto-discovers default SSH keys in `~/.ssh/`. 10-second connection timeout. Handles terminal raw mode and SIGWINCH for window resize. `Dial()` returns `*ssh.Client` for xftp SFTP usage; `Connect()` creates interactive terminal sessions for xssh.

- **securecrt/** — Parses SecureCRT `.ini` session files. Decrypts V2 passwords (prefix `02` uses SHA256+AES-256-CBC, prefix `03` uses bcrypt_pbkdf). Lazy decryption for performance. `bcrypt_pbkdf.go` has the custom key derivation.

- **xshell/** — Parses Xshell `.xsh` session files (INI format, UTF-16LE encoded). Decrypts passwords using RC4 with SHA256(masterPassword) as key, verified by SHA256 checksum. Auto-detects UTF-16LE BOM and encoding.

- **mobaxterm/** — Parses MobaXterm `.ini` config files. Reads `[Bookmarks]`/`[Bookmarks_N]` sections, extracts SSH sessions (type=0) with `%`-delimited fields. Decrypts Professional edition passwords using AES-CFB-8 with SHA512(masterPassword)[0:32] as key. Handles Windows-1252 encoding and MobaXterm special character escaping (`__DIEZE__`→`#`, etc.).

**Public package**: `pkg/config/` — global config singleton loaded from `~/.xsc/config.yaml`. Manages paths and settings for SecureCRT/Xshell/MobaXterm integration and SSH host key verification. `SSHConfig.StrictHostKey` is `*bool` — nil defaults to true (TOFU enabled).

## Test Coverage

Key test files:
- `internal/ssh/client_test.go` — SSH config building, host key callback, agent key listing
- `internal/xftp/selector_test.go` — selector creation, command matching/completion, password toggle, node rendering, cursor movement, search
- `internal/xftp/filepanel_test.go` — cursor movement, page scrolling (half/full), selection, filtering, formatting helpers
- `internal/xftp/operations_test.go` — yank/paste operations
- `internal/xftp/transfer_test.go` — transfer manager stats tracking
- `internal/securecrt/` — parser and password decryption (V2 prefix 02/03)
- `internal/xshell/` — parser and RC4 password decryption
- `internal/mobaxterm/` — parser and AES-CFB-8 password decryption
- `internal/tui/tui_test.go` — TUI model initialization
- `pkg/config/config_test.go` — config load/save, directory paths, known_hosts

## Code Conventions

- **Language**: Go 1.25+. Documentation and code comments are written in Chinese.
- **Import order**: stdlib, then third-party, then local (`github.com/ketor/xsc/...`)
- **Naming**: PascalCase for exported, camelCase for unexported. Acronyms stay uppercase (`SSH`, `TUI`, `CRT`).
- **YAML tags**: `yaml:"field_name,omitempty"` on struct fields; internal fields use `yaml:"-"`
- **Error handling**: wrap with `fmt.Errorf("context: %w", err)` — use `%w` not `%v`
- **File permissions**: session files saved as 0600
- **Specs**: Gherkin format in `specs/xssh.feature` — keep tests in sync with specs

## Extending the Codebase

- **New auth method**: add `AuthType` constant in `session.go` → add validation in `Session.Validate()` → implement in `ssh/client.go` → update TUI details panel
- **New CLI command (xssh)**: add case in `cmd/xssh/main.go` switch → implement handler → update `showHelp()`
- **New CLI command (xftp)**: add case in `cmd/xftp/main.go` switch → implement handler → update `showHelp()`
- **xssh TUI changes**: keybindings in `KeyMap`/`DefaultKeyMap()` in `tui/tui.go`, rendering in `View()` and `renderXxx()` methods, styles at top of `tui.go`
- **xftp TUI changes**: keybindings in `xftp/keymap.go`, modes in `xftp/model.go`, file operations in `xftp/operations.go`, panel rendering in `xftp/filepanel.go`, selector in `xftp/selector.go`, styles in `xftp/styles.go`
- **New xftp command mode command**: add to `executeCommand()` in `xftp/model.go`
- **New xftp selector command**: add to `selectorCommands` in `xftp/selector.go`, handle in `handleCommandKey()`
