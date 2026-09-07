package config

import (
	"os"
	"strings"
)

type Config struct {
	Env         string
	ServiceName string
	HTTPPort    int
	DBURL       string
	LibraryPath string
	AdminToken  string
	CoverDir    string
	SavesDir    string
	IGDBClient  string
	IGDBSecret  string
	CORSOrigins []string
}

func New() Config {
	return Config{
		Env:         envOr("ENV", "dev"),
		ServiceName: envOr("SERVICE_NAME", "cache-22-server"),
		HTTPPort:    envIntOr("HTTP_PORT", 8080),
		DBURL:       envOr("DB_URL", "postgres://cache22:cache22@localhost:5432/cache22?sslmode=disable"),
		LibraryPath: envOr("LIBRARY_PATH", "./data/library"),
		CoverDir:    envOr("COVER_DIR", "./data/covers"),
		SavesDir:    envOr("SAVES_DIR", "./data/saves"),
		IGDBClient:  envOr("IGDB_CLIENT_ID", ""),
		IGDBSecret:  envOr("IGDB_CLIENT_SECRET", ""),
		AdminToken:  envOr("ADMIN_TOKEN", ""),
		CORSOrigins: csvOr("CORS_ORIGINS"),
	}
}

func csvOr(key string) []string {
	raw := os.Getenv(key)
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOr(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var n int
	_, err := sscanfInt(v, &n)
	if err != nil {
		return def
	}
	return n
}
