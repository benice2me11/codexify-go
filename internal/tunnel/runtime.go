package tunnel

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/benice2me11/codexify-go/internal/config"
)

const (
	ClientVersion      = "0.0.15"
	releaseBase        = "https://github.com/openai/tunnel-client/releases/download/v0.0.15"
	maxDownloadBytes   = 100 * 1024 * 1024
	maxBinaryBytes     = 64 * 1024 * 1024
	installManifestVer = 1
)

var installMu sync.Mutex

type releaseAsset struct {
	ArchiveName   string
	BinaryName    string
	ArchiveSHA256 string
}

type installManifest struct {
	Version             int    `json:"version"`
	TunnelClientVersion string `json:"tunnelClientVersion"`
	Asset               string `json:"asset"`
	ArchiveSHA256       string `json:"archiveSha256"`
	BinarySHA256        string `json:"binarySha256"`
}

type RuntimeStatus struct {
	Managed   bool   `json:"managed"`
	Version   string `json:"version"`
	Path      string `json:"path"`
	Installed bool   `json:"installed"`
	Verified  bool   `json:"verified"`
	Detail    string `json:"detail,omitempty"`
}

func ResolveExecutable(ctx context.Context, cfg config.TunnelConfig) (string, error) {
	if explicit := strings.TrimSpace(cfg.Executable); explicit != "" {
		return explicit, nil
	}
	return EnsureManaged(ctx, cfg.ManagedDir)
}

func EnsureManaged(ctx context.Context, base string) (string, error) {
	installMu.Lock()
	defer installMu.Unlock()

	asset, err := currentReleaseAsset()
	if err != nil {
		return "", err
	}
	binaryPath, manifestPath, err := managedPaths(base, asset)
	if err != nil {
		return "", err
	}
	binaryExists := fileExists(binaryPath)
	manifestExists := fileExists(manifestPath)
	switch {
	case binaryExists && manifestExists:
		if err := validateManagedInstall(ctx, binaryPath, manifestPath, asset); err != nil {
			return "", err
		}
		return binaryPath, nil
	case binaryExists != manifestExists:
		return "", fmt.Errorf("incomplete managed tunnel runtime under %s; remove that version directory and retry", filepath.Dir(binaryPath))
	}

	if err := installManaged(ctx, binaryPath, manifestPath, asset); err != nil {
		return "", err
	}
	return binaryPath, nil
}

func ManagedStatus(ctx context.Context, base string) RuntimeStatus {
	status := RuntimeStatus{Managed: true, Version: ClientVersion}
	asset, err := currentReleaseAsset()
	if err != nil {
		status.Detail = err.Error()
		return status
	}
	binaryPath, manifestPath, err := managedPaths(base, asset)
	if err != nil {
		status.Detail = err.Error()
		return status
	}
	status.Path = binaryPath
	status.Installed = fileExists(binaryPath) && fileExists(manifestPath)
	if !status.Installed {
		status.Detail = "managed tunnel runtime is not installed"
		return status
	}
	if err := validateManagedInstall(ctx, binaryPath, manifestPath, asset); err != nil {
		status.Detail = err.Error()
		return status
	}
	status.Verified = true
	return status
}

func ValidateClient(ctx context.Context, path string, exactVersion string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("tunnel runtime does not exist: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("tunnel runtime is not a regular file")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(probeCtx, path, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("tunnel runtime version check failed: %w: %s", err, sanitizeOutput(out))
	}
	versionText := string(out)
	if exactVersion != "" && !strings.Contains(versionText, exactVersion) {
		return fmt.Errorf("managed tunnel runtime reports unexpected version: %s", sanitizeOutput(out))
	}
	helpCtx, helpCancel := context.WithTimeout(ctx, 10*time.Second)
	defer helpCancel()
	help, err := exec.CommandContext(helpCtx, path, "run", "--help").CombinedOutput()
	if err != nil {
		return fmt.Errorf("tunnel runtime compatibility check failed: %w: %s", err, sanitizeOutput(help))
	}
	required := []string{
		"--control-plane.tunnel-id",
		"--mcp.server-url",
		"--mcp.extra-headers",
		"--mcp.discovery-extra-headers",
		"--health.url-file",
	}
	helpText := string(help)
	for _, flag := range required {
		if !strings.Contains(helpText, flag) {
			return fmt.Errorf("tunnel runtime is missing required flag %s", flag)
		}
	}
	return nil
}

func installManaged(ctx context.Context, binaryPath, manifestPath string, asset releaseAsset) error {
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o700); err != nil {
		return err
	}
	archiveURL := releaseBase + "/" + asset.ArchiveName
	archive, err := fetchRelease(ctx, archiveURL)
	if err != nil {
		return err
	}
	archiveHash := sha256Hex(archive)
	if archiveHash != asset.ArchiveSHA256 {
		return errors.New("OpenAI tunnel runtime archive does not match the pinned SHA-256")
	}
	binary, err := extractBinary(archive, asset.BinaryName)
	if err != nil {
		return err
	}
	if err := atomicWrite(binaryPath, binary, 0o700); err != nil {
		return err
	}
	if err := ValidateClient(ctx, binaryPath, ClientVersion); err != nil {
		_ = os.Remove(binaryPath)
		return err
	}
	manifest := installManifest{
		Version:             installManifestVer,
		TunnelClientVersion: ClientVersion,
		Asset:               asset.ArchiveName,
		ArchiveSHA256:       archiveHash,
		BinarySHA256:        sha256Hex(binary),
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = os.Remove(binaryPath)
		return err
	}
	if err := atomicWrite(manifestPath, append(data, '\n'), 0o600); err != nil {
		_ = os.Remove(binaryPath)
		return err
	}
	return nil
}

func validateManagedInstall(ctx context.Context, binaryPath, manifestPath string, asset releaseAsset) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read managed tunnel manifest: %w", err)
	}
	var manifest installManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parse managed tunnel manifest: %w", err)
	}
	if manifest.Version != installManifestVer ||
		manifest.TunnelClientVersion != ClientVersion ||
		manifest.Asset != asset.ArchiveName ||
		manifest.ArchiveSHA256 != asset.ArchiveSHA256 {
		return errors.New("managed tunnel manifest does not match this codexify-go build")
	}
	binary, err := os.ReadFile(binaryPath)
	if err != nil {
		return fmt.Errorf("read managed tunnel runtime: %w", err)
	}
	if sha256Hex(binary) != manifest.BinarySHA256 {
		return errors.New("managed tunnel runtime failed its integrity check")
	}
	return ValidateClient(ctx, binaryPath, ClientVersion)
}

func fetchRelease(ctx context.Context, rawURL string) ([]byte, error) {
	client := &http.Client{
		Timeout: 120 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "https" || !allowedReleaseHost(req.URL.Hostname()) {
				return errors.New("unexpected tunnel release redirect")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download OpenAI tunnel runtime: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download OpenAI tunnel runtime: HTTP %s", resp.Status)
	}
	if resp.ContentLength > maxDownloadBytes {
		return nil, errors.New("OpenAI tunnel runtime download exceeds 100 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDownloadBytes {
		return nil, errors.New("OpenAI tunnel runtime download exceeds 100 MiB")
	}
	return data, nil
}

func allowedReleaseHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "github.com" ||
		host == "release-assets.githubusercontent.com" ||
		strings.HasSuffix(host, ".githubusercontent.com")
}

func extractBinary(archive []byte, binaryName string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("open tunnel runtime ZIP: %w", err)
	}
	for _, entry := range reader.File {
		if entry.Name != binaryName {
			continue
		}
		if entry.FileInfo().IsDir() || entry.UncompressedSize64 > maxBinaryBytes {
			return nil, errors.New("tunnel runtime ZIP binary is not a bounded regular file")
		}
		rc, err := entry.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(rc, maxBinaryBytes+1))
		closeErr := rc.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(data) > maxBinaryBytes {
			return nil, errors.New("tunnel runtime binary exceeds 64 MiB")
		}
		return data, nil
	}
	return nil, fmt.Errorf("release ZIP does not contain %s", binaryName)
}

func currentReleaseAsset() (releaseAsset, error) {
	osName := runtime.GOOS
	if osName == "darwin" {
		osName = "darwin"
	}
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
	case "arm64":
	default:
		return releaseAsset{}, fmt.Errorf("no pinned tunnel runtime for architecture %s", runtime.GOARCH)
	}
	var hash string
	switch osName + "/" + arch {
	case "darwin/amd64":
		hash = "2d3a2b3a985ad2fcfddc4a82a0caa6624ee9383e7d85e82563bf1fe3ce905794"
	case "darwin/arm64":
		hash = "e416ea9ea13e1b8be0d0a355fbd28143cfa55fe5a32b2986fce1a516d7b5e2ad"
	case "linux/amd64":
		hash = "f26f8b3ee6c335e38fa5cfbe6ce5635f53738f08a26eecf07d6cebacab4a1abf"
	case "linux/arm64":
		hash = "a868d295385b22449341fa141b911f3e991583e45b1fa2a5bfb946aed1861b88"
	case "windows/amd64":
		hash = "aa5ddb14dddd602fa59f3e6f4401aa8a79a218e341466226b7434127dff65dbc"
	case "windows/arm64":
		hash = "3c610dd27760987b11285670b35faa36fec8460e68d64a4cdda4342ce6b8e8be"
	default:
		return releaseAsset{}, fmt.Errorf("no pinned tunnel runtime for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	archiveName := fmt.Sprintf("tunnel-client-runtime-v%s-%s-%s.zip", ClientVersion, osName, arch)
	binaryName := "tunnel-client-runtime"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	return releaseAsset{ArchiveName: archiveName, BinaryName: binaryName, ArchiveSHA256: hash}, nil
}

func managedPaths(base string, asset releaseAsset) (string, string, error) {
	if strings.TrimSpace(base) == "" {
		return "", "", errors.New("managed tunnel directory is empty")
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(abs, "v"+ClientVersion)
	return filepath.Join(dir, asset.BinaryName), filepath.Join(dir, "manifest.json"), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sanitizeOutput(data []byte) string {
	value := strings.TrimSpace(string(data))
	if len(value) > 2000 {
		value = value[:2000]
	}
	return value
}
