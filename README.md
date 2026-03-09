![GoGogot](https://octagon-lab.sfo3.cdn.digitaloceanspaces.com/gogogot.jpg)

# GoGogot — Lightweight OpenClaw Written in Go

[![Go Version](https://img.shields.io/github/go-mod/go-version/aspasskiy/GoGogot?style=flat-square)](https://go.dev)
[![License](https://img.shields.io/github/license/aspasskiy/GoGogot?style=flat-square)](LICENSE)
[![Stars](https://img.shields.io/github/stars/aspasskiy/GoGogot?style=flat-square)](https://github.com/aspasskiy/GoGogot/stargazers)
[![Lines of code](https://img.shields.io/badge/lines-~4%2C500-blue?style=flat-square)](#)
[![Docker](https://img.shields.io/docker/pulls/octagonlab/gogogot?style=flat-square)](#quick-start)

A **lightweight, extensible, and secure** open-source AI agent that lives on your server. It runs shell commands, edits files, browses the web, manages persistent memory, and schedules tasks — a self-hosted alternative to OpenClaw (Claude Code) in ~4,500 lines of Go.

- **Single binary, ~15 MB, ~10 MB RAM** — deploys with one `docker run` command
- **Your keys stay on your server** — no cloud account, no telemetry, no phoning home
- **You pick the model** — built-in aliases from budget to frontier, or pass any [OpenRouter](https://openrouter.ai) slug directly
- **Extensible** — clean Go interfaces (`Backend`, `Transport`, `Tool`) make it trivial to add providers, transports, or custom tools

## Quick Start

### Prerequisites

- Get a `TELEGRAM_BOT_TOKEN` by creating a new bot via [@BotFather](https://t.me/BotFather) on Telegram.
- Find your `TELEGRAM_OWNER_ID` (your personal Telegram user ID) using a bot like [@userinfobot](https://t.me/userinfobot). **This is critical for security** — it ensures only you can communicate with your agent.

### Docker

No git clone needed — the image is published on Docker Hub:

```bash
docker run -d --restart unless-stopped \
  --name gogogot \
  -e TELEGRAM_BOT_TOKEN=... \
  -e TELEGRAM_OWNER_ID=... \
  -e GOGOGOT_PROVIDER=openrouter \
  -e OPENROUTER_API_KEY=... \
  -e GOGOGOT_MODEL=deepseek \
  -v ./data:/data \
  -v ./work:/work \
  octagonlab/gogogot:latest
```

The image supports `linux/amd64` and `linux/arm64` and ships with a full Ubuntu environment (bash, git, Python, Node.js, ripgrep, sqlite, postgresql-client, and more).

<details>
<summary>Alternative: Docker Compose</summary>

```bash
curl -O https://raw.githubusercontent.com/aspasskiy/GoGogot/main/deploy/docker-compose.yml

# Create .env with your keys
cat > .env <<EOF
TELEGRAM_BOT_TOKEN=...
TELEGRAM_OWNER_ID=...
GOGOGOT_PROVIDER=openrouter
OPENROUTER_API_KEY=...
GOGOGOT_MODEL=deepseek
EOF

docker compose up -d
```

</details>

<details>
<summary>Local development (without Docker)</summary>

Requires Go 1.25+:

```bash
make generate          # fetch OpenRouter model catalog
go run ./cmd
```

</details>

## Choosing a Model

Set the model via `GOGOGOT_MODEL` env var or `--model` CLI flag.

> **Provider and model are required.** Set `GOGOGOT_PROVIDER` (`anthropic` or `openrouter`), `GOGOGOT_MODEL`, and the corresponding API key (`ANTHROPIC_API_KEY` or `OPENROUTER_API_KEY`). The agent will not start without all three.

Built-in aliases:

| Alias | Model slug |
|-------|-----------|
| `claude` | `claude-sonnet-4-6` (Anthropic) / `anthropic/claude-sonnet-4.6` (OpenRouter) |
| `deepseek` | `deepseek/deepseek-v3.2` |
| `gemini` | `google/gemini-3-pro-preview` |
| `minimax` | `minimax/minimax-m2.5` |
| `qwen` | `qwen/qwen3.5-397b-a17b` |
| `llama` | `meta-llama/llama-4-maverick` |
| `kimi` | `moonshotai/kimi-k2.5` |

You can also pass any OpenRouter slug directly: `GOGOGOT_MODEL=openai/gpt-5.4`. All aliases are defined in [`llm/provider.go`](llm/provider.go).

Browse all available models: [OpenRouter Models](https://openrouter.ai/models) | Independent benchmarks: [PinchBench](https://pinchbench.com/)

## Features

27 built-in tools across 10 categories:

- **Telegram** — multi-chat, attachments, typing indicators
- **System** — bash, read/write/edit files, system info
- **Web** — Brave search, fetch pages, HTTP requests, downloads
- **Identity** — persistent `soul.md` / `user.md`, auto-evolving
- **Memory** — persistent markdown notes the agent manages itself
- **Skills** — reusable procedural knowledge
- **Task planning** — session-scoped checklist for multi-step work
- **Scheduling** — cron-based self-scheduling, persisted across restarts
- **Compaction** — automatic context compression near token limits
- **Multi-model** — 7 aliases + any OpenRouter slug
- **Observability** — structured events (LLM calls, tool runs, errors)

## Use Cases

- **Daily digest** — *"Find top 5 AI news, summarize each in 2 sentences, send me every morning at 9:00"*
- **Report generation** — *"Download sales data from this URL, calculate totals by region, generate a PDF report"*
- **File processing** — *"Take these 12 screenshots, merge them into a single PDF, and send the file back"*
- **Market research** — *"Search the web for pricing of competitors X, Y, Z and make a comparison table"*
- **Server monitoring** — *"Check disk and memory usage every hour, alert me if anything exceeds 80%"*
- **Data extraction** — *"Fetch this webpage, extract all email addresses and phone numbers into a CSV"*
- **Routine automation** — *"Every Friday at 18:00, pull this week's git commits and send me a changelog summary"*

## How It Works

The entire agent is a `for` loop. Call the LLM, execute tool calls, feed results back, repeat:

```go
func (a *Agent) Run(ctx context.Context, input []ContentBlock) error {
    a.messages = append(a.messages, userMessage(input))

    for {
        resp, err := a.llm.Call(ctx, a.messages, a.tools)
        if err != nil {
            return err
        }
        a.messages = append(a.messages, resp)

        if len(resp.ToolCalls) == 0 {
            break
        }

        results := a.executeTools(resp.ToolCalls)
        a.messages = append(a.messages, results)
    }
    return nil
}
```

Everything else — memory, scheduling, compaction, identity — is just tools the LLM can call inside this loop.

## Extending

GoGogot is designed to be extended without frameworks or plugin registries:

- Adding a new LLM backend (implement one `Backend` interface method)
- Adding a new transport like Discord or Slack (implement 3 `Transport` methods)
- Adding custom models via OpenRouter slug or code aliases

## License

MIT
