//go:build !windows

package workspace

// On Unix, EvalSymlinks already yields the on-disk path identity needed for
// workspace containment. Windows also needs to resolve short-name aliases.
func canonicalExistingPath(path string) (string, error) {
	return NormalizePathIdentity(path), nil
}
