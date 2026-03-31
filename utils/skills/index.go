package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Index holds a mapping of skill names to their metadata and paths.
type Index struct {
	mu     sync.RWMutex
	skills map[string]*Skill // keyed by skill name
	loaded bool
}

// NewIndex creates a new empty skill index.
func NewIndex() *Index {
	return &Index{
		skills: make(map[string]*Skill),
	}
}

// Load scans all skill directories and builds the index.
// Skills from higher-priority directories override lower-priority ones.
// Priority order: user (~/.comanda/skills/) > project (.comanda/skills/) > bundled.
func (idx *Index) Load() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.skills = make(map[string]*Skill)

	dirs := skillDirectories()

	// Load in reverse priority order so higher-priority dirs overwrite
	for i := len(dirs) - 1; i >= 0; i-- {
		dir := dirs[i]
		if err := idx.scanDirectory(dir.path, dir.source); err != nil {
			// Non-fatal: directory might not exist
			continue
		}
	}

	idx.loaded = true
	return nil
}

// Get returns a skill by name, or nil if not found.
func (idx *Index) Get(name string) *Skill {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.skills[name]
}

// All returns all skills sorted by name.
func (idx *Index) All() []*Skill {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	result := make([]*Skill, 0, len(idx.skills))
	for _, s := range idx.skills {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].DisplayName() < result[j].DisplayName()
	})
	return result
}

// Count returns the number of indexed skills.
func (idx *Index) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.skills)
}

type skillDir struct {
	path   string
	source string
}

// skillDirectories returns the directories to scan, in priority order (highest first).
func skillDirectories() []skillDir {
	var dirs []skillDir

	// 1. User-level: ~/.comanda/skills/
	if home, err := os.UserHomeDir(); err == nil {
		userDir := filepath.Join(home, ".comanda", "skills")
		dirs = append(dirs, skillDir{path: userDir, source: "user"})
	}

	// 2. Project-level: .comanda/skills/ (relative to cwd)
	if cwd, err := os.Getwd(); err == nil {
		projectDir := filepath.Join(cwd, ".comanda", "skills")
		dirs = append(dirs, skillDir{path: projectDir, source: "project"})
	}

	// 3. Bundled: <comanda-install>/skills/
	bundledDir := bundledSkillsDir()
	if bundledDir != "" {
		dirs = append(dirs, skillDir{path: bundledDir, source: "bundled"})
	}

	return dirs
}

// bundledSkillsDir returns the path to the bundled skills directory.
// It looks for a skills/ directory relative to the executable.
func bundledSkillsDir() string {
	// Try relative to the executable
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Join(filepath.Dir(exe), "skills")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}

	// For development: try relative to the source file
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		// utils/skills/index.go -> project root
		root := filepath.Dir(filepath.Dir(filepath.Dir(filename)))
		dir := filepath.Join(root, "skills")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}

	return ""
}

// scanDirectory reads all .md files in a directory and adds them to the index.
func (idx *Index) scanDirectory(dir, source string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		skill, err := LoadSkill(path)
		if err != nil {
			// Log but don't fail on individual skill parse errors
			fmt.Fprintf(os.Stderr, "Warning: skipping skill %s: %v\n", path, err)
			continue
		}

		skill.Source = source
		idx.skills[skill.DisplayName()] = skill
	}

	return nil
}

// SkillDirectoryPaths returns the list of directories that will be scanned for skills.
// Useful for debugging and the `skills list` command.
func SkillDirectoryPaths() []string {
	dirs := skillDirectories()
	paths := make([]string, len(dirs))
	for i, d := range dirs {
		paths[i] = d.path
	}
	return paths
}
