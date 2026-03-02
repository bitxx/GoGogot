package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
)

var logFile *os.File

type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			_ = h.Handle(ctx, r)
		}
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	hs := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		hs[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: hs}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	hs := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		hs[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: hs}
}

func newConsoleHandler() slog.Handler {
	styles := log.DefaultStyles()

	styles.Timestamp = lipgloss.NewStyle().Faint(true)
	styles.Prefix = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	styles.Message = lipgloss.NewStyle()
	styles.Key = lipgloss.NewStyle().Faint(true)
	styles.Separator = lipgloss.NewStyle().Faint(true)

	styles.Levels[log.DebugLevel] = lipgloss.NewStyle().
		SetString("DBG").
		Bold(true).
		MaxWidth(4).
		Foreground(lipgloss.Color("63"))

	styles.Levels[log.InfoLevel] = lipgloss.NewStyle().
		SetString("INF").
		Bold(true).
		MaxWidth(4).
		Foreground(lipgloss.Color("86"))

	styles.Levels[log.WarnLevel] = lipgloss.NewStyle().
		SetString("WRN").
		Bold(true).
		MaxWidth(4).
		Foreground(lipgloss.Color("192"))

	styles.Levels[log.ErrorLevel] = lipgloss.NewStyle().
		SetString("ERR").
		Bold(true).
		MaxWidth(4).
		Foreground(lipgloss.Color("204"))

	styles.Levels[log.FatalLevel] = lipgloss.NewStyle().
		SetString("FTL").
		Bold(true).
		MaxWidth(4).
		Foreground(lipgloss.Color("134"))

	l := log.NewWithOptions(os.Stderr, log.Options{
		Level:           log.DebugLevel,
		ReportTimestamp: true,
		TimeFormat:      "15:04:05",
		Prefix:          "gogogot",
	})
	l.SetStyles(styles)

	return l
}

func Init(dataDir, logLevel string) error {
	dir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("logger: mkdir: %w", err)
	}

	name := fmt.Sprintf("gogogot-%s.log", time.Now().Format("2006-01-02"))
	path := filepath.Join(dir, name)

	var err error
	logFile, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("logger: open: %w", err)
	}

	fileLevel := parseLevel(logLevel)

	fileHandler := slog.NewTextHandler(logFile, &slog.HandlerOptions{
		Level: fileLevel,
	})

	slog.SetDefault(slog.New(&multiHandler{
		handlers: []slog.Handler{fileHandler, newConsoleHandler()},
	}))

	slog.Info("logger initialized", "path", path, "file_level", fileLevel.String())
	return nil
}

func Close() {
	if logFile != nil {
		_ = logFile.Sync()
		_ = logFile.Close()
	}
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
