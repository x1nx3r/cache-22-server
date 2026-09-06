package authusecase

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/x1nx3r/cache-22-server/internal/entity"
	"golang.org/x/crypto/bcrypt"
)

const (
	SessionTTL = 30 * 24 * time.Hour
	MinPassLen = 8
)

var (
	ErrSetupDone      = errors.New("setup already completed")
	ErrBadSetupToken  = errors.New("bad setup token")
	ErrBadCredentials = errors.New("bad username or password")
	ErrWeakPassword   = errors.New("password must be 8 or more characters")
	ErrBadUsername    = errors.New("username must be 1-64 characters")
)

type Users interface {
	Count(ctx context.Context) (int, error)
	Create(ctx context.Context, username, hash string, admin bool) (entity.User, error)
	GetByUsername(ctx context.Context, username string) (entity.User, error)
	GetByID(ctx context.Context, id int64) (entity.User, error)
	List(ctx context.Context) ([]entity.User, error)
	Delete(ctx context.Context, id int64) error
	SetPassword(ctx context.Context, id int64, hash string) error
	SetAdmin(ctx context.Context, id int64, admin bool) error
}

type Sessions interface {
	Create(ctx context.Context, token string, userID int64, ttl time.Duration) error
	UserID(ctx context.Context, token string) (int64, bool, error)
	Revoke(ctx context.Context, token string) error
	RevokeUser(ctx context.Context, userID int64) error
}

type Auth struct {
	users      Users
	sessions   Sessions
	setupToken string
}

func New(users Users, sessions Sessions, setupToken string) *Auth {
	return &Auth{users: users, sessions: sessions, setupToken: setupToken}
}

func validUsername(name string) bool {
	name = strings.TrimSpace(name)
	return len(name) >= 1 && len(name) <= 64
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func newToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (a *Auth) NeedsSetup(ctx context.Context) (bool, error) {
	n, err := a.users.Count(ctx)
	return n == 0, err
}

func (a *Auth) SetupFirstAdmin(ctx context.Context, setupToken, username, password string) (entity.User, error) {
	needs, err := a.NeedsSetup(ctx)
	if err != nil {
		return entity.User{}, err
	}
	if !needs {
		return entity.User{}, ErrSetupDone
	}
	if a.setupToken == "" || subtle.ConstantTimeCompare([]byte(setupToken), []byte(a.setupToken)) != 1 {
		return entity.User{}, ErrBadSetupToken
	}
	return a.createUser(ctx, username, password, true)
}

func (a *Auth) createUser(ctx context.Context, username, password string, admin bool) (entity.User, error) {
	if !validUsername(username) {
		return entity.User{}, ErrBadUsername
	}
	if len(password) < MinPassLen {
		return entity.User{}, ErrWeakPassword
	}
	hash, err := hashPassword(password)
	if err != nil {
		return entity.User{}, err
	}
	return a.users.Create(ctx, username, hash, admin)
}

func (a *Auth) CreateUser(ctx context.Context, username, password string, admin bool) (entity.User, error) {
	return a.createUser(ctx, username, password, admin)
}

func (a *Auth) Login(ctx context.Context, username, password string) (string, entity.User, error) {
	u, err := a.users.GetByUsername(ctx, username)
	if err != nil {
		return "", entity.User{}, ErrBadCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return "", entity.User{}, ErrBadCredentials
	}
	token, err := newToken()
	if err != nil {
		return "", entity.User{}, err
	}
	if err := a.sessions.Create(ctx, token, u.ID, SessionTTL); err != nil {
		return "", entity.User{}, err
	}
	u.PasswordHash = ""
	return token, u, nil
}

func (a *Auth) Logout(ctx context.Context, token string) error {
	return a.sessions.Revoke(ctx, token)
}

func (a *Auth) Authenticate(ctx context.Context, token string) (entity.User, bool, error) {
	if token == "" {
		return entity.User{}, false, nil
	}
	id, ok, err := a.sessions.UserID(ctx, token)
	if err != nil || !ok {
		return entity.User{}, false, err
	}
	u, err := a.users.GetByID(ctx, id)
	if err != nil {
		return entity.User{}, false, err
	}
	u.PasswordHash = ""
	return u, true, nil
}

func (a *Auth) ListUsers(ctx context.Context) ([]entity.User, error) {
	users, err := a.users.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range users {
		users[i].PasswordHash = ""
	}
	return users, nil
}

func (a *Auth) DeleteUser(ctx context.Context, id int64) error {
	if err := a.sessions.RevokeUser(ctx, id); err != nil {
		return err
	}
	return a.users.Delete(ctx, id)
}

func (a *Auth) ResetPassword(ctx context.Context, id int64, password string) error {
	if len(password) < MinPassLen {
		return ErrWeakPassword
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	if err := a.sessions.RevokeUser(ctx, id); err != nil {
		return err
	}
	return a.users.SetPassword(ctx, id, hash)
}

func (a *Auth) SetAdmin(ctx context.Context, id int64, admin bool) error {
	return a.users.SetAdmin(ctx, id, admin)
}
