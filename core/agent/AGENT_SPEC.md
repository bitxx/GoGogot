# Sofie — Agent Specification

## 1. Philosophy

Sofie is a general-purpose agent. The LLM is the orchestrator — it decides strategy, plans, and self-corrects. The system provides a runtime loop, tools, memory, and guardrails. Strategies live in the system prompt, not in code structures.

### Principles

- **Simplicity** — one loop, good tools, smart prompt. Complexity is added only when metrics prove it helps.
- **LLM-first** — the LLM decides what to do, when to plan, when to critique its own work. The system executes, not orchestrates.
- **Observability** — every turn emits structured events via a channel. The UI/bridge consumes them.
- **Fail-safe** — guardrails detect stuck loops, budget overruns, and destructive actions. Failures are signals, not crashes.
- **Persistence** — chat history in `store.Chat`. Context window managed by `orchestration.Session` with compaction.

---

## 2. Architecture

```
┌────────────────────────────────────────────────────┐
│                    Guardrails                       │
│  loop detection · budget · safety · compaction      │
│  ┌──────────────────────────────────────────────┐  │
│  │              Eval Loop (optional)             │  │  ┌──────────────┐
│  │  ┌────────────────────────────────────────┐  │  │  │              │
│  │  │            Core Loop                   │  │  │  │  Events →    │
│  │  │                                        │  │  │  │  chan Event → │
│  │  │  User message                          │  │  │  │  Bridge / UI │
│  │  │    ↓                                   │  │  │  │              │
│  │  │  LLM (system prompt + context + tools) │  │  │  └──────────────┘
│  │  │    ↓                                   │  │  │
│  │  │  Tool calls? ──yes──→ Execute tools    │  │  │
│  │  │    │                    ↓              │  │  │
│  │  │    │              Tool results → LLM   │  │  │
│  │  │    │                    ↓              │  │  │
│  │  │    no                 (repeat)         │  │  │
│  │  │    ↓                                   │  │  │
│  │  │  Final response → User                 │  │  │
│  │  └────────────────────────────────────────┘  │  │
│  │                                               │  │
│  │  Evaluate (external: tests/linter/script)     │  │
│  │  Passed? → done. Failed? → inject feedback,   │  │
│  │  compact context, retry.                      │  │
│  └──────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────┘
```

Three layers, each optional:

| Layer | What it does | When needed |
|---|---|---|
| **Core Loop** | `while(tool_calls) { call LLM → exec tools }` | Always |
| **Eval Loop** | Core Loop + external evaluator + retry with feedback | Tasks with verifiable outcome (tests, build, linter) |
| **Guardrails** | Loop detection, budget, safety gates, compaction | Always (configurable) |

---

## 3. Core Loop

The engine. One `for` loop that sends messages to the LLM and executes tool calls.

```go
// agent/run.go (simplified — omits attachment handling)
func (a *Agent) Run(ctx context.Context, task string, attachments ...transport.Attachment) error {
    a.session.Append(orchestration.Message{
        Role:    "user",
        Content: []anthropic.ContentBlock{anthropic.TextBlock(task)},
    })

    for iteration := 1; ; iteration++ {
        if err := a.guardrails.Check(ctx, a.session); err != nil {
            return err
        }
        if err := a.maybeCompact(ctx); err != nil {
            slog.Error("compaction failed", "error", err)
        }

        a.emit(orchestration.EventLLMStart, nil)

        msgs := convertToAnthropicMessages(a.session.Messages())
        resp, err := a.client.Call(ctx, msgs, llm.CallOptions{
            System: a.config.SystemPrompt,
        })
        if err != nil {
            return err
        }

        a.session.Append(orchestration.Message{
            Role:    "assistant",
            Content: resp.Content,
            Usage:   &orchestration.Usage{InputTokens: resp.InputTokens, OutputTokens: resp.OutputTokens},
        })

        if len(toolCalls) == 0 {
            break
        }

        for _, tc := range toolCalls {
            a.emit(orchestration.EventToolStart, map[string]any{"name": tc.ToolName})
            result := a.registry.Execute(ctx, tc.ToolName, input)
            a.session.Append(orchestration.Message{
                Role:    "user",
                Content: []anthropic.ContentBlock{anthropic.ToolResultBlock(tc.ToolUseID, result.Output, result.IsErr)},
            })
            a.emit(orchestration.EventToolEnd, map[string]any{"name": tc.ToolName, "result": result.Output})
        }
    }
    return nil
}
```

The LLM sees: system prompt + session history + tool definitions. It decides everything — when to plan, when to ask the user, when to self-critique. These are prompt-level strategies, not code constructs.

### Dual Persistence

The agent maintains two parallel stores:
- **`orchestration.Session`** — in-memory message list for the LLM context window (compactable).
- **`store.Chat`** — persistent chat record with full text history (survives restarts).

### System Prompt Strategies

The system prompt includes behavioral guidance that replaces hardcoded patterns:

```
You are Sofie, a personal AI agent running on an Ubuntu server.
...
## How to work
- For complex tasks: break them down into steps before acting.
- After completing work: review your output for correctness.
- When uncertain: ask the user rather than guessing.
- When stuck: try a different approach instead of repeating.
```

Adding a new strategy = editing the prompt. Not writing Go code.

---

## 4. Tools

Tools are how the agent interacts with the world. Tool design is the highest-leverage investment — 80% of the agent's context comes from tool inputs and outputs.

### Interface

```go
// tools/tool.go
type Result struct {
    Output string
    IsErr  bool
}

type Handler func(ctx context.Context, input map[string]any) Result

type Tool struct {
    Name        string
    Description string
    Parameters  map[string]any   // JSON Schema properties
    Required    []string
    Handler     Handler
}
```

### Registry

```go
// tools/system/registry.go
type Registry struct {
    tt map[string]tools.Tool
}

func NewRegistry(tt []tools.Tool) *Registry
func (r *Registry) Execute(ctx context.Context, name string, input map[string]any) tools.Result
func (r *Registry) Definitions() []anthropic.ToolDef
func (r *Registry) Register(t tools.Tool)
```

The registry converts tools to `anthropic.ToolDef` for the LLM call and dispatches execution by name.

### Design Rules

1. **Purpose-built** — each tool does one thing. Don't expose raw APIs.
2. **Clean output** — format results for the LLM, not for humans. Concise, relevant.
3. **Absolute paths** — avoid ambiguity.
4. **Self-documenting** — descriptions are prompts. Write them as if explaining to a junior developer.
5. **Error-informative** — error messages should tell the LLM what went wrong and how to fix it.

### Subagents as Tools (future)

A subagent is a tool that spawns another Core Loop with a different system prompt and tool set. Not yet implemented — the architecture supports it naturally since `Agent` is self-contained.

---

## 5. Session & Memory

### Session (in-memory context)

The session tracks messages for the LLM context window. It lives in `agent/orchestration/session.go`.

```go
type Message struct {
    Role      string // "user" | "assistant"
    Content   []anthropic.ContentBlock
    Timestamp time.Time
    Usage     *Usage
    Metadata  map[string]any
}

type Session struct {
    ID         string
    FilePath   string
    messages   []Message
    TotalUsage Usage
}

func NewSession(id, filePath string) *Session
func (s *Session) Append(msg Message)
func (s *Session) Messages() []Message
func (s *Session) CompactAll(reason string)
```

### Chat Persistence (store.Chat)

Durable chat history is managed separately by `store.Chat`. The agent writes to both `Session` (for context) and `Chat` (for persistence) during `Run`.

### Soul (Long-Term Memory)

Soul is persistent cross-session memory. Stored as markdown files under the configured memory directory. The agent reads and writes soul via tools.

The agent has memory tools in `tools/system/memory.go`:

| Tool | What it does |
|---|---|
| `memory_list` | List available memory files |
| `memory_read` | Read a memory file |
| `memory_write` | Write or update a memory file |

System prompt instructs the agent to check memory before answering questions about prior work, preferences, or decisions. Memory is not auto-injected into context — the agent pulls what it needs.

### Skills (Procedural Knowledge)

Skills are reusable procedural knowledge — workflows, integrations, deployment procedures — that the agent creates from experience and reuses in future conversations. Stored as directories under `{dataDir}/skills/`, each containing a `SKILL.md` file.

**Difference from Memory:** Memory stores facts and preferences ("owner likes X", "server runs Ubuntu 22"). Skills store procedures ("how to deploy Docker containers", "how to set up a cron monitoring pipeline"). Memory is pulled by the agent on-demand. Skills are matched via description in the system prompt.

#### Skill Format

Each skill is a directory with a `SKILL.md` containing YAML frontmatter and markdown instructions:

```
skills/
├── deploy-docker/
│   ├── SKILL.md
│   └── scripts/
│       └── deploy.sh
└── morning-digest/
    └── SKILL.md
```

```markdown
---
name: deploy-docker
description: "Deploy services with Docker Compose. Use when: user asks to deploy, update, or restart a Docker service."
---

# Deploy Docker

## Steps
1. Check docker-compose.yml exists ...
2. Run `docker compose up -d` ...
```

#### Progressive Disclosure (three-level loading)

Inspired by OpenClaw's architecture, skills use a three-level loading system to manage context efficiently:

| Level | What's in context | When | Size |
|---|---|---|---|
| **1. Metadata** | `name` + `description` per skill | Always (in system prompt `<available_skills>` block) | ~100 words per skill |
| **2. SKILL.md body** | Full instructions | Only when agent selects the skill via `skill_read` | <5k words |
| **3. Bundled resources** | scripts/, references/, assets/ | On demand, agent reads as needed | Unlimited |

The system prompt includes an `<available_skills>` XML block with name, description, and file path for each skill. The agent scans descriptions, picks the best match, reads the full SKILL.md, and follows the instructions.

#### Self-Learning

The system prompt instructs the agent to create skills after solving non-trivial problems. This is the "fine-tuning" mechanism — the agent accumulates procedural knowledge over time.

#### Skill Tools

Defined in `tools/system/skills.go`:

| Tool | What it does |
|---|---|
| `skill_list` | List all skills with name + description |
| `skill_read` | Read full SKILL.md content |
| `skill_create` | Create a new skill (name, description, body) |
| `skill_update` | Update an existing skill's SKILL.md |
| `skill_delete` | Remove a skill directory |

#### Implementation

- **Loading & parsing:** `skills/skills.go` — `LoadAll()` scans skill directories, `parseFrontmatter()` extracts YAML metadata, `FormatForPrompt()` builds the `<available_skills>` block.
- **Storage:** `store.SkillsDir()` returns `{dataDir}/skills/`.
- **Prompt injection:** `agent/prompts.go` — `loadSkillsBlock()` loads skills at prompt build time and injects the formatted block.

---

## 6. Compaction

When context approaches the model's window limit, compaction fires automatically. It's infrastructure — the agent doesn't know it's happening.

### How It Works

```
Context size exceeds threshold (e.g. 80% of window)
  │
  ▼
Split older messages into chunks by token count
  │
  ▼
Summarize each chunk via LLM ("preserve decisions, TODOs, constraints, errors")
  │
  ▼
Replace old messages with summary in session
  │
  ▼
Continue execution with compressed context
```

### Implementation

```go
// agent/orchestration/compaction.go
type CompactionConfig struct {
    Threshold      float64 // 0.0–1.0, fraction of context window that triggers compaction
    SafetyMargin   float64 // 1.2 = 20% buffer for token estimate inaccuracy
    PreserveRecent int     // number of recent messages to keep uncompressed
    SummaryPrompt  string  // instruction for the summarization LLM call
}

func (cc *CompactionConfig) ShouldCompact(estimatedTokens, contextWindow int) bool
```

The agent layer provides the summarizer function by wrapping an LLM call with `NoTools=true`:

```go
// agent/compaction.go
func (a *Agent) maybeCompact(ctx context.Context) error {
    estimated := orchestration.EstimateTokens(a.session.Messages())
    if !a.config.Compaction.ShouldCompact(estimated, a.client.ContextWindow()) {
        return nil
    }
    return a.session.Compact(ctx, a.config.Compaction, a.summarize)
}
```

Compaction is triggered **automatically before each LLM call** if context exceeds threshold.

The summary preserves: decisions made, errors encountered, current plan, open questions, file paths mentioned, TODOs. It discards: verbose tool outputs, intermediate reasoning, superseded drafts.

### Token Estimation

```go
// agent/orchestration/tokens.go
const charsPerToken = 4

func EstimateTokens(messages []Message) int
```

Uses ~4 characters per token heuristic. Sufficient for compaction threshold decisions when combined with `SafetyMargin`.

### Eval Loop Compaction

When the Eval Loop retries after a failed evaluation, it uses `CompactAll` — replacing the entire failed attempt with a summary, starting fresh with just the task + feedback. This prevents error context from polluting the next attempt.

---

## 7. Guardrails

> **Status: partially implemented.** The guardrails framework exists but most detectors are stubs. The `Check` method currently returns nil. Designs below represent the target architecture.

Guardrails run at every iteration of the Core Loop. They protect against runaway execution.

### Loop Detection (TODO)

Detects when the agent is stuck repeating the same actions. Three planned detectors:

| Detector | What it catches | Threshold |
|---|---|---|
| **Repeat** | Same tool + same params called N times | Warning: 10, Critical: 20 |
| **No-Progress** | Polling tool returns identical results | Warning: 10, Critical: 20 |
| **Ping-Pong** | Alternating between two tool calls with no progress | Warning: 10, Critical: 20 |

```go
// agent/orchestration/guardrails.go
type LoopDetector struct {
    History []ToolCallRecord // sliding window, last 30 calls
}

type ToolCallRecord struct {
    ToolName   string
    ArgsHash   string
    ResultHash string
    Timestamp  time.Time
}
```

On warning: inject a message into context telling the LLM it's stuck. On critical: abort the turn.

### Budget Check (TODO)

Not yet implemented. Planned:

```go
type BudgetConfig struct {
    MaxTokens    int
    MaxCost      float64          // USD
    MaxToolCalls int
    MaxDuration  time.Duration
}
```

### Safety Gates

```go
type SafetyPolicy struct {
    RequireConfirmation []string // tool names that need user approval
    DenyList            []string // tools that are blocked entirely
    ReadOnly            bool     // if true, block all write tools
}
```

The struct exists and is passed through `AgentConfig`, but enforcement logic in `Guardrails.Check` is not yet wired up.

---

## 8. Eval Loop

Optional wrapper around the Core Loop for tasks with a verifiable outcome. This is the only "recipe" — everything else is prompt-level.

```go
// agent/eval.go
func (a *Agent) RunWithEval(ctx context.Context, task string, eval orchestration.Evaluator) error {
    maxIters := a.config.EvalIterations
    if maxIters <= 0 {
        maxIters = 1
    }

    for iter := 0; iter < maxIters; iter++ {
        err := a.Run(ctx, task)
        if err != nil {
            return err
        }

        if eval == nil {
            break
        }

        result := eval.Evaluate(ctx)
        a.emit(orchestration.EventEvalResult, map[string]any{
            "iteration": iter, "passed": result.Passed, "feedback": result.Feedback,
        })

        if result.Passed {
            return nil
        }

        a.session.CompactAll("attempt failed: " + result.Feedback)
        task = fmt.Sprintf("Previous attempt failed.\nFeedback: %s\nOriginal task: %s", result.Feedback, task)
    }
    return fmt.Errorf("eval loop: max iterations (%d) reached", maxIters)
}
```

```go
// agent/orchestration/eval.go
type Evaluator interface {
    Evaluate(ctx context.Context) EvalResult
}

type EvalResult struct {
    Passed   bool
    Score    float64
    Feedback string
    Details  map[string]any
}
```

Evaluator implementations (planned):
- **ScriptEvaluator** — runs a command (tests, linter, build). Exit 0 = passed.
- **LLMEvaluator** — asks LLM to evaluate the result against criteria.
- **CompositeEvaluator** — combines multiple evaluators.

---

## 9. Usage Tracking

Every LLM call reports token counts. Accumulated per-session.

```go
// agent/orchestration/usage.go
type Usage struct {
    InputTokens      int
    OutputTokens     int
    CacheReadTokens  int
    CacheWriteTokens int
    TotalTokens      int
    LLMCalls         int
    ToolCalls        int
    Cost             float64       // estimated USD
    Duration         time.Duration
}

func (u *Usage) Add(other Usage)
```

Usage is:
- Stored in each assistant `Message.Usage` in the session.
- Accumulated in `Session.TotalUsage`.
- Checked by budget guardrails (when implemented).

---

## 10. Event System

### Events

Every significant action emits a structured event via a buffered channel on the `Agent`.

```go
// agent/orchestration/events.go
type EventKind string

const (
    EventUserMessage EventKind = "user_message"
    EventLLMStart    EventKind = "llm_start"
    EventLLMResponse EventKind = "llm_response"
    EventLLMStream   EventKind = "llm_stream"
    EventToolStart   EventKind = "tool_start"
    EventToolEnd     EventKind = "tool_end"
    EventEvalRun     EventKind = "eval_run"
    EventEvalResult  EventKind = "eval_result"
    EventCompaction  EventKind = "compaction"
    EventLoopWarning EventKind = "loop_warning"
    EventError       EventKind = "error"
    EventDone        EventKind = "done"
)

type Event struct {
    Timestamp time.Time
    Kind      EventKind
    Source    string // "core-loop", "eval-loop", "compaction", "subagent:abc"
    Depth     int    // nesting level (0 = top agent, 1 = subagent, ...)
    Data      any
}
```

### Channel-Based Delivery

The agent emits events into a buffered channel (`chan orchestration.Event`, capacity 64). The bridge/UI goroutine reads from this channel via `range`:

```go
// agent/agent.go
type Agent struct {
    // ...
    Events chan orchestration.Event
}

func (a *Agent) emit(kind orchestration.EventKind, data any) {
    select {
    case a.Events <- orchestration.Event{...}:
    default:
        slog.Warn("agent event dropped — bus full", "kind", kind)
    }
}
```

The `transport/bridge` package consumes events to relay text responses and tool status to the user via the transport layer (Telegram, etc.).

### Event → UI Mapping

| Event | Bridge/UI action |
|---|---|
| `llm_stream` | Collect final text response |
| `tool_start` | Update status message ("Running bash...") |
| `tool_end` | Log result |
| `eval_result` | Pass/fail with feedback |
| `compaction` | Log token reduction |
| `loop_warning` | Warning indicator |
| `error` | Send error to user |
| `done` | Deliver final text, clean up status |

---

## 11. Configuration

```go
// agent/agent.go
type AgentConfig struct {
    SystemPrompt   string
    Model          string
    MaxTokens      int
    Tools          []string
    Safety         orchestration.SafetyPolicy
    Compaction     orchestration.CompactionConfig
    EvalIterations int
}
```

---

## 12. Extensibility

### Adding a New Tool

1. Create a `tools.Tool` struct (handler + description + JSON schema parameters).
2. Add it to the tool list passed to `system.NewRegistry()`.
3. Available immediately to the agent.

### Adding a New Evaluator

1. Implement the `orchestration.Evaluator` interface.
2. Pass it to `agent.RunWithEval()`.

### Changing Agent Behavior

1. Edit the system prompt (`agent/prompts.go`). No code changes needed for strategy adjustments.

---

## Appendix: What Was Removed and Why

The previous spec had 10 patterns (ReAct, Plan-then-Execute, OODA, ...) and 10 recipes (Ralph Loop, Debate, Supervisor-Workers, ...) as Go code structures. This was removed because:

1. **The LLM is the orchestrator.** "Plan then execute" is what a good LLM does when prompted to break down tasks. "Critique and revise" is what it does when prompted to review its work. These are prompt strategies, not code.

2. **Industry consensus.** Anthropic, Cursor, OpenClaw, and the OpenAI Agents SDK all converge on the same architecture: a while loop with tools. Complex orchestration frameworks are consistently outperformed by simple loops with good tools and prompts.

3. **Compounding errors.** Each additional layer of orchestration compounds failure probability. "85-90% accuracy per tool call means four or five calls is a coin flip." Simpler is more reliable.

4. **Extensibility.** Adding a new strategy should mean writing a paragraph in the system prompt, not implementing a Go interface with descriptors, routing rules, and middleware.

The only structural loop retained is the **Eval Loop** — because external verification (tests, linter) requires actual code to check results and retry. Everything else is delegated to the LLM.
