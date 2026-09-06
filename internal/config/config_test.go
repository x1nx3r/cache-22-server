package config

import (
	"testing"
)

func TestNewDefaults(t *testing.T) {
	t.Setenv("ENV", "")
	t.Setenv("SERVICE_NAME", "")
	t.Setenv("HTTP_PORT", "")
	t.Setenv("DB_URL", "")
	t.Setenv("LIBRARY_PATH", "")
	t.Setenv("ADMIN_TOKEN", "")

	cfg := New()

	if cfg.Env != "dev" {
		t.Errorf("Env = %q, want dev", cfg.Env)
	}
	if cfg.ServiceName != "cache-22-server" {
		t.Errorf("ServiceName = %q", cfg.ServiceName)
	}
	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %d, want 8080", cfg.HTTPPort)
	}
	if cfg.DBURL == "" {
		t.Error("DBURL must not be empty")
	}
	if cfg.LibraryPath != "./data/library" {
		t.Errorf("LibraryPath = %q", cfg.LibraryPath)
	}
	if cfg.AdminToken != "" {
		t.Errorf("AdminToken = %q, want empty", cfg.AdminToken)
	}
}

func TestNewOverrides(t *testing.T) {
	t.Setenv("ENV", "prod")
	t.Setenv("SERVICE_NAME", "s")
	t.Setenv("HTTP_PORT", "9090")
	t.Setenv("DB_URL", "postgres://u:p@h:5432/d?sslmode=disable")
	t.Setenv("LIBRARY_PATH", "/mnt/games")
	t.Setenv("ADMIN_TOKEN", "s3cret")

	cfg := New()

	if cfg.Env != "prod" || cfg.ServiceName != "s" || cfg.HTTPPort != 9090 {
		t.Errorf("overrides not applied: %+v", cfg)
	}
	if cfg.DBURL != "postgres://u:p@h:5432/d?sslmode=disable" {
		t.Errorf("DBURL = %q", cfg.DBURL)
	}
	if cfg.LibraryPath != "/mnt/games" {
		t.Errorf("LibraryPath = %q", cfg.LibraryPath)
	}
	if cfg.AdminToken != "s3cret" {
		t.Errorf("AdminToken = %q", cfg.AdminToken)
	}
}

func TestEnvIntOr(t *testing.T) {
	cases := []struct {
		name  string
		value string
		def   int
		want  int
	}{
		{"empty gives default", "", 8080, 8080},
		{"valid int", "9090", 8080, 9090},
		{"garbage gives default", "abc", 8080, 8080},
		{"float gives default", "80.5", 8080, 8080},
		{"negative passes through", "-1", 8080, -1},
		{"zero passes through", "0", 8080, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("TEST_INT_X", c.value)
			if got := envIntOr("TEST_INT_X", c.def); got != c.want {
				t.Errorf("envIntOr = %d, want %d", got, c.want)
			}
		})
	}
}

func TestEnvOr(t *testing.T) {
	t.Setenv("TEST_STR_X", "")
	if got := envOr("TEST_STR_X", "d"); got != "d" {
		t.Errorf("empty = %q, want d", got)
	}
	t.Setenv("TEST_STR_X", "v")
	if got := envOr("TEST_STR_X", "d"); got != "v" {
		t.Errorf("set = %q, want v", got)
	}
	if got := envOr("TEST_STR_UNSET_CACHE22", "d"); got != "d" {
		t.Errorf("unset = %q, want d", got)
	}
}
