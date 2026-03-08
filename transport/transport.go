package transport

import "context"

const (
	CmdNewChat    = "new_chat"
	CmdSwitchChat = "switch_chat"
	CmdStop       = "stop"
)

type Command struct {
	Name   string
	Args   map[string]string
	Result *CommandResult
}

type CommandResult struct {
	Data  map[string]string
	Error error
}

type Message struct {
	ChannelID   string
	Text        string
	Attachments []Attachment
	Command     *Command
}

type Attachment struct {
	Filename string
	MimeType string
	Data     []byte
}

type Handler func(ctx context.Context, msg Message)

// Transport is the core interface every communication channel must implement.
type Transport interface {
	Name() string
	Run(ctx context.Context, handler Handler) error
	SendText(ctx context.Context, channelID string, text string) error
}

// FileSender is implemented by transports that can deliver files.
type FileSender interface {
	SendFile(ctx context.Context, channelID, path, caption string) error
}

// TypingNotifier is implemented by transports that support typing indicators.
type TypingNotifier interface {
	SendTyping(ctx context.Context, channelID string) error
}

// Phase represents a high-level stage of the agent's work.
type Phase string

const (
	PhaseThinking Phase = "thinking"
	PhasePlanning Phase = "planning"
	PhaseTool     Phase = "tool"
)

// AgentStatus carries structured information about what the agent is doing.
// Each transport renders it in its own style (emoji, spinner, animation, etc.).
type AgentStatus struct {
	Phase  Phase
	Tool   string // raw tool name (empty when not in tool phase)
	Detail string // human-readable label: "Editing file", "go build", etc.
}

// StatusUpdater is implemented by transports that can post/edit/delete status messages.
type StatusUpdater interface {
	SendStatus(ctx context.Context, channelID string, status AgentStatus) (statusID string, err error)
	UpdateStatus(ctx context.Context, channelID, statusID string, status AgentStatus) error
	DeleteStatus(ctx context.Context, channelID, statusID string) error
}
