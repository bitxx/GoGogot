package local

import (
	"fmt"
	"os"
	"strings"
)

func (s *LocalStore) GetActiveEpisodeID() (string, error) {
	epID, err := s.loadActiveEpisodeID()
	if err != nil {
		return "", err
	}
	if epID == "" {
		return "", fmt.Errorf("no active episode")
	}
	return epID, nil
}

func (s *LocalStore) SetActiveEpisodeID(episodeID string) error {
	return os.WriteFile(s.activeEpisodePath(), []byte(episodeID+"\n"), 0o644)
}

func (s *LocalStore) loadActiveEpisodeID() (string, error) {
	data, err := os.ReadFile(s.activeEpisodePath())
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
