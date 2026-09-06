package saveusecase

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/x1nx3r/cache-22-server/internal/entity"
)

type memStore struct {
	rows map[string]entity.Save
}

func mkey(u int64, s string, slot int) string {
	return string(rune(u)) + s + string(rune(slot))
}

func (m *memStore) Get(_ context.Context, u int64, s string, slot int) (entity.Save, error) {
	r, ok := m.rows[mkey(u, s, slot)]
	if !ok {
		return entity.Save{}, os.ErrNotExist
	}
	return r, nil
}

func (m *memStore) Upsert(_ context.Context, s entity.Save) error {
	m.rows[mkey(s.UserID, s.Serial, s.Slot)] = s
	return nil
}

func TestPutRotatesBackups(t *testing.T) {
	dir := t.TempDir()
	svc := New(&memStore{rows: map[string]entity.Save{}}, dir)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		data := []byte{byte(i), 1, 2, 3}
		if _, err := svc.Put(ctx, 7, "G", 1, data); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(dir, "7", "G"))
	if err != nil {
		t.Fatal(err)
	}
	baks := 0
	for _, e := range entries {
		if len(e.Name()) > 4 && e.Name()[len(e.Name())-4:] == ".bak" {
			baks++
		}
	}
	if baks != BackupsKept {
		t.Errorf("backups = %d, want %d", baks, BackupsKept)
	}
	meta, raw, err := svc.Get(ctx, 7, "G", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 4 || raw[0] != 4 || meta.Size != 4 || meta.SHA256 == "" {
		t.Errorf("latest = %+v %v", meta, raw)
	}
}

func TestGetMissing(t *testing.T) {
	svc := New(&memStore{rows: map[string]entity.Save{}}, t.TempDir())
	if _, _, err := svc.Get(context.Background(), 7, "NOPE", 1); err == nil {
		t.Error("missing save must error")
	}
}
