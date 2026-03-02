package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type MemoryFile struct {
	Name string
	Size int64
}

func ListMemory() ([]MemoryFile, error) {
	if err := ensureDirs(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(memoryDir())
	if err != nil {
		return nil, err
	}
	var out []MemoryFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, MemoryFile{Name: e.Name(), Size: info.Size()})
	}
	return out, nil
}

func ReadMemory(filename string) (string, error) {
	if err := ensureDirs(); err != nil {
		return "", err
	}
	safe := filepath.Base(filename)
	data, err := os.ReadFile(filepath.Join(memoryDir(), safe))
	if os.IsNotExist(err) {
		return "", fmt.Errorf("memory file %q not found", safe)
	}
	return string(data), err
}

func WriteMemory(filename, content string) error {
	if err := ensureDirs(); err != nil {
		return err
	}
	safe := filepath.Base(filename)
	if !strings.HasSuffix(safe, ".md") {
		safe += ".md"
	}
	return os.WriteFile(filepath.Join(memoryDir(), safe), []byte(content), 0o644)
}

func DeleteMemory(filename string) error {
	safe := filepath.Base(filename)
	return os.Remove(filepath.Join(memoryDir(), safe))
}
