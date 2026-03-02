![GoGogot](https://octagon-lab.sfo3.cdn.digitaloceanspaces.com/gogogot.jpg)

# GoGogot — Lightweight Self-Hosted AI Agent Written in Go

[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/github/license/aspasskiy/GoGogot)](LICENSE)
[![Stars](https://img.shields.io/github/stars/aspasskiy/GoGogot?style=flat)](https://github.com/aspasskiy/GoGogot/stargazers)
[![Lines of code](https://img.shields.io/badge/lines%20of%20code-~4500-blue)](#)
[![Docker](https://img.shields.io/badge/deploy-docker%20compose-2496ED?logo=docker&logoColor=white)](#deployment)

An open-source, self-hosted personal AI agent written in Go. Deploy on your own server as a single ~15 MB binary — it runs shell commands, edits files, browses the web, manages persistent memory, and schedules tasks. A lightweight alternative to OpenClaw (Claude Code) in ~4,500 lines of Go. No frameworks, no plugins, no magic.

```
You (Telegram) → GoGogot → bash, files, web, memory, scheduler → You
```

## What is GoGogot?

GoGogot is an open-source AI agent that lives on your server. It can execute shell commands, read and write files, search and browse the web, maintain persistent memory, and run scheduled tasks — all powered by Claude or MiniMax. The entire agent is a single Go binary under 15 MB that idles at ~10 MB RAM and deploys with one `docker compose up` command.

## GoGogot vs OpenClaw (Claude Code) vs Nanobot


|                    | OpenClaw                               | Nanobot                     | GoGogot                |
| ------------------ | -------------------------------------- | --------------------------- | ---------------------- |
| Language           | TypeScript                             | Python                      | Go                     |
| Codebase           | ~430,000 LOC                           | ~4,000 LOC                  | ~4,500 LOC             |
| Dependencies       | 800+ npm packages                      | 30+ pip packages            | 7                      |
| Install size       | ~1 GB (Node.js + npm)                  | ~150 MB (Python + pip)      | ~15 MB (single binary) |
| RAM idle / working | ~450 MB / 2–8 GB (known memory leaks)  | ~100 MB / ~300 MB           | ~10 MB / ~30 MB        |
| Min requirements   | 2 GB RAM, 2 CPU cores                  | 512 MB RAM                  | Anything with Linux    |
| Deploy             | CLI wizard + daemon + Node >= 22       | `pip install` + config      | `docker compose up`    |
| Best for           | Feature completeness                   | Learning / research         | Production use         |


The bottleneck is always the LLM API call, not the agent runtime. GoGogot compiles to a single binary, runs on a $4/month VPS, and stays out of the way between requests.

## You Are In Control

Everything is configured explicitly via environment variables passed at deploy time. API keys never leave your server — there is no cloud account, no SaaS dashboard, no telemetry, no phoning home.


| Variable             | Purpose                                      |
| -------------------- | -------------------------------------------- |
| `ANTHROPIC_API_KEY`  | Claude (direct API)                          |
| `OPENROUTER_API_KEY` | MiniMax and other models via OpenRouter      |
| `GOGOGOT_MODEL`      | `claude` or `minimax` — you choose the model |
| `TELEGRAM_BOT_TOKEN` | Your Telegram bot                            |
| `TELEGRAM_OWNER_ID`  | Only this user can talk to the bot           |
| `BRAVE_API_KEY`      | Web search (optional)                        |


## Cost: MiniMax vs Claude

You pick the price/quality tradeoff. Both models work out of the box — switch with an env var or CLI flag: `--model=minimax`.


| Model                         | Input (per 1M tokens) | Output (per 1M tokens) |
| ----------------------------- | --------------------- | ---------------------- |
| MiniMax M2.5 (via OpenRouter) | $0.30                 | $1.10                  |
| Claude Opus 4.6               | $5.00                 | $25.00                 |
| Claude Opus 4                 | $15.00                | $75.00                 |


A typical agent session (~50K input, ~10K output):


| Model           | Cost per session |
| --------------- | ---------------- |
| MiniMax         | ~$0.03           |
| Claude Opus 4.6 | ~$0.50           |
| Claude Opus 4   | ~$1.50           |


For routine tasks — daily digests, file management, web lookups — MiniMax is more than enough. Switch to Claude for complex reasoning when you need it. One env var.

## Extensible by Design

LLM providers and chat transports are clean Go interfaces. Adding a new one = implementing a few methods. No plugin system, no registry, no YAML config.

**LLM provider** — 3 methods:

```go
type LLM interface {
    Call(ctx context.Context, messages []Message, opts CallOptions) (*Response, error)
    ModelLabel() string
    ContextWindow() int
}
```

Ships with: **Anthropic** (native SDK) and **OpenAI-compatible** (OpenRouter — MiniMax, etc.)

**Transport** — 3 methods:

```go
type Transport interface {
    Name() string
    Run(ctx context.Context, handler Handler) error
    SendText(ctx context.Context, channelID string, text string) error
}
```

Optional capabilities via extra interfaces: `FileSender`, `TypingNotifier`, `StatusUpdater`.

Ships with: **Telegram**. Want Discord, Slack, or Matrix? Implement 3 methods and plug it in.

## Features

- **Telegram** — multi-chat, attachments (images, documents), typing indicators
- **System access** — bash, read/write/edit files, system info
- **Web** — search (Brave), fetch pages, HTTP requests, download files
- **Memory** — persistent markdown files the agent reads and writes itself
- **Scheduling** — cron-based self-scheduling, persisted across restarts
- **Compaction** — automatic context compression when approaching token limits
- **Multi-model** — Claude and MiniMax, manually chosen, not auto-routed
- **Observability** — structured event system (LLM calls, tool executions, errors)

## Architecture

```
┌──────────┐     ┌────────┐     ┌───────┐     ┌─────┐
│ Telegram │────▸│ Bridge │────▸│ Agent │────▸│ LLM │
└──────────┘     └────────┘     └───┬───┘     └─────┘
                                    │
                                    ▼
                               ┌─────────┐
                               │  Tools  │
                               └─────────┘
                          bash, files, web,
                        memory, scheduler, ...
```

The entire agent is one loop:

```go
for {
    guardrails.Check()
    maybeCompact()

    response := llm.Call(session.Messages())

    if no tool calls in response {
        break // done, send final text to user
    }

    for each tool call {
        result := tools.Execute(name, input)
        session.Append(result)
    }
}
```

The LLM decides everything — when to plan, when to ask the user, when to self-correct. These are prompt strategies, not code constructs. Adding a new behavior = editing the system prompt.

## Tools

17 built-in tools across 5 categories:

- **System** — `bash`, `read_file`, `write_file`, `edit_file`, `list_files`, `system_info`
- **Web** — `web_search` (Brave), `web_fetch`, `web_request`, `web_download`
- **Memory** — `memory_list`, `memory_read`, `memory_write`
- **Scheduling** — `schedule_add`, `schedule_list`, `schedule_remove`
- **Transport** — `send_file`



## Installation & Quick Start

```bash
git clone https://github.com/aspasskiy/GoGogot.git
cd GoGogot

# Configure
cp .env.example .env
# Edit .env: set TELEGRAM_BOT_TOKEN, TELEGRAM_OWNER_ID, API keys

# Run
docker compose -f deploy/docker-compose.yml up -d
```

The Docker image ships with a full Ubuntu environment: bash, git, Python, Node.js, ripgrep, sqlite, postgresql-client, and more.

## Philosophy

From the [orchestration spec](ORCHESTRATION_SPEC.md):

> **Simplicity** — one loop, good tools, smart prompt. Complexity is added only when metrics prove it helps.

> **LLM-first** — the LLM decides what to do, when to plan, when to critique its own work. The system executes, not orchestrates.

The previous spec had 10 orchestration patterns and 10 recipes as Go code structures. They were removed because a simple loop with good tools and prompts consistently outperforms complex orchestration frameworks. The only structural pattern retained is the **eval loop** — because external verification (tests, linter) requires actual code to check results and retry.

## Tech Stack

- **Go 1.25** — compiled, concurrent, zero-dependency runtime
- [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go) — native Claude API
- [openai-go](https://github.com/openai/openai-go) — OpenRouter / OpenAI-compatible providers
- [telegram-bot-api](https://github.com/go-telegram-bot-api/telegram-bot-api) — Telegram transport
- [robfig/cron](https://github.com/robfig/cron) — scheduler
- [uber/fx](https://github.com/uber-go/fx) — dependency injection
- [goquery](https://github.com/PuerkitoBio/goquery) — HTML parsing for web tools

## License

MIT