package store

import (
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var tzRegex = regexp.MustCompile(`(?im)^.*timezone:\s*(\S+)`)

// LoadTimezone extracts the IANA timezone from user.md (line matching "timezone: <value>").
// Fallback: TZ env var, then UTC.
func LoadTimezone() *time.Location {
	if content := ReadUser(); content != "" {
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
