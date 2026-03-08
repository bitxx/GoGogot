package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Skill struct {
	Name        string
	Description string
	FilePath    string
	Dir         string
}

// LoadSkills discovers skills from the given root directory.
// Each immediate subdirectory containing a SKILL.md is treated as a skill.
func LoadSkills(rootDir string) ([]Skill, error) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillMd := filepath.Join(rootDir, e.Name(), "SKILL.md")
		data, err := os.ReadFile(skillMd)
		if err != nil {
			continue
		}
		name, desc := parseSkillFrontmatter(string(data))
		if name == "" {
			name = e.Name()
		}
		out = append(out, Skill{
			Name:        name,
			Description: desc,
			FilePath:    skillMd,
			Dir:         filepath.Join(rootDir, e.Name()),
		})
	}
	return out, nil
}

// FormatSkillsForPrompt builds an <available_skills> XML block for system prompt injection.
func FormatSkillsForPrompt(skills []Skill) string {
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

func CreateSkill(rootDir, name, description, body string) (string, error) {
	safeName := sanitizeSkillName(name)
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

func UpdateSkill(rootDir, name, content string) error {
	safeName := sanitizeSkillName(name)
	skillMd := filepath.Join(rootDir, safeName, "SKILL.md")

	if _, err := os.Stat(skillMd); os.IsNotExist(err) {
		return fmt.Errorf("skill %q not found", safeName)
	}
	return os.WriteFile(skillMd, []byte(content), 0o644)
}

func DeleteSkill(rootDir, name string) error {
	safeName := sanitizeSkillName(name)
	skillDir := filepath.Join(rootDir, safeName)

	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		return fmt.Errorf("skill %q not found", safeName)
	}
	return os.RemoveAll(skillDir)
}

func ReadSkill(rootDir, name string) (string, error) {
	safeName := sanitizeSkillName(name)
	data, err := os.ReadFile(filepath.Join(rootDir, safeName, "SKILL.md"))
	if os.IsNotExist(err) {
		return "", fmt.Errorf("skill %q not found", safeName)
	}
	return string(data), err
}

func parseSkillFrontmatter(content string) (name, description string) {
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
		if key, val, ok := splitSkillYAMLLine(line); ok {
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

func splitSkillYAMLLine(line string) (key, val string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	val = strings.TrimSpace(line[idx+1:])
	val = strings.Trim(val, `"'`)
	return key, val, true
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

func sanitizeSkillName(name string) string {
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
