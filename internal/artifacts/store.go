package artifacts

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/benice2me11/codexify-go/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const Prefix = "codexify-go://artifact/"

type Store struct {
	cfg  config.ArtifactEgressConfig
	root string
	mu   sync.Mutex
}

type Receipt struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	MIMEType string `json:"mimeType"`
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
	Durable  bool   `json:"durable"`
	ChatLink string `json:"chatLink"`
}

// Owner binds a grouped export to the exact conversation and checkout epoch.
// All fields must originate from validated server-side workspace resolution,
// never from user-supplied artifact arguments.
type Owner struct {
	Conversation     string `json:"conversation"`
	Group            string `json:"group"`
	Repository       string `json:"repository"`
	ProjectRoot      string `json:"projectRoot"`
	Fingerprint      string `json:"membershipFingerprint"`
	BindingEpoch     string `json:"bindingEpoch"`
	CheckoutRevision uint64 `json:"checkoutRevision"`
}

type record struct {
	Version        int    `json:"version"`
	Token          string `json:"token"`
	Name           string `json:"name"`
	MIMEType       string `json:"mimeType"`
	Bytes          int64  `json:"bytes"`
	SHA256         string `json:"sha256"`
	CreatedAt      string `json:"createdAt"`
	Snapshot       string `json:"snapshot,omitempty"`
	SourceRoot     string `json:"sourceRoot,omitempty"`
	SourceRelative string `json:"sourceRelative,omitempty"`
	Owner          *Owner `json:"owner,omitempty"`
}

func New(cfg config.ArtifactEgressConfig) (*Store, error) {
	root := cfg.Dir
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return nil, errors.New("cannot resolve user home for artifact store")
		}
		root = filepath.Join(home, ".codexify-go", "artifacts")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Store{cfg: cfg, root: filepath.Clean(abs)}, nil
}

func (s *Store) Enabled() bool { return s.cfg.Enabled }

func (s *Store) Export(root *workspace.Root, rel string) (*mcp.ResourceLink, Receipt, error) {
	return s.ExportScoped(root, rel, nil)
}

// ExportScoped creates a protected group resource when owner is nonnil.
// Legacy unscoped exports retain their established token-capability behavior.
func (s *Store) ExportScoped(root *workspace.Root, rel string, owner *Owner) (*mcp.ResourceLink, Receipt, error) {
	if !s.cfg.Enabled {
		return nil, Receipt{}, errors.New("artifact egress is disabled by configuration")
	}
	if root == nil {
		return nil, Receipt{}, errors.New("artifact export requires an active workspace")
	}
	if owner != nil {
		if owner.Conversation == "" || owner.Group == "" || owner.Repository == "" ||
			owner.Fingerprint == "" || owner.BindingEpoch == "" ||
			owner.CheckoutRevision == 0 ||
			owner.ProjectRoot != root.Path() {
			return nil, Receipt{}, errors.New("artifact owner must identify the validated conversation and source checkout epoch")
		}
	}
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return nil, Receipt{}, errors.New("path must be a non-empty workspace-relative file path")
	}
	path, err := root.Resolve(rel, false)
	if err != nil {
		return nil, Receipt{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, Receipt{}, err
	}
	if !info.Mode().IsRegular() {
		return nil, Receipt{}, errors.New("export path must be a regular file")
	}
	if info.Size() > s.cfg.MaxFileBytes {
		return nil, Receipt{}, fmt.Errorf("file is %d bytes; artifactEgress.maxFileBytes is %d", info.Size(), s.cfg.MaxFileBytes)
	}

	data, err := readBounded(path, s.cfg.MaxFileBytes)
	if err != nil {
		return nil, Receipt{}, err
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	relative, err := root.Relative(path)
	if err != nil {
		return nil, Receipt{}, err
	}
	token, err := randomToken()
	if err != nil {
		return nil, Receipt{}, err
	}
	uri := Prefix + token

	rec := record{
		Version:        1,
		Token:          token,
		Name:           filepath.Base(path),
		MIMEType:       mimeType,
		Bytes:          int64(len(data)),
		SHA256:         digest,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		SourceRoot:     root.Path(),
		SourceRelative: relative,
	}
	if owner != nil {
		copyOwner := *owner
		rec.Owner = &copyOwner
		rec.Version = 2
	}
	durable := int64(len(data)) <= s.cfg.SnapshotMaxFileBytes &&
		int64(len(data)) <= s.cfg.MaxSnapshotBytes &&
		s.cfg.MaxSnapshotBytes > 0
	if !durable && !s.cfg.FallbackToSource {
		return nil, Receipt{}, errors.New("artifact snapshot is unavailable and artifactEgress.fallbackToSource is false")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return nil, Receipt{}, err
	}
	if err := s.pruneLocked(int64(len(data)), durable); err != nil {
		return nil, Receipt{}, err
	}
	if durable {
		rec.Snapshot = token + ".blob"
		if err := writeAtomic(filepath.Join(s.root, rec.Snapshot), data, 0o600); err != nil {
			return nil, Receipt{}, err
		}
	}
	meta, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		if durable {
			_ = os.Remove(filepath.Join(s.root, rec.Snapshot))
		}
		return nil, Receipt{}, err
	}
	if err := writeAtomic(s.recordPath(token), append(meta, '\n'), 0o600); err != nil {
		if durable {
			_ = os.Remove(filepath.Join(s.root, rec.Snapshot))
		}
		return nil, Receipt{}, err
	}

	size := rec.Bytes
	link := &mcp.ResourceLink{
		URI:      uri,
		Name:     rec.Name,
		MIMEType: rec.MIMEType,
		Size:     &size,
	}
	receipt := Receipt{
		Path:     relative,
		Name:     rec.Name,
		MIMEType: rec.MIMEType,
		Bytes:    rec.Bytes,
		SHA256:   rec.SHA256,
		Durable:  durable,
		ChatLink: uri,
	}
	return link, receipt, nil
}

func (s *Store) Read(uri string) (*mcp.ReadResourceResult, error) {
	return s.ReadScoped(uri, nil)
}

// ReadScoped never discloses a grouped artifact unless the caller proves the
// original conversation/group/member/checkout binding. A raw resource URI is
// deliberately insufficient for grouped exports.
func (s *Store) ReadScoped(uri string, caller *Owner) (*mcp.ReadResourceResult, error) {
	return s.ReadAuthorized(uri, func(recorded Owner) bool {
		return caller != nil && *caller == recorded
	})
}

// ReadAuthorized validates a resource's owner within the same locked record
// read that serves its bytes. The callback must independently authenticate
// and revalidate the member binding at request time; it must not trust URI
// contents or an alias presented directly by the requester.
func (s *Store) ReadAuthorized(uri string, authorize func(Owner) bool) (*mcp.ReadResourceResult, error) {
	if !strings.HasPrefix(uri, Prefix) {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	token := strings.TrimPrefix(uri, Prefix)
	if len(token) != 32 || strings.IndexFunc(token, func(r rune) bool {
		return !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f'))
	}) >= 0 {
		return nil, mcp.ResourceNotFoundError(uri)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.recordPath(token))
	if errors.Is(err, os.ErrNotExist) {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	if err != nil {
		return nil, err
	}
	var rec record
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	if rec.Token != token {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	if rec.Owner != nil {
		if authorize == nil || !authorize(*rec.Owner) {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		// Group snapshots, unlike legacy durable exports, have a bounded
		// lifetime. This applies equally to blobs and source fallbacks.
		if s.groupRecordExpired(rec) {
			_ = os.Remove(s.recordPath(token))
			if rec.Snapshot != "" {
				_ = os.Remove(filepath.Join(s.root, filepath.Base(rec.Snapshot)))
			}
			return nil, mcp.ResourceNotFoundError(uri)
		}
	}
	var body []byte
	if rec.Snapshot != "" {
		body, err = readBounded(filepath.Join(s.root, filepath.Base(rec.Snapshot)), s.cfg.MaxFileBytes)
	} else {
		created, parseErr := time.Parse(time.RFC3339Nano, rec.CreatedAt)
		if parseErr != nil || time.Since(created) > s.cfg.ReferenceTTL.Duration() {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		root, rootErr := workspace.New(rec.SourceRoot)
		if rootErr != nil {
			return nil, fmt.Errorf("artifact source workspace is unavailable: %w", rootErr)
		}
		path, resolveErr := root.Resolve(filepath.FromSlash(rec.SourceRelative), false)
		if resolveErr != nil {
			return nil, resolveErr
		}
		body, err = readBounded(path, s.cfg.MaxFileBytes)
	}
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
		URI:      uri,
		MIMEType: rec.MIMEType,
		Blob:     body,
	}}}, nil
}

func (s *Store) recordPath(token string) string {
	return filepath.Join(s.root, token+".json")
}

func (s *Store) pruneLocked(incoming int64, durable bool) error {
	entries, err := os.ReadDir(s.root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	type item struct {
		rec  record
		path string
		when time.Time
	}
	var records []item
	var snapshotBytes int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(s.root, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var rec record
		if json.Unmarshal(data, &rec) != nil {
			continue
		}
		if s.groupRecordExpired(rec) {
			_ = os.Remove(path)
			if rec.Snapshot != "" {
				_ = os.Remove(filepath.Join(s.root, filepath.Base(rec.Snapshot)))
			}
			continue
		}
		when, _ := time.Parse(time.RFC3339Nano, rec.CreatedAt)
		records = append(records, item{rec: rec, path: path, when: when})
		if rec.Snapshot != "" {
			snapshotBytes += rec.Bytes
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].when.Before(records[j].when) })

	removeOldest := func() bool {
		if len(records) == 0 {
			return false
		}
		oldest := records[0]
		records = records[1:]
		_ = os.Remove(oldest.path)
		if oldest.rec.Snapshot != "" {
			_ = os.Remove(filepath.Join(s.root, filepath.Base(oldest.rec.Snapshot)))
			snapshotBytes -= oldest.rec.Bytes
		}
		return true
	}
	for len(records) >= s.cfg.MaxReferences {
		if !removeOldest() {
			break
		}
	}
	if durable {
		for snapshotBytes+incoming > s.cfg.MaxSnapshotBytes {
			if !removeOldest() {
				break
			}
		}
		if snapshotBytes+incoming > s.cfg.MaxSnapshotBytes {
			return errors.New("artifact snapshot budget could not be satisfied")
		}
	}
	return nil
}

func (s *Store) groupRecordExpired(rec record) bool {
	if rec.Owner == nil {
		return false
	}
	created, err := time.Parse(time.RFC3339Nano, rec.CreatedAt)
	return err != nil || s.cfg.ReferenceTTL.Duration() <= 0 ||
		time.Since(created) > s.cfg.ReferenceTTL.Duration()
}

func readBounded(path string, max int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := io.LimitReader(file, max+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("file exceeds %d-byte artifact limit", max)
	}
	return data, nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := fmt.Sprintf("%s.tmp.%d", path, time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func randomToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
