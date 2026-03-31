package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitFrontmatter(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantFM     string
		wantBody   string
		wantErr    bool
	}{
		{
			name: "basic frontmatter",
			input: `---
name: test
description: A test skill
---

# Hello

Body content here.`,
			wantFM:   "name: test\ndescription: A test skill",
			wantBody: "# Hello\n\nBody content here.",
		},
		{
			name:     "no frontmatter",
			input:    "# Just a markdown file\n\nWith content.",
			wantFM:   "",
			wantBody: "# Just a markdown file\n\nWith content.",
		},
		{
			name: "unclosed frontmatter",
			input: `---
name: test
description: never closed`,
			wantErr: true,
		},
		{
			name:     "empty content",
			input:    "",
			wantFM:   "",
			wantBody: "",
		},
		{
			name: "frontmatter only",
			input: `---
name: test
---`,
			wantFM:   "name: test",
			wantBody: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, body, err := splitFrontmatter(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fm != tt.wantFM {
				t.Errorf("frontmatter = %q, want %q", fm, tt.wantFM)
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

func TestParseSkill(t *testing.T) {
	content := `---
name: code-review
description: Review code for bugs and security issues
when_to_use: When the user asks to review code
model: claude-sonnet-4
allowed-tools:
  - FileRead
  - Grep
arguments:
  - severity
argument-hint: "code-review [--severity low|medium|high]"
effort: medium
---

# Code Review

Review the following code with ${severity:-medium} severity:

${USER_INPUT}`

	skill, err := ParseSkill(content)
	if err != nil {
		t.Fatalf("ParseSkill() error: %v", err)
	}

	if skill.Name != "code-review" {
		t.Errorf("Name = %q, want %q", skill.Name, "code-review")
	}
	if skill.Description != "Review code for bugs and security issues" {
		t.Errorf("Description = %q", skill.Description)
	}
	if skill.Model != "claude-sonnet-4" {
		t.Errorf("Model = %q, want %q", skill.Model, "claude-sonnet-4")
	}
	if len(skill.AllowedTools) != 2 {
		t.Errorf("AllowedTools = %v, want 2 items", skill.AllowedTools)
	}
	if len(skill.Arguments) != 1 || skill.Arguments[0] != "severity" {
		t.Errorf("Arguments = %v, want [severity]", skill.Arguments)
	}
	if skill.Effort != "medium" {
		t.Errorf("Effort = %q, want %q", skill.Effort, "medium")
	}
	if skill.Body == "" {
		t.Error("Body should not be empty")
	}
}

func TestParseSkillUserInvocable(t *testing.T) {
	// Default (not specified) should be invocable
	skill1, _ := ParseSkill("---\nname: test\ndescription: test\n---\nbody")
	if !skill1.IsUserInvocable() {
		t.Error("should be user-invocable by default")
	}

	// Explicitly false
	skill2, _ := ParseSkill("---\nname: test\ndescription: test\nuser-invocable: false\n---\nbody")
	if skill2.IsUserInvocable() {
		t.Error("should not be user-invocable when set to false")
	}

	// Explicitly true
	skill3, _ := ParseSkill("---\nname: test\ndescription: test\nuser-invocable: true\n---\nbody")
	if !skill3.IsUserInvocable() {
		t.Error("should be user-invocable when set to true")
	}
}

func TestSubstituteVariables(t *testing.T) {
	body := "Hello ${USER_INPUT}, today is ${TIMESTAMP}. Dir: ${COMANDA_SKILL_DIR}"
	vars := map[string]string{
		"USER_INPUT":       "world",
		"TIMESTAMP":        "2025-01-01T00:00:00Z",
		"COMANDA_SKILL_DIR": "/tmp/skills",
	}

	result := SubstituteVariables(body, vars)
	expected := "Hello world, today is 2025-01-01T00:00:00Z. Dir: /tmp/skills"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestResolveDefaults(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"${length:-medium}", "medium"},
		{"${already_set}", "${already_set}"},
		{"no vars here", "no vars here"},
		{"${a:-foo} and ${b:-bar}", "foo and bar"},
		{"${empty:-}", ""},
	}

	for _, tt := range tests {
		got := resolveDefaults(tt.input)
		if got != tt.want {
			t.Errorf("resolveDefaults(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRenderBody(t *testing.T) {
	skill := &Skill{
		Name: "test",
		Body: "Summary (${length:-medium}): ${USER_INPUT}",
		Dir:  "/tmp/skills",
	}

	// With explicit arg
	result := RenderBody(skill, "my text", map[string]string{"length": "short"}, "")
	if result != "Summary (short): my text" {
		t.Errorf("got %q", result)
	}

	// Without arg (uses default)
	result = RenderBody(skill, "my text", nil, "")
	if result != "Summary (medium): my text" {
		t.Errorf("got %q, want default 'medium'", result)
	}
}

func TestDisplayName(t *testing.T) {
	// With name set
	s1 := &Skill{Name: "my-skill", FilePath: "/tmp/other.md"}
	if s1.DisplayName() != "my-skill" {
		t.Errorf("DisplayName() = %q, want %q", s1.DisplayName(), "my-skill")
	}

	// Without name, uses filename
	s2 := &Skill{FilePath: "/tmp/cool-skill.md"}
	if s2.DisplayName() != "cool-skill" {
		t.Errorf("DisplayName() = %q, want %q", s2.DisplayName(), "cool-skill")
	}
}

func TestLoadSkill(t *testing.T) {
	// Create a temp skill file
	dir := t.TempDir()
	path := filepath.Join(dir, "test-skill.md")
	content := `---
description: A test skill
model: gpt-4o
---

Do something with ${USER_INPUT}`

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	skill, err := LoadSkill(path)
	if err != nil {
		t.Fatalf("LoadSkill() error: %v", err)
	}

	if skill.Name != "test-skill" {
		t.Errorf("Name = %q, want %q (derived from filename)", skill.Name, "test-skill")
	}
	if skill.Description != "A test skill" {
		t.Errorf("Description = %q", skill.Description)
	}
	if skill.Model != "gpt-4o" {
		t.Errorf("Model = %q", skill.Model)
	}
	if skill.Dir != dir {
		t.Errorf("Dir = %q, want %q", skill.Dir, dir)
	}
}

func TestIndexLoadAndGet(t *testing.T) {
	// Create a temp directory with skills
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, ".comanda", "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a test skill
	content := `---
name: my-skill
description: A discoverable skill
---

Do the thing.`

	if err := os.WriteFile(filepath.Join(skillsDir, "my-skill.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Change to the temp dir so project-level discovery works
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	idx := NewIndex()
	if err := idx.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	skill := idx.Get("my-skill")
	if skill == nil {
		t.Fatal("Get('my-skill') returned nil")
	}
	if skill.Description != "A discoverable skill" {
		t.Errorf("Description = %q", skill.Description)
	}
	if skill.Source != "project" {
		t.Errorf("Source = %q, want %q", skill.Source, "project")
	}

	all := idx.All()
	found := false
	for _, s := range all {
		if s.Name == "my-skill" {
			found = true
		}
	}
	if !found {
		t.Error("All() does not contain my-skill")
	}
}

func TestIndexPriority(t *testing.T) {
	// Create project-level and "user-level" skills with same name
	dir := t.TempDir()

	// Project level
	projectDir := filepath.Join(dir, ".comanda", "skills")
	os.MkdirAll(projectDir, 0755)
	os.WriteFile(filepath.Join(projectDir, "dupe.md"), []byte("---\nname: dupe\ndescription: project version\n---\nproject body"), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	idx := NewIndex()
	idx.Load()

	skill := idx.Get("dupe")
	if skill == nil {
		t.Fatal("skill not found")
	}
	// Project should be found since we only have project dir
	if skill.Source != "project" {
		t.Errorf("Source = %q, want %q", skill.Source, "project")
	}
}
