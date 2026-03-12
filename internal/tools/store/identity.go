package store

import (
	"context"
	"fmt"
	"gogogot/internal/tools/types"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/rs/zerolog/log"
)

func (s *Store) IdentityTools(onTimezoneChange func(*time.Location)) []types.Tool {
	return []types.Tool{
		{
			Name:  "soul_read",
			Label: "Reading identity",
			Description: "Read your soul.md — your personality, values, and behavioral rules. This file defines who you are across all conversations.",
			Parameters:  map[string]any{},
			Handler: func(_ context.Context, _ map[string]any) types.Result {
				content := s.ReadSoul()
				if content == "" {
					return types.Result{Output: "(soul.md is empty — define your identity)"}
				}
				return types.Result{Output: content}
			},
		},
		{
			Name:  "soul_write",
			Label: "Updating identity",
			Description: "Write or update your soul.md — your identity file. Define your personality traits, communication style, core values, and behavioral rules. Read first with soul_read before updating to avoid losing information.",
			Parameters: map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": "The full content for soul.md in markdown format",
				},
			},
			Required: []string{"content"},
			Handler: func(_ context.Context, input map[string]any) types.Result {
				content, err := types.GetString(input, "content")
				if err != nil {
					return types.ErrResult(err)
				}
				if err := s.WriteSoul(content); err != nil {
					return types.Result{Output: "error writing soul.md: " + err.Error(), IsErr: true}
				}
				return types.Result{Output: fmt.Sprintf("soul.md updated (%d bytes)", len(content))}
			},
		},
		{
			Name:  "user_read",
			Label: "Reading user profile",
			Description: "Read your user.md — everything you know about your owner. This file is loaded into your context automatically.",
			Parameters:  map[string]any{},
			Handler: func(_ context.Context, _ map[string]any) types.Result {
				content := s.ReadUser()
				if content == "" {
					return types.Result{Output: "(user.md is empty — learn about your owner)"}
				}
				return types.Result{Output: content}
			},
		},
		{
			Name:  "user_write",
			Label: "Updating user profile",
			Description: "Write or update your user.md — your owner's profile. Store their name, preferences, timezone, work context, communication style. Read first with user_read before updating to avoid losing information.",
			Parameters: map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": "The full content for user.md in markdown format",
				},
			},
			Required: []string{"content"},
			Handler: func(_ context.Context, input map[string]any) types.Result {
				content, err := types.GetString(input, "content")
				if err != nil {
					return types.ErrResult(err)
				}

				oldTZ := s.LoadTimezone()

				if err := s.WriteUser(content); err != nil {
					return types.Result{Output: "error writing user.md: " + err.Error(), IsErr: true}
				}

				newTZ := s.LoadTimezone()
				if newTZ.String() != oldTZ.String() && onTimezoneChange != nil {
					onTimezoneChange(newTZ)
					log.Info().Str("from", oldTZ.String()).Str("to", newTZ.String()).Msg("timezone changed via user_write")
				}

				return types.Result{Output: fmt.Sprintf("user.md updated (%d bytes)", len(content))}
			},
		},
	}
}

// --- Implementation ---

var tzRegex = regexp.MustCompile(`(?im)^.*timezone:\s*(\S+)`)

func (s *Store) LoadTimezone() *time.Location {
	if content := s.ReadUser(); content != "" {
		if m := tzRegex.FindStringSubmatch(content); len(m) > 1 {
			if loc, err := time.LoadLocation(m[1]); err == nil {
				return loc
			}
		}
	}
	if tz := os.Getenv("TZ"); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return time.UTC
}

func (s *Store) soulPath() string { return filepath.Join(s.dataDir, "soul.md") }
func (s *Store) userPath() string { return filepath.Join(s.dataDir, "user.md") }

func (s *Store) ReadSoul() string {
	data, err := os.ReadFile(s.soulPath())
	if err != nil {
		return ""
	}
	return string(data)
}

func (s *Store) WriteSoul(content string) error {
	return os.WriteFile(s.soulPath(), []byte(content), 0o644)
}

func (s *Store) ReadUser() string {
	data, err := os.ReadFile(s.userPath())
	if err != nil {
		return ""
	}
	return string(data)
}

func (s *Store) WriteUser(content string) error {
	return os.WriteFile(s.userPath(), []byte(content), 0o644)
}
