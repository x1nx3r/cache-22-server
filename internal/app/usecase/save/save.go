package saveusecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/x1nx3r/cache-22-server/internal/entity"
)

// BackupsKept caps rotated conflict backups per slot.
const BackupsKept = 3

type Saves interface {
	Get(ctx context.Context, userID int64, serial string, slot int) (entity.Save, error)
	Upsert(ctx context.Context, s entity.Save) error

	Delete(ctx context.Context, userID int64, serial string, slot int) error
}

type Save struct {
	repo Saves
	dir  string
}

func New(repo Saves, dir string) *Save {
	return &Save{repo: repo, dir: dir}
}

func (s *Save) filePath(userID int64, serial string, slot int) string {
	return filepath.Join(s.dir, fmt.Sprint(userID), serial, fmt.Sprintf("slot%d.ps2", slot))
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Get returns metadata plus file bytes. os.ErrNotExist when never uploaded.
func (s *Save) Get(ctx context.Context, userID int64, serial string, slot int) (entity.Save, []byte, error) {
	meta, err := s.repo.Get(ctx, userID, serial, slot)
	if err != nil {
		return meta, nil, err
	}
	raw, err := os.ReadFile(s.filePath(userID, serial, slot))
	if err != nil {
		return meta, nil, err
	}
	return meta, raw, nil
}

// Put stores bytes, rotating the previous version into a timestamped backup
// (keep-last-3) when the content actually changed.
func (s *Save) Put(ctx context.Context, userID int64, serial string, slot int, data []byte) (entity.Save, error) {
	path := s.filePath(userID, serial, slot)
	if old, err := os.ReadFile(path); err == nil && hashBytes(old) != hashBytes(data) {
		bak := fmt.Sprintf("%s.%d.bak", path, time.Now().UnixNano())
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return entity.Save{}, err
		}
		if err := os.WriteFile(bak, old, 0o644); err != nil {
			return entity.Save{}, err
		}
		s.pruneBackups(path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return entity.Save{}, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return entity.Save{}, err
	}
	meta := entity.Save{UserID: userID, Serial: serial, Slot: slot,
		SHA256: hashBytes(data), Size: int64(len(data))}
	if err := s.repo.Upsert(ctx, meta); err != nil {
		return entity.Save{}, err
	}
	return s.repo.Get(ctx, userID, serial, slot)
}

func (s *Save) pruneBackups(path string) {
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return
	}
	base := filepath.Base(path) + "."
	var baks []string
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > len(base) && e.Name()[:len(base)] == base {
			baks = append(baks, filepath.Join(filepath.Dir(path), e.Name()))
		}
	}
	sort.Strings(baks)
	for len(baks) > BackupsKept {
		os.Remove(baks[0])
		baks = baks[1:]
	}
}

// Delete removes the save, its backups and its metadata row.
func (s *Save) Delete(ctx context.Context, userID int64, serial string, slot int) error {
	path := s.filePath(userID, serial, slot)
	os.Remove(path)
	entries, _ := os.ReadDir(filepath.Dir(path))
	base := filepath.Base(path) + "."
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), base) {
			os.Remove(filepath.Join(filepath.Dir(path), e.Name()))
		}
	}
	return s.repo.Delete(ctx, userID, serial, slot)
}
