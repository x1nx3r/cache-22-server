package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cache-22/cache-22-server/internal/entity"
)

type fakeRepo struct {
	stored map[string]entity.Game
	err    error
}

func (f *fakeRepo) List(_ context.Context) ([]entity.Game, error) { return nil, nil }

func (f *fakeRepo) GetBySerial(_ context.Context, _ string) (entity.Game, error) {
	return entity.Game{}, nil
}

func (f *fakeRepo) Upsert(_ context.Context, g entity.Game) error {
	if f.err != nil {
		return f.err
	}
	if f.stored == nil {
		f.stored = map[string]entity.Game{}
	}
	f.stored[g.Serial] = g
	return nil
}

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(make([]byte, size)); err != nil {
		t.Fatal(err)
	}
}

func TestRunScansImagesOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SLUS-20312.iso"), 100)
	writeFile(t, filepath.Join(dir, "SCES-50000.chd"), 200)
	writeFile(t, filepath.Join(dir, "notes.txt"), 50)
	writeFile(t, filepath.Join(dir, "GAME.CSO"), 300)
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "sub", "NESTED.iso"), 400)

	repo := &fakeRepo{}
	sc := New(dir, repo)
	n, err := sc.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 3 {
		t.Errorf("scanned = %d, want 3 (iso, chd, cso; txt and nested dir skipped)", n)
	}
	for _, serial := range []string{"SLUS-20312", "SCES-50000", "GAME"} {
		g, ok := repo.stored[serial]
		if !ok {
			t.Errorf("missing stored game %s", serial)
			continue
		}
		if g.Title != serial {
			t.Errorf("%s Title = %q", serial, g.Title)
		}
		if g.FilePath == "" {
			t.Errorf("%s has empty FilePath", serial)
		}
	}
	if repo.stored["SLUS-20312"].SizeBytes != 100 {
		t.Errorf("size = %d, want 100", repo.stored["SLUS-20312"].SizeBytes)
	}
}

func TestRunCreatesMissingLibraryDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nope", "library")
	repo := &fakeRepo{}
	n, err := New(dir, repo).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 0 {
		t.Errorf("scanned = %d, want 0", n)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("library dir not created: %v", err)
	}
}

func TestRunEmptyDir(t *testing.T) {
	repo := &fakeRepo{}
	n, err := New(t.TempDir(), repo).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 0 {
		t.Errorf("scanned = %d, want 0", n)
	}
}

func TestQuickHash(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "e.iso")
	writeFile(t, empty, 0)
	h, err := quickHash(empty, 1<<20)
	if err != nil {
		t.Fatalf("quickHash: %v", err)
	}
	if h != "da39a3ee5e6b4b0d3255bfef95601890afd80709" {
		t.Errorf("empty hash = %q", h)
	}

	full := filepath.Join(dir, "f.iso")
	writeFile(t, full, 100)
	h2, err := quickHash(full, 1<<20)
	if err != nil {
		t.Fatalf("quickHash: %v", err)
	}
	if h2 == "" || h2 == h {
		t.Errorf("non-empty file must hash differently, got %q", h2)
	}

	if _, err := quickHash(filepath.Join(dir, "missing.iso"), 1<<20); err == nil {
		t.Error("want error for missing file")
	}
}
