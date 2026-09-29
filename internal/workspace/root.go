package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Root struct {
	path string
	real string
}

func Canonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = NormalizePathIdentity(abs)
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return NormalizePathIdentity(resolved), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	ancestor := abs
	var suffix []string
	for {
		if _, statErr := os.Lstat(ancestor); statErr == nil {
			break
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", err
		}
		suffix = append([]string{filepath.Base(ancestor)}, suffix...)
		ancestor = parent
	}
	realAncestor, evalErr := filepath.EvalSymlinks(ancestor)
	if evalErr != nil {
		return "", evalErr
	}
	out := realAncestor
	for _, part := range suffix {
		out = filepath.Join(out, part)
	}
	return NormalizePathIdentity(out), nil
}

// NormalizePathIdentity collapses platform-specific aliases that refer to the
// same filesystem path. Windows canonicalization may return extended-length
// device paths while Git and user input use ordinary DOS/UNC paths.
func NormalizePathIdentity(path string) string {
	clean := filepath.Clean(path)
	if runtime.GOOS != "windows" {
		return clean
	}
	upper := strings.ToUpper(clean)
	const extendedUNC = `\\?\UNC\`
	const deviceUNC = `\\.\UNC\`
	if strings.HasPrefix(upper, extendedUNC) {
		return filepath.Clean(`\\` + clean[len(extendedUNC):])
	}
	if strings.HasPrefix(upper, deviceUNC) {
		return filepath.Clean(`\\` + clean[len(deviceUNC):])
	}
	for _, prefix := range []string{`\\?\`, `\\.\`} {
		if strings.HasPrefix(clean, prefix) {
			rest := clean[len(prefix):]
			if len(rest) >= 3 &&
				((rest[0] >= 'A' && rest[0] <= 'Z') || (rest[0] >= 'a' && rest[0] <= 'z')) &&
				rest[1] == ':' && os.IsPathSeparator(rest[2]) {
				return filepath.Clean(rest)
			}
		}
	}
	return clean
}

func New(path string) (*Root, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("workspace root is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat workspace root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace root is not a directory: %s", abs)
	}
	real, err := Canonical(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root symlinks: %w", err)
	}
	real = filepath.Clean(real)
	return &Root{path: real, real: real}, nil
}

func (r *Root) Path() string { return r.path }

func (r *Root) Resolve(rel string, allowMissing bool) (string, error) {
	if rel == "" || rel == "." {
		return r.real, nil
	}
	if filepath.IsAbs(rel) {
		return "", errors.New("path must be workspace-relative")
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes workspace")
	}
	candidate := filepath.Join(r.real, clean)
	if err := r.ensureInside(candidate); err != nil {
		return "", err
	}

	resolved, err := filepath.EvalSymlinks(candidate)
	if err == nil {
		if err := r.ensureInside(resolved); err != nil {
			return "", err
		}
		return resolved, nil
	}
	if !allowMissing || !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	ancestor := candidate
	var suffix []string
	for {
		if ancestor == r.real {
			break
		}
		if _, statErr := os.Lstat(ancestor); statErr == nil {
			break
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", errors.New("could not find existing workspace ancestor")
		}
		suffix = append([]string{filepath.Base(ancestor)}, suffix...)
		ancestor = parent
	}

	realAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	if err := r.ensureInside(realAncestor); err != nil {
		return "", err
	}
	resolved = realAncestor
	for _, part := range suffix {
		resolved = filepath.Join(resolved, part)
	}
	if err := r.ensureInside(resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func (r *Root) Relative(abs string) (string, error) {
	abs = NormalizePathIdentity(abs)
	if err := r.ensureInside(abs); err != nil {
		return "", err
	}
	rel, err := filepath.Rel(r.real, abs)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return ".", nil
	}
	return filepath.ToSlash(rel), nil
}

func (r *Root) ensureInside(path string) error {
	rel, err := filepath.Rel(r.real, NormalizePathIdentity(path))
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("path escapes workspace: %s", path)
	}
	return nil
}
