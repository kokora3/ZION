package objects

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kokora3/zion/internal/protocol"
)

const DefaultQuotaBytes uint64 = 1 << 30 // 1 GiB local operational default.

var (
	ErrObjectNotFound = errors.New("object not found")
	ErrObjectCorrupt  = errors.New("object corrupt")
	ErrStoreFull      = errors.New("object store quota exceeded")
)

type PutResult string

const (
	PutStored        PutResult = "STORED"
	PutAlreadyExists PutResult = "ALREADY_EXISTS"
)

type Metadata struct {
	ObjectID protocol.ObjectID
	Size     uint64
	StoredAt time.Time
}

type Store struct {
	root       string
	quotaBytes uint64

	mu          sync.Mutex
	objectBytes uint64
	objectCount uint64
}

// Open creates or opens a store. A zero quota selects DefaultQuotaBytes.
func Open(root string, quotaBytes uint64) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("object store directory is empty")
	}
	if quotaBytes == 0 {
		quotaBytes = DefaultQuotaBytes
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve object store directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(abs, "sha256"), 0o700); err != nil {
		return nil, fmt.Errorf("create object store: %w", err)
	}
	s := &Store{root: abs, quotaBytes: quotaBytes}
	if err := s.scanUsage(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) scanUsage() error {
	base := filepath.Join(s.root, "sha256")
	return filepath.WalkDir(base, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("scan object store: %w", walkErr)
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".obj") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat stored object: %w", err)
		}
		if !info.Mode().IsRegular() || info.Size() < 0 {
			return nil
		}
		s.objectCount++
		if uint64(info.Size()) > ^uint64(0)-s.objectBytes {
			return fmt.Errorf("object store usage overflow")
		}
		s.objectBytes += uint64(info.Size())
		return nil
	})
}

func (s *Store) pathForID(id protocol.ObjectID) (string, error) {
	if err := id.Validate(); err != nil || id.Algorithm != protocol.HashAlgorithmSHA256 {
		if err == nil {
			err = fmt.Errorf("unsupported object hash algorithm")
		}
		return "", fmt.Errorf("invalid object ID: %w", err)
	}
	digest := fmt.Sprintf("%x", id.Digest)
	if len(digest) != 64 {
		return "", fmt.Errorf("invalid object ID digest")
	}
	return filepath.Join(s.root, "sha256", digest[:2], digest[2:4], digest+".obj"), nil
}

func (s *Store) Put(ctx context.Context, object Object) (protocol.ObjectID, PutResult, error) {
	if err := contextErr(ctx); err != nil {
		return protocol.ObjectID{}, "", err
	}
	canonical, err := object.CanonicalBytes()
	if err != nil {
		return protocol.ObjectID{}, "", err
	}
	id, err := object.Core.ObjectID()
	if err != nil {
		return protocol.ObjectID{}, "", fmt.Errorf("derive object ID: %w", err)
	}
	path, err := s.pathForID(id)
	if err != nil {
		return protocol.ObjectID{}, "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextErr(ctx); err != nil {
		return protocol.ObjectID{}, "", err
	}
	if _, err := os.Stat(path); err == nil {
		if _, verifyErr := readAndVerify(path, id); verifyErr != nil {
			return protocol.ObjectID{}, "", verifyErr
		}
		return id, PutAlreadyExists, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return protocol.ObjectID{}, "", fmt.Errorf("stat object: %w", err)
	}
	size := uint64(len(canonical))
	if size > s.quotaBytes || s.objectBytes > s.quotaBytes-size {
		return protocol.ObjectID{}, "", ErrStoreFull
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return protocol.ObjectID{}, "", fmt.Errorf("create object shard: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".put-*.tmp")
	if err != nil {
		return protocol.ObjectID{}, "", fmt.Errorf("create object temp file: %w", err)
	}
	tmpName := tmp.Name()
	keepTemp := true
	defer func() {
		if keepTemp {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return protocol.ObjectID{}, "", fmt.Errorf("secure object temp file: %w", err)
	}
	if _, err := tmp.Write(canonical); err != nil {
		_ = tmp.Close()
		return protocol.ObjectID{}, "", fmt.Errorf("write object: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return protocol.ObjectID{}, "", fmt.Errorf("sync object: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return protocol.ObjectID{}, "", fmt.Errorf("close object: %w", err)
	}
	if err := contextErr(ctx); err != nil {
		return protocol.ObjectID{}, "", err
	}
	if err := os.Rename(tmpName, path); err != nil {
		if _, verifyErr := readAndVerify(path, id); verifyErr == nil {
			return id, PutAlreadyExists, nil
		}
		return protocol.ObjectID{}, "", fmt.Errorf("install object: %w", err)
	}
	keepTemp = false
	s.objectBytes += size
	s.objectCount++
	_ = syncDirectory(filepath.Dir(path))
	return id, PutStored, nil
}

func (s *Store) Get(ctx context.Context, id protocol.ObjectID) (Object, error) {
	if err := contextErr(ctx); err != nil {
		return Object{}, err
	}
	path, err := s.pathForID(id)
	if err != nil {
		return Object{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return readAndVerify(path, id)
}

func (s *Store) Has(ctx context.Context, id protocol.ObjectID) (bool, error) {
	_, err := s.Get(ctx, id)
	if errors.Is(err, ErrObjectNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) Stat(ctx context.Context, id protocol.ObjectID) (Metadata, error) {
	if err := contextErr(ctx); err != nil {
		return Metadata{}, err
	}
	path, err := s.pathForID(id)
	if err != nil {
		return Metadata{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	object, err := readAndVerify(path, id)
	if err != nil {
		return Metadata{}, err
	}
	canonical, err := object.CanonicalBytes()
	if err != nil {
		return Metadata{}, fmt.Errorf("%w: %v", ErrObjectCorrupt, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return Metadata{}, fmt.Errorf("stat object: %w", err)
	}
	return Metadata{ObjectID: id, Size: uint64(len(canonical)), StoredAt: info.ModTime().UTC()}, nil
}

// Usage reports local operational accounting. It is never canonical state.
func (s *Store) Usage() (objectCount, objectBytes, quotaBytes uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.objectCount, s.objectBytes, s.quotaBytes
}

func readAndVerify(path string, requested protocol.ObjectID) (Object, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Object{}, ErrObjectNotFound
	}
	if err != nil {
		return Object{}, fmt.Errorf("open object: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Object{}, fmt.Errorf("stat object: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > MaxObjectBytes {
		return Object{}, fmt.Errorf("%w: invalid stored file size", ErrObjectCorrupt)
	}
	raw, err := io.ReadAll(io.LimitReader(file, MaxObjectBytes+1))
	if err != nil {
		return Object{}, fmt.Errorf("read object: %w", err)
	}
	if len(raw) > MaxObjectBytes || int64(len(raw)) != info.Size() {
		return Object{}, fmt.Errorf("%w: stored file changed or exceeds limit", ErrObjectCorrupt)
	}
	object, err := Decode(raw)
	if err != nil {
		return Object{}, fmt.Errorf("%w: %v", ErrObjectCorrupt, err)
	}
	actual, err := object.Core.ObjectID()
	if err != nil || actual.String() != requested.String() {
		return Object{}, fmt.Errorf("%w: content does not match requested ObjectID", ErrObjectCorrupt)
	}
	return object, nil
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}
	return ctx.Err()
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
