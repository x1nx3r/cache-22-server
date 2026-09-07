package xmodule

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/x1nx3r/cache-22-server/internal/app/repository"
	authusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/auth"
	coverusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/cover"
	gameusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/game"
	saveusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/save"
	"github.com/x1nx3r/cache-22-server/internal/config"
	httphandler "github.com/x1nx3r/cache-22-server/internal/handler/http"
	webhandler "github.com/x1nx3r/cache-22-server/internal/handler/web"
	"github.com/x1nx3r/cache-22-server/internal/infra/db"
	"github.com/x1nx3r/cache-22-server/internal/infra/igdb"
	"github.com/x1nx3r/cache-22-server/internal/infra/scanner"
)

type Dependencies struct {
	Handler *httphandler.Handler
	Web     *webhandler.Handler
	Scanner *scanner.Scanner
	Games   repository.GameRepository
	DB      *sql.DB
}

func Init(cfg config.Config) (*Dependencies, error) {
	dbConn, err := db.Connect(cfg.DBURL)
	if err != nil {
		return nil, err
	}
	games := db.NewGameRepository(dbConn)
	health := db.NewHealthRepository(dbConn)
	users := db.NewUserRepository(dbConn)
	sessions := db.NewSessionRepository(dbConn)
	covers := db.NewCoverRepository(dbConn)
	saves := db.NewSaveRepository(dbConn)
	uc := gameusecase.New(games, health)
	authSvc := authusecase.New(users, sessions, cfg.AdminToken)
	coverSvc := coverusecase.New(games, covers, igdb.New(cfg.IGDBClient, cfg.IGDBSecret), cfg.CoverDir)
	saveSvc := saveusecase.New(saves, cfg.SavesDir)
	sc := scanner.New(cfg.LibraryPath, games)
	h := httphandler.New(uc, sc, authSvc, coverSvc, saveSvc)
	wh := webhandler.New(uc, sc, authSvc, cfg.Env == "prod")
	return &Dependencies{Handler: h, Web: wh, Scanner: sc, Games: games, DB: dbConn}, nil
}

// NewHTTPServer builds the API + web server. Deliberately no Read/Write
// timeouts: chunk uploads and range downloads stream for minutes. Only the
// headers are deadline-guarded, which is what Slowloris needs.
func NewHTTPServer(cfg config.Config, d *Dependencies) *http.Server {
	mux := http.NewServeMux()
	d.Handler.Routes(mux)
	d.Web.Routes(mux)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	return &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", cfg.HTTPPort),
		Handler:           withCORS(cfg, mux),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// withCORS adds CORS headers only for origins explicitly configured via
// CORS_ORIGINS. The default (unset) sends none: the Go client is not a
// browser and the web UI is same-origin.
func withCORS(cfg config.Config, next http.Handler) http.Handler {
	origins := map[string]bool{}
	for _, o := range cfg.CORSOrigins {
		origins[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origins[r.Header.Get("Origin")] {
			w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
