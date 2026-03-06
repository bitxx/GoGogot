package event

import "time"

type Kind string

const (
	UserMessage Kind = "user_message"
	LLMStart    Kind = "llm_start"
	LLMResponse Kind = "llm_response"
	LLMStream   Kind = "llm_stream"
	ToolStart   Kind = "tool_start"
	ToolEnd     Kind = "tool_end"
	EvalRun     Kind = "eval_run"
	EvalResult  Kind = "eval_result"
	Compaction  Kind = "compaction"
	LoopWarning Kind = "loop_warning"
	Error       Kind = "error"
	Done        Kind = "done"
)

type Event struct {
	Timestamp time.Time
	Kind      Kind
	Source    string // "core-loop", "eval-loop", "compaction", "subagent:abc"
	Depth     int    // nesting level (0 = top agent, 1 = subagent, ...)
	Data      any
}

type Bus struct {
	subscribers []func(Event)
}

func NewBus() *Bus {
	return &Bus{
		subscribers: make([]func(Event), 0),
	}
}

func (eb *Bus) Emit(e Event) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	for _, sub := range eb.subscribers {
		sub(e)
	}
}

func (eb *Bus) Subscribe(fn func(Event)) {
	eb.subscribers = append(eb.subscribers, fn)
}
