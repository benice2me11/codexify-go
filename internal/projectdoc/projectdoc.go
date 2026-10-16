package projectdoc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/benice2me11/codexify-go/internal/workspace"
)

const (
	OverrideFile = "AGENTS.override.md"
	DefaultFile  = "AGENTS.md"
	Separator    = "--- project-doc ---"
)

type Entry struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

type Document struct {
	Entries []Entry `json:"entries"`
	Content string  `json:"content"`
}

func Load(workDir string, cfg config.ProjectDocConfig) Document {
	return load(workDir, cfg, nil)
}

// LoadWithin preserves the legacy instruction precedence while refusing
// AGENTS.md/override symlinks or ancestor roots outside the selected member.
// It is used for group bindings; legacy single-project behavior is unchanged.
func LoadWithin(root *workspace.Root, cfg config.ProjectDocConfig) Document {
	if root == nil {
		return Document{}
	}
	return load(root.Path(), cfg, root)
}

func load(workDir string, cfg config.ProjectDocConfig, confinement *workspace.Root) Document {
	if cfg.MaxBytes <= 0 {
		return Document{}
	}
	dirs := projectDirs(workDir, cfg.RootMarkers)
	displayBase := workDir
	if len(dirs) > 0 {
		displayBase = dirs[0]
	}
	names := []string{OverrideFile, DefaultFile}
	for _, name := range cfg.FallbackFilenames {
		name = strings.TrimSpace(name)
		if name != "" && !contains(names, name) {
			names = append(names, name)
		}
	}

	remaining := cfg.MaxBytes
	var entries []Entry
	for _, dir := range dirs {
		if remaining <= 0 {
			break
		}
		for _, name := range names {
			path := filepath.Join(dir, name)
			if confinement != nil {
				rel, relErr := filepath.Rel(confinement.Path(), path)
				if relErr != nil {
					continue
				}
				resolved, resolveErr := confinement.Resolve(rel, false)
				if resolveErr != nil {
					continue
				}
				path = resolved
			}
			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				break
			}
			truncated := len(data) > remaining
			if len(data) > remaining {
				data = data[:remaining]
				for len(data) > 0 && !utf8.Valid(data) {
					data = data[:len(data)-1]
				}
			}
			text := string(data)
			if strings.TrimSpace(text) != "" {
				rel := displayPath(displayBase, path)
				entries = append(entries, Entry{Path: rel, Content: text, Truncated: truncated})
				remaining -= len(data)
			}
			break
		}
	}
	if len(entries) == 0 {
		return Document{}
	}
	var chunks []string
	for _, entry := range entries {
		chunks = append(chunks, entry.Content)
	}
	return Document{Entries: entries, Content: strings.Join(chunks, "\n\n")}
}

func projectDirs(workDir string, markers []string) []string {
	workDir = filepath.Clean(workDir)
	root := ""
	if gitRoot, err := gitTopLevel(workDir); err == nil {
		root = gitRoot
	} else if len(markers) > 0 {
		cursor := workDir
		for {
			found := false
			for _, marker := range markers {
				if _, err := os.Stat(filepath.Join(cursor, marker)); err == nil {
					root = cursor
					found = true
					break
				}
			}
			if found {
				break
			}
			parent := filepath.Dir(cursor)
			if parent == cursor {
				break
			}
			cursor = parent
		}
	}
	if root == "" {
		return []string{workDir}
	}
	rel, err := filepath.Rel(root, workDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return []string{workDir}
	}
	dirs := []string{root}
	if rel == "." {
		return dirs
	}
	cursor := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cursor = filepath.Join(cursor, part)
		dirs = append(dirs, cursor)
	}
	return dirs
}

func gitTopLevel(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return filepath.Clean(strings.TrimSpace(string(out))), nil
}

func displayPath(workDir, path string) string {
	if rel, err := filepath.Rel(workDir, path); err == nil && rel != "." {
		return filepath.ToSlash(rel)
	}
	return filepath.Base(path)
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
