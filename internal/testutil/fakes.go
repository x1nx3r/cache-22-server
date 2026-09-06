package testutil

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	authusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/auth"
	"github.com/x1nx3r/cache-22-server/internal/entity"
	"golang.org/x/crypto/bcrypt"
)

type FakeUsers struct {
	mu     sync.Mutex
	next   int64
	byName map[string]entity.User
	byID   map[int64]entity.User
}

func NewFakeUsers() *FakeUsers {
	return &FakeUsers{byName: map[string]entity.User{}, byID: map[int64]entity.User{}}
}

func (f *FakeUsers) Count(_ context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.byID), nil
}

func (f *FakeUsers) Create(_ context.Context, username, hash string, admin bool) (entity.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	u := entity.User{ID: f.next, Username: username, PasswordHash: hash, IsAdmin: admin}
	f.byName[username] = u
	f.byID[u.ID] = u
	return u, nil
}

func (f *FakeUsers) GetByUsername(_ context.Context, username string) (entity.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byName[username]
	if !ok {
		return entity.User{}, sql.ErrNoRows
	}
	return u, nil
}

func (f *FakeUsers) GetByID(_ context.Context, id int64) (entity.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[id]
	if !ok {
		return entity.User{}, sql.ErrNoRows
	}
	return u, nil
}

func (f *FakeUsers) List(_ context.Context) ([]entity.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []entity.User
	for _, u := range f.byID {
		out = append(out, u)
	}
	if out == nil {
		out = []entity.User{}
	}
	return out, nil
}

func (f *FakeUsers) Delete(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u := f.byID[id]
	delete(f.byID, id)
	delete(f.byName, u.Username)
	return nil
}

func (f *FakeUsers) SetPassword(_ context.Context, id int64, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u := f.byID[id]
	u.PasswordHash = hash
	f.byID[id] = u
	f.byName[u.Username] = u
	return nil
}

func (f *FakeUsers) SetAdmin(_ context.Context, id int64, admin bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u := f.byID[id]
	u.IsAdmin = admin
	f.byID[id] = u
	f.byName[u.Username] = u
	return nil
}

type FakeSessions struct {
	mu   sync.Mutex
	uids map[string]int64
}

func NewFakeSessions() *FakeSessions {
	return &FakeSessions{uids: map[string]int64{}}
}

func (f *FakeSessions) Create(_ context.Context, token string, userID int64, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uids[token] = userID
	return nil
}

func (f *FakeSessions) UserID(_ context.Context, token string) (int64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.uids[token]
	return id, ok, nil
}

func (f *FakeSessions) Revoke(_ context.Context, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.uids, token)
	return nil
}

func (f *FakeSessions) RevokeUser(_ context.Context, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for t, id := range f.uids {
		if id == userID {
			delete(f.uids, t)
		}
	}
	return nil
}

func HashForTest(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(h)
}

func NewAuthFresh(t *testing.T, setupToken string) *authusecase.Auth {
	t.Helper()
	return authusecase.New(NewFakeUsers(), NewFakeSessions(), setupToken)
}

func NewAuth(t *testing.T, setupToken string) (*authusecase.Auth, string, string) {
	t.Helper()
	users := NewFakeUsers()
	a := authusecase.New(users, NewFakeSessions(), setupToken)
	ctx := context.Background()
	if _, err := users.Create(ctx, "admin", HashForTest(t, "password1"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := users.Create(ctx, "player", HashForTest(t, "password1"), false); err != nil {
		t.Fatal(err)
	}
	adminTok, _, err := a.Login(ctx, "admin", "password1")
	if err != nil {
		t.Fatal(err)
	}
	playerTok, _, err := a.Login(ctx, "player", "password1")
	if err != nil {
		t.Fatal(err)
	}
	return a, adminTok, playerTok
}
