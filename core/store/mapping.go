package store

import (
	"encoding/json"
	"fmt"
	"os"
)

func loadMapping() (map[string]string, error) {
	data, err := os.ReadFile(mappingPath())
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func saveMapping(m map[string]string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(mappingPath(), data, 0o644)
}

func LoadOrCreateByExternalID(externalID string) (*Chat, error) {
	if err := ensureDirs(); err != nil {
		return nil, err
	}
	m, err := loadMapping()
	if err != nil {
		return nil, err
	}
	if chatID, ok := m[externalID]; ok {
		chat, err := LoadChat(chatID)
		if err == nil {
			return chat, nil
		}
	}
	chat := NewChat()
	if err := chat.Save(); err != nil {
		return nil, err
	}
	m[externalID] = chat.ID
	if err := saveMapping(m); err != nil {
		return nil, err
	}
	return chat, nil
}

func GetExternalMapping(externalID string) (string, error) {
	m, err := loadMapping()
	if err != nil {
		return "", err
	}
	chatID, ok := m[externalID]
	if !ok {
		return "", fmt.Errorf("no mapping for %q", externalID)
	}
	return chatID, nil
}

func SetExternalMapping(externalID, chatID string) error {
	if err := ensureDirs(); err != nil {
		return err
	}
	m, err := loadMapping()
	if err != nil {
		return err
	}
	m[externalID] = chatID
	return saveMapping(m)
}
