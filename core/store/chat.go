package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Chat struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
}

func NewChat() *Chat {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return &Chat{
		ID:        hex.EncodeToString(b),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func (c *Chat) path() string {
	return filepath.Join(chatsDir(), c.ID+".json")
}

func (c *Chat) Save() error {
	if err := ensureDirs(); err != nil {
		return err
	}
	c.UpdatedAt = time.Now()
	if c.Title == "" && len(c.Messages) > 0 {
		c.Title = truncTitle(c.Messages[0].Content)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path(), data, 0o644)
}

func LoadChat(id string) (*Chat, error) {
	p := filepath.Join(chatsDir(), id+".json")
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("chat %q not found", id)
	}
	var c Chat
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

type ChatInfo struct {
	ID        string
	Title     string
	UpdatedAt time.Time
}

func ListChats() ([]ChatInfo, error) {
	if err := ensureDirs(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(chatsDir())
	if err != nil {
		return nil, err
	}
	var out []ChatInfo
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(chatsDir(), e.Name()))
		if err != nil {
			continue
		}
		var c Chat
		if json.Unmarshal(data, &c) == nil {
			out = append(out, ChatInfo{ID: c.ID, Title: c.Title, UpdatedAt: c.UpdatedAt})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func truncTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 60 {
		s = s[:60] + "..."
	}
	return s
}
