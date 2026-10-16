package patch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode"

	"github.com/benice2me11/codexify-go/internal/workspace"
)

const (
	beginPatch = "*** Begin Patch"
	endPatch   = "*** End Patch"
	addFile    = "*** Add File: "
	deleteFile = "*** Delete File: "
	updateFile = "*** Update File: "
	moveTo     = "*** Move to: "
	endOfFile  = "*** End of File"
)

type Chunk struct {
	Context   string
	OldLines  []string
	NewLines  []string
	EndOfFile bool
}

type Action struct {
	Kind     string
	Path     string
	MovePath string
	Lines    []string
	Chunks   []Chunk
}

type planned struct {
	action         Action
	source         string
	dest           string
	contents       string
	sourceIdentity os.FileInfo
}

func Apply(root *workspace.Root, input string) (string, error) {
	if root == nil {
		return "", errors.New("active workspace is unavailable")
	}
	actions, err := Parse(input)
	if err != nil {
		return "", fmt.Errorf("invalid patch: %w", err)
	}

	seen := map[string]struct{}{}
	plans := make([]planned, 0, len(actions))
	for _, action := range actions {
		source, err := root.Resolve(filepath.FromSlash(action.Path), action.Kind == "add")
		if err != nil {
			return "", fmt.Errorf("patch does not apply: %s: %w", actionLabel(action), err)
		}
		key := strings.ToLower(filepath.Clean(source))
		if _, exists := seen[key]; exists {
			return "", fmt.Errorf("patch does not apply: multiple operations target %s", action.Path)
		}
		seen[key] = struct{}{}

		p := planned{action: action, source: source}
		switch action.Kind {
		case "add":
			p.contents = renderAdded(action.Lines)
		case "delete":
			info, err := os.Stat(source)
			if err != nil || !info.Mode().IsRegular() {
				return "", fmt.Errorf("patch does not apply: Delete File: %q does not exist as a regular file", action.Path)
			}
			_ = os.SameFile(info, info) // eager Windows file ID before later mutation
			p.sourceIdentity = info
		case "update":
			info, err := os.Stat(source)
			if err != nil || !info.Mode().IsRegular() {
				return "", fmt.Errorf("patch does not apply: Update File: %q does not exist as a regular file", action.Path)
			}
			_ = os.SameFile(info, info)
			p.sourceIdentity = info
			original, err := os.ReadFile(source)
			if err != nil {
				return "", fmt.Errorf("patch does not apply: read %s: %w", action.Path, err)
			}
			p.contents, err = ApplyUpdate(string(original), action.Chunks, action.Path)
			if err != nil {
				return "", fmt.Errorf("patch does not apply: %w", err)
			}
			if action.MovePath != "" {
				dest, err := root.Resolve(filepath.FromSlash(action.MovePath), true)
				if err != nil {
					return "", fmt.Errorf("patch does not apply: move destination %q: %w", action.MovePath, err)
				}
				p.dest = dest
			}
		default:
			return "", fmt.Errorf("unknown patch action %q", action.Kind)
		}
		plans = append(plans, p)
	}

	var summary []string
	for _, p := range plans {
		action := p.action
		var err error
		if err := revalidatePlanned(root, p); err != nil {
			return "", fmt.Errorf("patch destination changed before applying %s: %w", actionLabel(action), err)
		}
		switch action.Kind {
		case "add":
			err = ensureParent(p.source)
			if err == nil {
				err = revalidatePlanned(root, p)
			}
			if err == nil {
				err = os.WriteFile(p.source, []byte(p.contents), 0o644)
			}
			if err == nil {
				summary = append(summary, "A "+action.Path)
			}
		case "delete":
			err = os.Remove(p.source)
			if err == nil {
				summary = append(summary, "D "+action.Path)
			}
		case "update":
			if p.dest != "" && filepath.Clean(p.dest) != filepath.Clean(p.source) {
				err = ensureParent(p.dest)
				if err == nil {
					err = revalidatePlanned(root, p)
				}
				if err == nil {
					err = os.WriteFile(p.dest, []byte(p.contents), 0o644)
				}
				if err == nil {
					err = os.Remove(p.source)
				}
				if err == nil {
					summary = append(summary, fmt.Sprintf("R %s -> %s", action.Path, action.MovePath))
				}
			} else {
				err = os.WriteFile(p.source, []byte(p.contents), 0o644)
				if err == nil {
					summary = append(summary, "M "+action.Path)
				}
			}
		}
		if err != nil {
			completed := "<none>"
			if len(summary) > 0 {
				completed = strings.Join(summary, "\n")
			}
			return "", fmt.Errorf(
				"patch failed while applying %s: %w\nCompleted before failure:\n%s\nThe failing operation may also have modified its target; inspect the working tree before retrying",
				actionLabel(action), err, completed,
			)
		}
	}
	return "Patch applied:\n" + strings.Join(summary, "\n"), nil
}

func revalidatePlanned(root *workspace.Root, p planned) error {
	resolved, err := root.Resolve(filepath.FromSlash(p.action.Path), p.action.Kind == "add")
	if err != nil || !sameCanonicalPath(resolved, p.source) {
		return errors.New("source path is no longer within the selected checkout")
	}
	if p.sourceIdentity != nil {
		info, err := os.Stat(resolved)
		if err != nil || !os.SameFile(p.sourceIdentity, info) {
			return errors.New("source file identity changed after patch verification")
		}
	}
	if p.dest != "" {
		resolved, err := root.Resolve(filepath.FromSlash(p.action.MovePath), true)
		if err != nil || !sameCanonicalPath(resolved, p.dest) {
			return errors.New("move destination is no longer within the selected checkout")
		}
	}
	return nil
}

func sameCanonicalPath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func Parse(input string) ([]Action, error) {
	normalized := strings.ReplaceAll(input, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 || lines[0] != beginPatch {
		return nil, fmt.Errorf("the first line of the patch must be %q", beginPatch)
	}
	if lines[len(lines)-1] != endPatch {
		return nil, fmt.Errorf("the last line of the patch must be %q", endPatch)
	}

	var actions []Action
	for i := 1; i < len(lines); {
		line := lines[i]
		if line == endPatch {
			i++
			continue
		}
		if path, ok := strings.CutPrefix(line, addFile); ok {
			if strings.TrimSpace(path) == "" {
				return nil, errors.New("Add File path must be non-empty")
			}
			i++
			var added []string
			for i < len(lines) && !isActionStart(lines[i]) {
				body := lines[i]
				if !strings.HasPrefix(body, "+") {
					return nil, fmt.Errorf("unexpected line in add hunk %q; every line must start with '+'", body)
				}
				added = append(added, body[1:])
				i++
			}
			actions = append(actions, Action{Kind: "add", Path: path, Lines: added})
			continue
		}
		if path, ok := strings.CutPrefix(line, deleteFile); ok {
			if strings.TrimSpace(path) == "" {
				return nil, errors.New("Delete File path must be non-empty")
			}
			actions = append(actions, Action{Kind: "delete", Path: path})
			i++
			continue
		}
		if path, ok := strings.CutPrefix(line, updateFile); ok {
			if strings.TrimSpace(path) == "" {
				return nil, errors.New("Update File path must be non-empty")
			}
			action := Action{Kind: "update", Path: path}
			i++
			if i < len(lines) {
				if dest, ok := strings.CutPrefix(lines[i], moveTo); ok {
					if strings.TrimSpace(dest) == "" {
						return nil, errors.New("Move to path must be non-empty")
					}
					action.MovePath = dest
					i++
				}
			}
			var chunks []Chunk
			haveChunk := false
			for i < len(lines) && !isActionStart(lines[i]) {
				body := lines[i]
				if strings.HasPrefix(body, "@@") {
					chunks = append(chunks, Chunk{Context: strings.TrimSpace(strings.TrimPrefix(body, "@@"))})
					haveChunk = true
					i++
					continue
				}
				if !haveChunk {
					chunks = append(chunks, Chunk{})
					haveChunk = true
				}
				chunk := &chunks[len(chunks)-1]
				switch {
				case body == endOfFile:
					chunk.EndOfFile = true
				case strings.HasPrefix(body, "+"):
					chunk.NewLines = append(chunk.NewLines, body[1:])
				case strings.HasPrefix(body, "-"):
					chunk.OldLines = append(chunk.OldLines, body[1:])
				case body == "":
					chunk.OldLines = append(chunk.OldLines, "")
					chunk.NewLines = append(chunk.NewLines, "")
				case strings.HasPrefix(body, " "):
					text := body[1:]
					chunk.OldLines = append(chunk.OldLines, text)
					chunk.NewLines = append(chunk.NewLines, text)
				default:
					return nil, fmt.Errorf("unexpected line in update hunk %q; lines must start with ' ', '+', or '-'", body)
				}
				i++
			}
			if len(chunks) == 0 {
				return nil, fmt.Errorf("update hunk for %q contains no chunks", path)
			}
			action.Chunks = chunks
			actions = append(actions, action)
			continue
		}
		return nil, fmt.Errorf("unexpected line in patch: %q", line)
	}
	if len(actions) == 0 {
		return nil, errors.New("the patch contains no hunks")
	}
	return actions, nil
}

func ApplyUpdate(original string, chunks []Chunk, path string) (string, error) {
	crlf := usesCRLF(original)
	normalized := strings.ReplaceAll(original, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	replacements, err := computeReplacements(lines, chunks, path)
	if err != nil {
		return "", err
	}
	sort.Slice(replacements, func(i, j int) bool { return replacements[i].start < replacements[j].start })
	result := append([]string(nil), lines...)
	for i := len(replacements) - 1; i >= 0; i-- {
		r := replacements[i]
		end := r.start + r.oldLen
		next := make([]string, 0, len(result)-r.oldLen+len(r.newLines))
		next = append(next, result[:r.start]...)
		next = append(next, r.newLines...)
		next = append(next, result[end:]...)
		result = next
	}
	result = append(result, "")
	joined := strings.Join(result, "\n")
	if crlf {
		joined = strings.ReplaceAll(joined, "\n", "\r\n")
	}
	return joined, nil
}

type replacement struct {
	start    int
	oldLen   int
	newLines []string
}

func computeReplacements(lines []string, chunks []Chunk, path string) ([]replacement, error) {
	var out []replacement
	index := 0
	for _, chunk := range chunks {
		if chunk.Context != "" {
			found := seek(lines, []string{chunk.Context}, index, false)
			if found < 0 {
				return nil, fmt.Errorf("failed to find context %q in %s", chunk.Context, path)
			}
			index = found + 1
		}
		if len(chunk.OldLines) == 0 {
			out = append(out, replacement{start: len(lines), newLines: append([]string(nil), chunk.NewLines...)})
			continue
		}
		pattern := append([]string(nil), chunk.OldLines...)
		newLines := append([]string(nil), chunk.NewLines...)
		found := seek(lines, pattern, index, chunk.EndOfFile)
		if found < 0 && len(pattern) > 0 && pattern[len(pattern)-1] == "" {
			pattern = pattern[:len(pattern)-1]
			if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
				newLines = newLines[:len(newLines)-1]
			}
			found = seek(lines, pattern, index, chunk.EndOfFile)
		}
		if found < 0 {
			return nil, fmt.Errorf("failed to find expected lines in %s:\n%s", path, strings.Join(chunk.OldLines, "\n"))
		}
		out = append(out, replacement{start: found, oldLen: len(pattern), newLines: newLines})
		index = found + len(pattern)
	}
	return out, nil
}

func seek(lines, pattern []string, start int, eof bool) int {
	if len(pattern) == 0 {
		return start
	}
	if len(pattern) > len(lines) {
		return -1
	}
	last := len(lines) - len(pattern)
	searchStart := start
	if eof {
		searchStart = last
	}
	if searchStart < 0 {
		searchStart = 0
	}
	if searchStart > last {
		return -1
	}
	comparators := []func(string, string) bool{
		func(a, b string) bool { return a == b },
		func(a, b string) bool {
			return strings.TrimRightFunc(a, unicode.IsSpace) == strings.TrimRightFunc(b, unicode.IsSpace)
		},
		func(a, b string) bool { return strings.TrimSpace(a) == strings.TrimSpace(b) },
		func(a, b string) bool { return normalizeFuzzy(a) == normalizeFuzzy(b) },
	}
	for _, matches := range comparators {
		for i := searchStart; i <= last; i++ {
			ok := true
			for p := range pattern {
				if !matches(lines[i+p], pattern[p]) {
					ok = false
					break
				}
			}
			if ok {
				return i
			}
		}
	}
	return -1
}

func normalizeFuzzy(text string) string {
	text = strings.TrimSpace(text)
	var b strings.Builder
	for _, r := range text {
		switch {
		case (r >= '\u2010' && r <= '\u2015') || r == '\u2212':
			b.WriteByte('-')
		case r >= '\u2018' && r <= '\u201B':
			b.WriteByte('\'')
		case r >= '\u201C' && r <= '\u201F':
			b.WriteByte('"')
		case r == '\u00A0' || (r >= '\u2000' && r <= '\u200A') || r == '\u202F' || r == '\u205F' || r == '\u3000':
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func renderAdded(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func usesCRLF(contents string) bool {
	crlf := strings.Count(contents, "\r\n")
	if crlf == 0 {
		return false
	}
	lf := strings.Count(contents, "\n")
	return crlf*2 >= lf
}

func isActionStart(line string) bool {
	return strings.HasPrefix(line, addFile) ||
		strings.HasPrefix(line, deleteFile) ||
		strings.HasPrefix(line, updateFile) ||
		line == endPatch
}

func actionLabel(action Action) string {
	switch action.Kind {
	case "add":
		return "Add File: " + action.Path
	case "delete":
		return "Delete File: " + action.Path
	case "update":
		if action.MovePath != "" {
			return fmt.Sprintf("Update File: %s -> %s", action.Path, action.MovePath)
		}
		return "Update File: " + action.Path
	default:
		return action.Kind + ": " + action.Path
	}
}

func ensureParent(path string) error {
	parent := filepath.Dir(path)
	if parent == "" || parent == "." {
		return nil
	}
	return os.MkdirAll(parent, 0o755)
}
