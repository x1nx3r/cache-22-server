package scanner

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/x1nx3r/cache-22-server/internal/app/repository"
	"github.com/x1nx3r/cache-22-server/internal/entity"
)

var imageExts = map[string]bool{
	".iso": true, ".chd": true, ".cso": true, ".zso": true,
}

type Scanner struct {
	libraryPath string
	games       repository.GameRepository
	hashBytes   int64
}

func New(libraryPath string, games repository.GameRepository) *Scanner {
	return &Scanner{libraryPath: libraryPath, games: games, hashBytes: 4 << 20}
}

func (s *Scanner) LibraryDir() string {
	return s.libraryPath
}

func (s *Scanner) Run(ctx context.Context) (int, error) {
	if err := os.MkdirAll(s.libraryPath, 0o755); err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(s.libraryPath)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, e := range entries {
		if ctx.Err() != nil {
			return count, ctx.Err()
		}
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !imageExts[ext] {
			continue
		}
		full := filepath.Join(s.libraryPath, e.Name())
		g, err := s.inspect(full)
		if err != nil {
			continue
		}
		if err := s.games.Upsert(ctx, g); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *Scanner) Inspect(path string) (entity.Game, error) {
	return s.inspect(path)
}

func (s *Scanner) inspect(full string) (entity.Game, error) {
	st, err := os.Stat(full)
	if err != nil {
		return entity.Game{}, err
	}
	base := strings.TrimSuffix(filepath.Base(full), filepath.Ext(full))
	hash, err := quickHash(full, s.hashBytes)
	if err != nil {
		hash = ""
	}
	return entity.Game{
		Serial:     base,
		RedumpHash: hash,
		Title:      base,
		SizeBytes:  st.Size(),
		FilePath:   full,
	}, nil
}

func quickHash(path string, n int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.CopyN(h, f, n); err != nil && err != io.EOF {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
