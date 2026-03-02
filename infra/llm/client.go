package llm

import (
	"context"
	"log/slog"

	anthpkg "gogogot/infra/llm/anthropic"
	oaipkg "gogogot/infra/llm/openai"
)

type (
	Message      = anthpkg.Message
	Response     = anthpkg.Response
	ToolDef      = anthpkg.ToolDef
	ContentBlock = anthpkg.ContentBlock
)

type LLM interface {
	Call(ctx context.Context, messages []Message, opts CallOptions) (*Response, error)
	ModelID() string
	ModelLabel() string
	ContextWindow() int
}

type CallOptions struct {
	System       string
	SystemAppend string
	Memory       string
	NoTools      bool
}

type Client struct {
	anth     *anthpkg.Backend
	oai      *oaipkg.Backend
	model    string
	tools    []ToolDef
	provider Provider
}

func NewClient(p Provider, toolDefs []ToolDef) *Client {
	c := &Client{
		model:    p.Model,
		tools:    toolDefs,
		provider: p,
	}

	switch p.Format {
	case "openai":
		c.oai = oaipkg.NewBackend(p.BaseURL, p.APIKey)
	default:
		c.anth = anthpkg.NewBackend(p.APIKey, p.BaseURL)
	}

	return c
}

func (c *Client) ModelID() string {
	return c.model
}

func (c *Client) ModelLabel() string {
	return c.provider.Label
}

func (c *Client) ContextWindow() int {
	return c.provider.ContextWindow
}

func (c *Client) Call(ctx context.Context, messages []Message, opts CallOptions) (*Response, error) {
	sys := opts.System
	if opts.SystemAppend != "" {
		sys += "\n\n" + opts.SystemAppend
	}
	if opts.Memory != "" {
		sys += "\n\n## Current Memory\n" + opts.Memory
	}

	var tools []ToolDef
	if !opts.NoTools {
		tools = c.tools
	}

	slog.Debug("llm.Call start",
		"model", c.model,
		"messages", len(messages),
		"tools_enabled", !opts.NoTools,
		"tools_count", len(tools),
		"has_memory", opts.Memory != "",
		"backend", c.provider.Format,
	)

	if c.oai != nil {
		return c.oai.Call(ctx, c.model, sys, messages, tools, 4096)
	}

	return c.anth.Call(ctx, c.model, sys, messages, tools, 4096)
}
