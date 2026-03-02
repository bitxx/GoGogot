package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"gogogot/core/agent"
	"gogogot/infra/config"
	"gogogot/infra/llm"
	"gogogot/infra/logger"
	"gogogot/core/scheduler"
	"gogogot/core/store"
	"gogogot/tools/system"
	"gogogot/infra/transport/bridge"
	"gogogot/infra/transport/telegram"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	modelFlag := flag.String("model", "", "model: claude, minimax (default: first available)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if *modelFlag != "" {
		cfg.Model = *modelFlag
	}

	if !cfg.HasAnyProvider() {
		fmt.Fprintln(os.Stderr, "error: set ANTHROPIC_API_KEY or OPENROUTER_API_KEY in .env")
		os.Exit(1)
	}

	if err := logger.Init(cfg.DataDir, cfg.LogLevel); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	defer logger.Close()

	store.Init(cfg.DataDir)
	sched := scheduler.New(cfg.DataDir)
	if err := sched.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting scheduler: %v\n", err)
		os.Exit(1)
	}
	defer sched.Stop()

	provider, err := selectProvider(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	t, err := buildTransport(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	allTools := coreTools(cfg.BraveAPIKey, sched)
	allTools = append(allTools, bridge.TransportTools()...)
	reg := system.NewRegistry(allTools)

	client := llm.NewClient(*provider, reg.Definitions())
	agentCfg := agent.AgentConfig{
		SystemPrompt: agent.SystemPrompt(agent.PromptContext{
			TransportName: t.Name(),
			ModelLabel:    provider.Label,
		}),
		MaxTokens:  4096,
		Compaction: agent.DefaultCompaction(),
	}
	b := bridge.New(t, client, agentCfg, reg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Sofie is running [%s, %s]. Press Ctrl+C to stop.\n", t.Name(), provider.Label)
	if err := b.Run(ctx); err != nil && ctx.Err() == nil {
		slog.Error("bridge run error", "error", err)
	}
	fmt.Println("Shutting down.")
}

func selectProvider(cfg *config.Config) (*llm.Provider, error) {
	providers := llm.AvailableProviders(cfg)
	if len(providers) == 0 {
		return nil, fmt.Errorf("no LLM providers available")
	}
	if cfg.Model == "" {
		return &providers[0], nil
	}
	for i := range providers {
		if providers[i].ID == cfg.Model {
			return &providers[i], nil
		}
	}
	return nil, fmt.Errorf("unknown model %q", cfg.Model)
}

func buildTransport(cfg *config.Config) (*telegram.Transport, error) {
	switch cfg.Transport {
	case "telegram":
		if cfg.TelegramToken == "" {
			return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required for telegram transport")
		}
		if cfg.TelegramOwnerID == 0 {
			return nil, fmt.Errorf("TELEGRAM_OWNER_ID is required for telegram transport")
		}
		return telegram.New(cfg.TelegramToken, cfg.TelegramOwnerID)
	default:
		return nil, fmt.Errorf("unknown transport: %s", cfg.Transport)
	}
}
