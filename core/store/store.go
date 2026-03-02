package store

import (
	"os"
	"path/filepath"
)

var dataDir string

func Init(dir string) {
	dataDir = dir
}

func DataDir() string {
	if dataDir != "" {
		return dataDir
	}
	if dir := os.Getenv("GOGOGOT_DATA_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gogogot")
}

func chatsDir() string    { return filepath.Join(DataDir(), "chats") }
func memoryDir() string   { return filepath.Join(DataDir(), "memory") }
func SkillsDir() string   { return filepath.Join(DataDir(), "skills") }
func mappingPath() string { return filepath.Join(DataDir(), "external_chats.json") }

func ensureDirs() error {
	for _, dir := range []string{chatsDir(), memoryDir(), SkillsDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}
