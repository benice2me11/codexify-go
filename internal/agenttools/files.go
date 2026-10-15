package agenttools

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/benice2me11/codexify-go/internal/workspace"
	"github.com/bmatcuk/doublestar/v4"
)

const maxWriteBytes = 2 << 20

type Files struct {
	Root *workspace.Root
}

// RepositoryContext identifies which group checkout actually serviced a
// routed tool call. Absent in legacy single-project responses.
type RepositoryContext struct {
	Alias             string `json:"alias"`
	SourceProjectRoot string `json:"sourceProjectRoot"`
	ProjectRoot       string `json:"projectRoot"`
	Checkout          string `json:"checkout"`
}

type ReadFileInput struct {
	Repository string `json:"repository,omitempty" jsonschema:"optional project-group repository alias; defaults to active repository"`
	Path       string `json:"path" jsonschema:"workspace-relative file path"`
	Offset     int    `json:"offset,omitempty" jsonschema:"zero-based first line"`
	Limit      int    `json:"limit,omitempty" jsonschema:"maximum lines to return; default 200"`
}

type ReadFileOutput struct {
	Repository *RepositoryContext `json:"repository,omitempty"`
	Path       string             `json:"path"`
	Content    string             `json:"content"`
	NextOffset *int               `json:"nextOffset,omitempty"`
}

func (f *Files) ReadFile(in ReadFileInput) (ReadFileOutput, error) {
	if in.Offset < 0 {
		return ReadFileOutput{}, errors.New("offset must be >= 0")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}
	path, err := f.Root.Resolve(in.Path, false)
	if err != nil {
		return ReadFileOutput{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return ReadFileOutput{}, err
	}
	if info.IsDir() {
		return ReadFileOutput{}, errors.New("path is a directory")
	}
	file, err := os.Open(path)
	if err != nil {
		return ReadFileOutput{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2<<20)
	var b strings.Builder
	line := 0
	returned := 0
	hasMore := false
	for scanner.Scan() {
		if line >= in.Offset {
			if returned >= limit {
				hasMore = true
				break
			}
			b.WriteString(strconv.Itoa(line + 1))
			b.WriteByte('\t')
			b.WriteString(scanner.Text())
			b.WriteByte('\n')
			returned++
		}
		line++
	}
	if err := scanner.Err(); err != nil {
		return ReadFileOutput{}, err
	}
	rel, _ := f.Root.Relative(path)
	out := ReadFileOutput{Path: rel, Content: b.String()}
	if hasMore {
		next := in.Offset + returned
		out.NextOffset = &next
	}
	return out, nil
}

type WriteFileInput struct {
	Repository string `json:"repository,omitempty" jsonschema:"optional project-group repository alias"`
	Path       string `json:"path" jsonschema:"workspace-relative destination path"`
	Content    string `json:"content" jsonschema:"complete replacement UTF-8 text content"`
}

type WriteFileOutput struct {
	Repository *RepositoryContext `json:"repository,omitempty"`
	Path       string             `json:"path"`
	Bytes      int                `json:"bytes"`
}

func (f *Files) WriteFile(in WriteFileInput) (WriteFileOutput, error) {
	if len(in.Content) > maxWriteBytes {
		return WriteFileOutput{}, fmt.Errorf("content exceeds %d bytes", maxWriteBytes)
	}
	path, err := f.Root.Resolve(in.Path, true)
	if err != nil {
		return WriteFileOutput{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return WriteFileOutput{}, err
	}
	if err := os.WriteFile(path, []byte(in.Content), 0o644); err != nil {
		return WriteFileOutput{}, err
	}
	rel, _ := f.Root.Relative(path)
	return WriteFileOutput{Path: rel, Bytes: len(in.Content)}, nil
}

type GlobInput struct {
	Repository string `json:"repository,omitempty" jsonschema:"optional project-group repository alias"`
	Pattern    string `json:"pattern" jsonschema:"glob pattern, supports double-star recursion"`
	Path       string `json:"path,omitempty" jsonschema:"optional workspace-relative base directory"`
}

type GlobOutput struct {
	Repository *RepositoryContext `json:"repository,omitempty"`
	Matches    []string           `json:"matches"`
}

func (f *Files) Glob(in GlobInput) (GlobOutput, error) {
	if strings.TrimSpace(in.Pattern) == "" {
		return GlobOutput{}, errors.New("pattern is required")
	}
	if filepath.IsAbs(in.Pattern) || containsParentSegment(filepath.ToSlash(in.Pattern)) {
		return GlobOutput{}, errors.New("glob pattern must stay within workspace")
	}
	base, err := f.Root.Resolve(in.Path, false)
	if err != nil {
		return GlobOutput{}, err
	}
	info, err := os.Stat(base)
	if err != nil {
		return GlobOutput{}, err
	}
	if !info.IsDir() {
		return GlobOutput{}, errors.New("glob base path is not a directory")
	}
	matches, err := doublestar.Glob(os.DirFS(base), filepath.ToSlash(in.Pattern), doublestar.WithFilesOnly())
	if err != nil {
		return GlobOutput{}, err
	}
	sort.Strings(matches)
	if len(matches) > 1000 {
		matches = matches[:1000]
	}
	baseRel, _ := f.Root.Relative(base)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		// os.DirFS and glob libraries do not constitute a security boundary:
		// every discovered path must pass the same canonical containment
		// check as read_file before it can be disclosed to the caller.
		matched, err := f.Root.Resolve(filepath.Join(baseRel, filepath.FromSlash(match)), false)
		if err != nil {
			return GlobOutput{}, fmt.Errorf("glob result escapes selected repository: %w", err)
		}
		if _, err := f.Root.Relative(matched); err != nil {
			return GlobOutput{}, err
		}
		if baseRel == "." {
			out = append(out, filepath.ToSlash(match))
		} else {
			out = append(out, filepath.ToSlash(filepath.Join(baseRel, filepath.FromSlash(match))))
		}
	}
	return GlobOutput{Matches: out}, nil
}

type GrepInput struct {
	Repository string `json:"repository,omitempty" jsonschema:"optional project-group repository alias"`
	Pattern    string `json:"pattern" jsonschema:"RE2 regular expression"`
	Path       string `json:"path,omitempty" jsonschema:"workspace-relative file or directory"`
	Glob       string `json:"glob,omitempty" jsonschema:"optional file glob filter"`
	MaxResults int    `json:"maxResults,omitempty" jsonschema:"maximum matches; default 100"`
}

type GrepMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type GrepOutput struct {
	Repository *RepositoryContext `json:"repository,omitempty"`
	Matches    []GrepMatch        `json:"matches"`
	Truncated  bool               `json:"truncated"`
}

func (f *Files) Grep(in GrepInput) (GrepOutput, error) {
	if strings.TrimSpace(in.Pattern) == "" {
		return GrepOutput{}, errors.New("pattern is required")
	}
	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return GrepOutput{}, err
	}
	max := in.MaxResults
	if max <= 0 {
		max = 100
	}
	if max > 2000 {
		max = 2000
	}
	start, err := f.Root.Resolve(in.Path, false)
	if err != nil {
		return GrepOutput{}, err
	}
	var matches []GrepMatch
	truncated := false

	visit := func(path string, info os.FileInfo) error {
		if len(matches) >= max {
			truncated = true
			return errStopWalk
		}
		if info.IsDir() {
			if info.Name() == ".git" && path != start {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := f.Root.Relative(path)
		if err != nil {
			return err
		}
		if in.Glob != "" {
			ok, err := doublestar.Match(in.Glob, filepath.ToSlash(rel))
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
		}
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 2<<20)
		line := 0
		for scanner.Scan() {
			line++
			text := scanner.Text()
			if strings.IndexByte(text, 0) >= 0 {
				return nil
			}
			if re.MatchString(text) {
				matches = append(matches, GrepMatch{Path: rel, Line: line, Text: text})
				if len(matches) >= max {
					truncated = true
					return errStopWalk
				}
			}
		}
		return nil
	}

	info, err := os.Stat(start)
	if err != nil {
		return GrepOutput{}, err
	}
	if info.IsDir() {
		err = filepath.Walk(start, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			return visit(path, info)
		})
	} else {
		err = visit(start, info)
	}
	if err != nil && !errors.Is(err, errStopWalk) {
		return GrepOutput{}, err
	}
	return GrepOutput{Matches: matches, Truncated: truncated}, nil
}

var errStopWalk = errors.New("stop walk")

func containsParentSegment(pattern string) bool {
	for _, part := range strings.Split(pattern, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}
