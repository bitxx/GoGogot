package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Skill struct {
	Name        string
	Description string
	FilePath    string // absolute path to SKILL.md
	Dir         string // skill directory (for scripts/, references/)
}

// LoadAll discovers skills from the given root directory.
// Each immediate subdirectory containing a SKILL.md is treated as a skill.
func LoadAll(rootDir string) ([]Skill, error) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var skills []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillMd := filepath.Join(rootDir, e.Name(), "SKILL.md")
		data, err := os.ReadFile(skillMd)
		if err != nil {
			continue
		}
		name, desc := parseFrontmatter(string(data))
		if name == "" {
			name = e.Name()
		}
		skills = append(skills, Skill{
			Name:        name,
			Description: desc,
			FilePath:    skillMd,
			Dir:         filepath.Join(rootDir, e.Name()),
		})
	}
	return skills, nil
}

// FormatForPrompt builds an <available_skills> XML block for system prompt injection.
// Only name, description, and file path are included — the model reads
// the full SKILL.md via the read tool when it decides to activate a skill.
func FormatForPrompt(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<available_skills>\n")
	for _, s := range skills {
		b.WriteString(fmt.Sprintf(
			"<skill name=%q description=%q location=%q />\n",
			s.Name, s.Description, s.FilePath,
		))
	}
	b.WriteString("</available_skills>")
	return b.String()
}

// parseFrontmatter extracts name and description from YAML frontmatter.
// Expects the standard `---` delimited block at the top of the file.
func parseFrontmatter(content string) (name, description string) {
	if !strings.HasPrefix(content, "---") {
		return "", ""
	}
	end := strings.Index(content[3:], "---")
	if end < 0 {
		return "", ""
	}
	block := content[3 : 3+end]

	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if key, val, ok := splitYAMLLine(line); ok {
			switch key {
			case "name":
				name = val
			case "description":
				description = val
			}
		}
	}
	return name, description
}

func splitYAMLLine(line string) (key, val string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	val = strings.TrimSpace(line[idx+1:])
	val = strings.Trim(val, `"'`)
	return key, val, true
}

// CreateSkill creates a new skill directory with a SKILL.md file.
func CreateSkill(rootDir, name, description, body string) (string, error) {
	safeName := sanitizeName(name)
	skillDir := filepath.Join(rootDir, safeName)

	if _, err := os.Stat(skillDir); err == nil {
		return "", fmt.Errorf("skill %q already exists", safeName)
	}

	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return "", err
	}

	content := formatSkillMd(name, description, body)
	skillMd := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillMd, []byte(content), 0o644); err != nil {
		return "", err
	}
	return skillMd, nil
}

// UpdateSkill overwrites the SKILL.md of an existing skill.
func UpdateSkill(rootDir, name, content string) error {
	safeName := sanitizeName(name)
	skillMd := filepath.Join(rootDir, safeName, "SKILL.md")

	if _, err := os.Stat(skillMd); os.IsNotExist(err) {
		return fmt.Errorf("skill %q not found", safeName)
	}
	return os.WriteFile(skillMd, []byte(content), 0o644)
}

// DeleteSkill removes a skill directory entirely.
func DeleteSkill(rootDir, name string) error {
	safeName := sanitizeName(name)
	skillDir := filepath.Join(rootDir, safeName)

	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		return fmt.Errorf("skill %q not found", safeName)
	}
	return os.RemoveAll(skillDir)
}

// ReadSkill returns the raw content of a skill's SKILL.md.
func ReadSkill(rootDir, name string) (string, error) {
	safeName := sanitizeName(name)
	data, err := os.ReadFile(filepath.Join(rootDir, safeName, "SKILL.md"))
	if os.IsNotExist(err) {
		return "", fmt.Errorf("skill %q not found", safeName)
	}
	return string(data), err
}

func formatSkillMd(name, description, body string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("name: %s\n", name))
	b.WriteString(fmt.Sprintf("description: %q\n", description))
	b.WriteString("---\n\n")
	if body != "" {
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		if r == ' ' || r == '_' {
			return '-'
		}
		return -1
	}, name)
	return strings.Trim(name, "-")
}
