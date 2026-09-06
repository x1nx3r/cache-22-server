package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/cache-22/cache-22-server/internal/entity"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	base := os.Getenv("TEST_DB_URL")
	if base == "" {
		base = "postgres://cache22:cache22@localhost:5432/cache22?sslmode=disable"
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("bad test db url: %v", err)
	}
	adminURL := *u
	adminURL.Path = "/postgres"
	admin, err := sql.Open("pgx", adminURL.String())
	if err != nil {
		t.Skipf("postgres unreachable (%v), skipping pg test", err)
	}
	if err := admin.Ping(); err != nil {
		admin.Close()
		t.Skipf("postgres unreachable (%v), skipping pg test", err)
	}

	name := fmt.Sprintf("cache22_test_%d_%d", os.Getpid(), time.Now().UnixNano()%1000000)
	if _, err := admin.Exec(`CREATE DATABASE "` + name + `"`); err != nil {
		t.Fatalf("create scratch db: %v", err)
	}

	testURL := *u
	testURL.Path = "/" + name
	db, err := Connect(testURL.String())
	if err != nil {
		admin.Exec(`DROP DATABASE "` + name + `"`)
		t.Fatalf("connect scratch db: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		admin.Exec(`DROP DATABASE "` + name + `" WITH (FORCE)`)
		admin.Close()
	})
	return db
}

func TestUpsertAndGet(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	r := NewGameRepository(db)

	g := entity.Game{Serial: "T-1", RedumpHash: "h", Title: "T", SizeBytes: 42, FilePath: "/x/t.iso"}
	if err := r.Upsert(ctx, g); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := r.GetBySerial(ctx, "T-1")
	if err != nil {
		t.Fatalf("GetBySerial: %v", err)
	}
	if got.SizeBytes != 42 || got.FilePath != "/x/t.iso" || got.Title != "T" {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("timestamps must be set by the database")
	}

	g.SizeBytes = 43
	g.Title = "T2"
	time.Sleep(10 * time.Millisecond)
	if err := r.Upsert(ctx, g); err != nil {
		t.Fatalf("re-Upsert: %v", err)
	}
	got2, err := r.GetBySerial(ctx, "T-1")
	if err != nil {
		t.Fatalf("GetBySerial after update: %v", err)
	}
	if got2.SizeBytes != 43 || got2.Title != "T2" {
		t.Errorf("update not applied: %+v", got2)
	}

	if _, err := r.GetBySerial(ctx, "MISSING"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("missing err = %v, want ErrNoRows", err)
	}
}

func TestList(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	r := NewGameRepository(db)

	empty, err := r.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("want empty list, got %d", len(empty))
	}

	for _, g := range []entity.Game{
		{Serial: "B", Title: "Bravo"},
		{Serial: "A", Title: "Alpha"},
	} {
		if err := r.Upsert(ctx, g); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}
	got, err := r.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d games, want 2", len(got))
	}
	if got[0].Title != "Alpha" || got[1].Title != "Bravo" {
		t.Errorf("list not ordered by title: %+v", got)
	}
}
