# chloe

[![Go Version](https://img.shields.io/github/go-mod/go-version/fractalops/chloe)](https://go.dev/)
[![License](https://img.shields.io/github/license/fractalops/chloe)](LICENSE)
[![Release](https://img.shields.io/github/v/release/fractalops/chloe)](https://github.com/fractalops/chloe/releases)

A beautiful TUI to spy on all your [Claude Code](https://docs.anthropic.com/en/docs/claude-code) sessions.

![demo](demo.gif)

## Features

- **Process monitoring** — CPU, memory, token burn rate, and open files for active sessions
- **Conversation viewer** — browse any session's messages with rendered markdown, expand to full content
- **Resume sessions** — jump back into any session directly from the TUI
- **Fuzzy search** — filter across project names, messages, and session IDs
- **Session stats** — token usage, model, turn count, and duration

## Install

```bash
# Quick install (Linux/macOS)
curl -sSL https://raw.githubusercontent.com/fractalops/chloe/main/install.sh | bash

# From source
git clone https://github.com/fractalops/chloe.git && cd chloe && make install

# Go install
go install github.com/fractalops/chloe@latest
```

## Usage

```bash
chloe
```

## Acknowledgments

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Bubbles](https://github.com/charmbracelet/bubbles),
[Lip Gloss](https://github.com/charmbracelet/lipgloss),
and [Glamour](https://github.com/charmbracelet/glamour).

## License

[MIT](LICENSE)
