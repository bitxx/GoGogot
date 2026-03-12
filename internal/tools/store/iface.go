package store

import "time"

// Store defines the persistence contract for all application data.
// Implementations may use a local filesystem, S3, or any other backend.
type Store interface {
	EpisodePersister

	// Episodes
	NewEpisode() *Episode
	LoadEpisode(id string) (*Episode, error)
	ListEpisodes() ([]EpisodeInfo, error)

	// Active episode mapping
	GetActiveEpisodeID() (string, error)
	SetActiveEpisodeID(id string) error

	// Memory
	ListMemory() ([]MemoryFile, error)
	ReadMemory(filename string) (string, error)
	WriteMemory(filename, content string) error
	DeleteMemory(filename string) error

	// Identity
	ReadSoul() string
	WriteSoul(content string) error
	ReadUser() string
	WriteUser(content string) error
	LoadTimezone() *time.Location

	// Skills
	SkillsDir() string
	LoadSkills() ([]Skill, error)
	CreateSkill(name, description, body string) (string, error)
	UpdateSkill(name, content string) error
	DeleteSkill(name string) error
	ReadSkill(name string) (string, error)
}

// EpisodePersister is the subset of Store that Episode delegates its I/O to.
// Concrete store implementations satisfy this automatically by implementing Store.
type EpisodePersister interface {
	SaveEpisode(ep *Episode) error
	LoadMessages(ep *Episode) error
	AppendMessage(ep *Episode, msg Turn)
	ReplaceMessages(ep *Episode, msgs []Turn) error
	TextMessages(ep *Episode) ([]Message, error)
	HasMessages(ep *Episode) bool
}
