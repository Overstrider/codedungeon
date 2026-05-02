package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/loldinis/codedungeon/internal/db"
)

const (
	VerifyOK      = "OK"
	VerifyMissing = "MISSING"
	VerifyDrifted = "DRIFTED"
)

type Record struct {
	RunID     int64
	Module    string
	OwnerType string
	OwnerID   string
	Phase     string
	Role      string
	Kind      string
	Path      string
	Metadata  map[string]any
}

type Verification struct {
	Artifact db.Artifact `json:"artifact"`
	Status   string      `json:"status"`
	Detail   string      `json:"detail,omitempty"`
}

type Registry struct {
	store *db.Store
	root  string
}

func NewRegistry(store *db.Store, root string) Registry {
	if strings.TrimSpace(root) == "" {
		root, _ = os.Getwd()
	}
	abs, err := filepath.Abs(root)
	if err == nil {
		root = abs
	}
	return Registry{store: store, root: filepath.Clean(root)}
}

func (r Registry) Register(record Record) (db.Artifact, error) {
	if r.store == nil {
		return db.Artifact{}, fmt.Errorf("artifact registry requires a store")
	}
	if err := validateRecord(record); err != nil {
		return db.Artifact{}, err
	}
	absPath, relPath, err := r.resolvePath(record.Path)
	if err != nil {
		return db.Artifact{}, err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return db.Artifact{}, err
	}
	artifactType := "file"
	if info.IsDir() {
		artifactType = "directory"
	}
	metadata := "{}"
	if len(record.Metadata) > 0 {
		body, err := json.Marshal(record.Metadata)
		if err != nil {
			return db.Artifact{}, fmt.Errorf("artifact metadata: %w", err)
		}
		metadata = string(body)
	}
	artifact := db.Artifact{
		RunID:        record.RunID,
		Module:       strings.TrimSpace(record.Module),
		OwnerType:    strings.TrimSpace(record.OwnerType),
		OwnerID:      strings.TrimSpace(record.OwnerID),
		Phase:        strings.TrimSpace(record.Phase),
		Role:         strings.TrimSpace(record.Role),
		Kind:         strings.TrimSpace(record.Kind),
		Path:         relPath,
		AbsPath:      absPath,
		ArtifactType: artifactType,
		MediaType:    mediaTypeForPath(absPath, artifactType),
		MetadataJSON: metadata,
	}
	if artifactType == "file" {
		sum, size, err := hashFile(absPath)
		if err != nil {
			return db.Artifact{}, err
		}
		artifact.SHA256 = sum
		artifact.Bytes = size
	}
	id, err := r.store.RegisterArtifact(artifact)
	if err != nil {
		return db.Artifact{}, err
	}
	artifact.ID = id
	return artifact, nil
}

func (r Registry) RegisterMany(records []Record) ([]db.Artifact, error) {
	out := make([]db.Artifact, 0, len(records))
	for _, record := range records {
		if strings.TrimSpace(record.Path) == "" {
			continue
		}
		artifact, err := r.Register(record)
		if err != nil {
			return out, err
		}
		out = append(out, artifact)
	}
	return out, nil
}

func RegisterIfExists(registry Registry, record Record) error {
	if strings.TrimSpace(record.Path) == "" {
		return nil
	}
	abs, _, resolveErr := registry.resolvePath(record.Path)
	if resolveErr != nil {
		return resolveErr
	}
	if _, err := os.Stat(abs); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	_, err := registry.Register(record)
	return err
}

func (r Registry) VerifyRun(runID int64) ([]Verification, error) {
	if r.store == nil {
		return nil, fmt.Errorf("artifact registry requires a store")
	}
	rows, err := r.store.ArtifactsByRun(runID)
	if err != nil {
		return nil, err
	}
	out := make([]Verification, 0, len(rows))
	for _, artifact := range rows {
		out = append(out, r.verifyArtifact(artifact))
	}
	return out, nil
}

func (r Registry) verifyArtifact(artifact db.Artifact) Verification {
	path := artifact.AbsPath
	if strings.TrimSpace(path) == "" {
		abs, _, err := r.resolvePath(artifact.Path)
		if err == nil {
			path = abs
		} else {
			path = filepath.FromSlash(artifact.Path)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return Verification{Artifact: artifact, Status: VerifyMissing, Detail: err.Error()}
	}
	if artifact.ArtifactType == "directory" {
		if !info.IsDir() {
			return Verification{Artifact: artifact, Status: VerifyDrifted, Detail: "expected directory"}
		}
		return Verification{Artifact: artifact, Status: VerifyOK}
	}
	if info.IsDir() {
		return Verification{Artifact: artifact, Status: VerifyDrifted, Detail: "expected file"}
	}
	sum, size, err := hashFile(path)
	if err != nil {
		return Verification{Artifact: artifact, Status: VerifyMissing, Detail: err.Error()}
	}
	if artifact.Bytes != 0 && artifact.Bytes != size {
		return Verification{Artifact: artifact, Status: VerifyDrifted, Detail: "size mismatch"}
	}
	if artifact.SHA256 != "" && !strings.EqualFold(artifact.SHA256, sum) {
		return Verification{Artifact: artifact, Status: VerifyDrifted, Detail: "sha256 mismatch"}
	}
	return Verification{Artifact: artifact, Status: VerifyOK}
}

func validateRecord(record Record) error {
	required := map[string]string{
		"module":     record.Module,
		"owner_type": record.OwnerType,
		"owner_id":   record.OwnerID,
		"role":       record.Role,
		"kind":       record.Kind,
		"path":       record.Path,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("artifact %s is required", name)
		}
	}
	return nil
}

func (r Registry) resolvePath(path string) (string, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", fmt.Errorf("artifact path is required")
	}
	raw := filepath.FromSlash(path)
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(r.root, raw)
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", "", err
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(r.root, abs)
	if err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		return abs, filepath.ToSlash(rel), nil
	}
	return abs, filepath.ToSlash(filepath.Clean(path)), nil
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func mediaTypeForPath(path, artifactType string) string {
	if artifactType == "directory" {
		return "inode/directory"
	}
	ext := strings.ToLower(filepath.Ext(path))
	if mt := mime.TypeByExtension(ext); mt != "" {
		return mt
	}
	switch ext {
	case ".md":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".log":
		return "text/plain"
	case ".patch", ".diff":
		return "text/x-diff"
	default:
		return "application/octet-stream"
	}
}

func KindForPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "file"
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return "directory"
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".markdown":
		return "markdown"
	case ".json":
		return "json"
	case ".jsonl", ".ndjson":
		return "jsonl"
	case ".log", ".txt":
		return "log"
	case ".patch", ".diff":
		return "patch"
	default:
		return "file"
	}
}
