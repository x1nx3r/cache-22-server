package authusecase

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/x1nx3r/cache-22-server/internal/entity"
	"golang.org/x/crypto/bcrypt"
)

type fakeUsers struct {
	mu     sync.Mutex
	next   int64
	byName map[string]entity.User
	byID   map[int64]entity.User
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byName: map[string]entity.User{}, byID: map[int64]entity.User{}}
}

func (f *fakeUsers) Count(_ context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.byID), nil
}

func (f *fakeUsers) Create(_ context.Context, username, hash string, admin bool) (entity.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	username = strings.ToLower(strings.TrimSpace(username))
	if _, ok := f.byName[username]; ok {
		return entity.User{}, errors.New("duplicate")
	}
	f.next++
	u := entity.User{ID: f.next, Username: username, PasswordHash: hash, IsAdmin: admin}
	f.byName[username] = u
	f.byID[u.ID] = u
	return u, nil
}

func (f *fakeUsers) GetByUsername(_ context.Context, username string) (entity.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byName[strings.ToLower(strings.TrimSpace(username))]
	if !ok {
		return entity.User{}, sql.ErrNoRows
	}
	return u, nil
}

func (f *fakeUsers) GetByID(_ context.Context, id int64) (entity.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[id]
	if !ok {
		return entity.User{}, sql.ErrNoRows
	}
	return u, nil
}

func (f *fakeUsers) List(_ context.Context) ([]entity.User, error) {
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

func (f *fakeUsers) Delete(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u := f.byID[id]
	delete(f.byID, id)
	delete(f.byName, u.Username)
	return nil
}

func (f *fakeUsers) SetPassword(_ context.Context, id int64, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u := f.byID[id]
	u.PasswordHash = hash
	f.byID[id] = u
	f.byName[u.Username] = u
	return nil
}

func (f *fakeUsers) SetAdmin(_ context.Context, id int64, admin bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u := f.byID[id]
	u.IsAdmin = admin
	f.byID[id] = u
	f.byName[u.Username] = u
	return nil
}

type fakeSessions struct {
	mu   sync.Mutex
	uids map[string]int64
}

func newFakeSessions() *fakeSessions {
	return &fakeSessions{uids: map[string]int64{}}
}

func (f *fakeSessions) Create(_ context.Context, token string, userID int64, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uids[token] = userID
	return nil
}

func (f *fakeSessions) UserID(_ context.Context, token string) (int64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.uids[token]
	return id, ok, nil
}

func (f *fakeSessions) Revoke(_ context.Context, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.uids, token)
	return nil
}

func (f *fakeSessions) RevokeUser(_ context.Context, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for t, id := range f.uids {
		if id == userID {
			delete(f.uids, t)
		}
	}
	return nil
}

func newAuth() *Auth {
	return New(newFakeUsers(), newFakeSessions(), "setup-secret")
}

func TestSetupFirstAdmin(t *testing.T) {
	a := newAuth()
	ctx := context.Background()

	if needs, _ := a.NeedsSetup(ctx); !needs {
		t.Fatal("fresh db needs setup")
	}
	if _, err := a.SetupFirstAdmin(ctx, "wrong", "admin", "password1"); !errors.Is(err, ErrBadSetupToken) {
		t.Errorf("bad setup token err = %v", err)
	}
	if _, err := a.SetupFirstAdmin(ctx, "setup-secret", "admin", "short"); !errors.Is(err, ErrWeakPassword) {
		t.Errorf("weak password err = %v", err)
	}
	u, err := a.SetupFirstAdmin(ctx, "setup-secret", "Admin", "password1")
	if err != nil {
		t.Fatal(err)
	}
	if !u.IsAdmin || u.Username != "admin" {
		t.Errorf("admin = %+v (want lowercase admin)", u)
	}
	if _, err := a.SetupFirstAdmin(ctx, "setup-secret", "x", "password1"); !errors.Is(err, ErrSetupDone) {
		t.Errorf("second setup err = %v", err)
	}
}

func TestLoginLogout(t *testing.T) {
	a := newAuth()
	ctx := context.Background()
	if _, err := a.SetupFirstAdmin(ctx, "setup-secret", "admin", "password1"); err != nil {
		t.Fatal(err)
	}

	if _, _, err := a.Login(ctx, "admin", "wrong"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("bad password err = %v", err)
	}
	if _, _, err := a.Login(ctx, "nobody", "password1"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("bad user err = %v", err)
	}
	token, u, err := a.Login(ctx, "ADMIN", "password1")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || u.PasswordHash != "" {
		t.Errorf("login = %q %+v", token, u)
	}
	got, ok, err := a.Authenticate(ctx, token)
	if err != nil || !ok || got.Username != "admin" {
		t.Errorf("auth = %+v %v %v", got, ok, err)
	}
	if _, ok, _ := a.Authenticate(ctx, "bogus"); ok {
		t.Error("bogus token must not authenticate")
	}
	if _, ok, _ := a.Authenticate(ctx, ""); ok {
		t.Error("empty token must not authenticate")
	}
	if err := a.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := a.Authenticate(ctx, token); ok {
		t.Error("logged-out token must not authenticate")
	}
}

func TestUserAdmin(t *testing.T) {
	a := newAuth()
	ctx := context.Background()
	if _, err := a.SetupFirstAdmin(ctx, "setup-secret", "admin", "password1"); err != nil {
		t.Fatal(err)
	}
	u, err := a.CreateUser(ctx, "player", "password1", false)
	if err != nil {
		t.Fatal(err)
	}
	if u.IsAdmin {
		t.Error("player must not be admin")
	}
	users, err := a.ListUsers(ctx)
	if err != nil || len(users) != 2 {
		t.Fatalf("list = %v %v", users, err)
	}
	for _, x := range users {
		if x.PasswordHash != "" {
			t.Error("hashes must never leave the usecase")
		}
	}
	if err := a.SetAdmin(ctx, u.ID, true); err != nil {
		t.Fatal(err)
	}
	token, _, err := a.Login(ctx, "player", "password1")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ResetPassword(ctx, u.ID, "password22"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Login(ctx, "player", "password1"); !errors.Is(err, ErrBadCredentials) {
		t.Error("old password must stop working")
	}
	if _, ok, _ := a.Authenticate(ctx, token); ok {
		t.Error("reset must revoke sessions")
	}
	if _, _, err := a.Login(ctx, "player", "password22"); err != nil {
		t.Errorf("new password: %v", err)
	}
	if err := a.ResetPassword(ctx, u.ID, "short"); !errors.Is(err, ErrWeakPassword) {
		t.Errorf("weak reset err = %v", err)
	}
	if err := a.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Login(ctx, "player", "password22"); !errors.Is(err, ErrBadCredentials) {
		t.Error("deleted user must not log in")
	}
}

func TestBcryptCost(t *testing.T) {
	h, err := hashPassword("password1")
	if err != nil {
		t.Fatal(err)
	}
	cost, err := bcrypt.Cost([]byte(h))
	if err != nil || cost < bcrypt.MinCost {
		t.Errorf("cost = %d, %v", cost, err)
	}
}
