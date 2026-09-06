package httphandler

import (
	"testing"

	authusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/auth"
	"github.com/x1nx3r/cache-22-server/internal/testutil"
)

func newTestAuth(t *testing.T) (*authusecase.Auth, string, string) {
	t.Helper()
	return testutil.NewAuth(t, "setup-secret")
}
