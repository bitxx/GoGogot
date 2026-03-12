package store

import (
	"context"
	"fmt"
	"gogogot/internal/tools/types"
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

func (s *Store) SkillTools() []types.Tool {
	skillsDir := s.SkillsDir()
	return []types.Tool{
		{
			Name:  "skill_list",
			Label: "Listing skills",
			Description: "List all available skills with their name and description. Skills are reusable procedural knowledge — workflows, integrations, how-to guides — that you created from past experience.",
			Parameters:  map[string]any{},
			Handler: func(_ context.Context, _ map[string]any) types.Result {
				loaded, err := LoadSkills(skillsDir)
				if err != nil {
					return types.Result{Output: "error listing skills: " + err.Error(), IsErr: true}
				}
				if len(loaded) == 0 {
					return types.Result{Output: "(no skills yet — use skill_create to save procedural knowledge for reuse)"}
				}
				var b strings.Builder
				for _, sk := range loaded {
					fmt.Fprintf(&b, "- %s: %s\n  path: %s\n", sk.Name, sk.Description, sk.FilePath)
				}
				return types.Result{Output: b.String()}
			},
		},
		{
			Name:  "skill_read",
			Label: "Reading skill",
			Description: "Read the full content of a skill's SKILL.md. Use this when a skill matches the current task — " +
				"read it first, then follow the instructions inside.",
			Parameters: map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name (lowercase, hyphen-separated)",
				},
			},
			Required: []string{"name"},
			Handler: func(_ context.Context, input map[string]any) types.Result {
				name, err := types.GetString(input, "name")
				if err != nil {
					return types.ErrResult(err)
				}
				content, err := ReadSkill(skillsDir, name)
				if err != nil {
					return types.Result{Output: err.Error(), IsErr: true}
				}
				return types.Result{Output: content}
			},
		},
		{
			Name:  "skill_create",
			Label: "Creating skill",
			Description: "Create a new skill to capture procedural knowledge for future reuse. " +
				"Use after solving a non-trivial problem — save the workflow so you can do it better next time. " +
				"The skill will appear in your available_skills on future conversations.",
			Parameters: map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Short lowercase name with hyphens, e.g. deploy-docker, morning-digest",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "When to use this skill — include triggers and context so you can match it later",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Markdown instructions: commands, steps, examples, gotchas",
				},
			},
			Required: []string{"name", "description", "body"},
			Handler: func(_ context.Context, input map[string]any) types.Result {
				name, err := types.GetString(input, "name")
				if err != nil {
					return types.ErrResult(err)
				}
				desc, err := types.GetString(input, "description")
				if err != nil {
					return types.ErrResult(err)
				}
				body, err := types.GetString(input, "body")
				if err != nil {
					return types.ErrResult(err)
				}

				path, err := CreateSkill(skillsDir, name, desc, body)
				if err != nil {
					return types.Result{Output: "error creating skill: " + err.Error(), IsErr: true}
				}
				return types.Result{Output: fmt.Sprintf("skill %q created at %s — it will appear in your available_skills on future conversations", name, path)}
			},
		},
		{
			Name:  "skill_update",
			Label: "Updating skill",
			Description: "Update an existing skill's SKILL.md with new content. " +
				"Read the skill first with skill_read, then write the improved version.",
			Parameters: map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name to update",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Full new content for SKILL.md (including frontmatter)",
				},
			},
			Required: []string{"name", "content"},
			Handler: func(_ context.Context, input map[string]any) types.Result {
				name, err := types.GetString(input, "name")
				if err != nil {
					return types.ErrResult(err)
				}
				content, err := types.GetString(input, "content")
				if err != nil {
					return types.ErrResult(err)
				}

				if err := UpdateSkill(skillsDir, name, content); err != nil {
					return types.Result{Output: "error updating skill: " + err.Error(), IsErr: true}
				}
				return types.Result{Output: fmt.Sprintf("skill %q updated", name)}
			},
		},
		{
			Name:  "skill_delete",
			Label: "Deleting skill",
			Description: "Delete a skill and its entire directory. Use when a skill is obsolete or incorrect.",
			Parameters: map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name to delete",
				},
			},
			Required: []string{"name"},
			Handler: func(_ context.Context, input map[string]any) types.Result {
				name, err := types.GetString(input, "name")
				if err != nil {
					return types.ErrResult(err)
				}
				if err := DeleteSkill(skillsDir, name); err != nil {
					return types.Result{Output: "error deleting skill: " + err.Error(), IsErr: true}
				}
				return types.Result{Output: fmt.Sprintf("skill %q deleted", name)}
			},
		},
	}
}

// --- Implementation ---

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
