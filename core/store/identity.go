package store

import (
	"os"
	"path/filepath"
)

func soulPath() string { return filepath.Join(DataDir(), "soul.md") }
func userPath() string { return filepath.Join(DataDir(), "user.md") }

func ReadSoul() string {
	data, err := os.ReadFile(soulPath())
	if err != nil {
		return ""
	}
	return string(data)
}

func WriteSoul(content string) error {
	return os.WriteFile(soulPath(), []byte(content), 0o644)
}

func ReadUser() string {
	data, err := os.ReadFile(userPath())
	if err != nil {
		return ""
	}
	return string(data)
}

func WriteUser(content string) error {
	return os.WriteFile(userPath(), []byte(content), 0o644)
}
