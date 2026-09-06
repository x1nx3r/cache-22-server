package xmodule

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/cache-22/cache-22-server/internal/app/repository"
	authusecase "github.com/cache-22/cache-22-server/internal/app/usecase/auth"
	coverusecase "github.com/cache-22/cache-22-server/internal/app/usecase/cover"
	gameusecase "github.com/cache-22/cache-22-server/internal/app/usecase/game"
	"github.com/cache-22/cache-22-server/internal/config"
	httphandler "github.com/cache-22/cache-22-server/internal/handler/http"
	webhandler "github.com/cache-22/cache-22-server/internal/handler/web"
	"github.com/cache-22/cache-22-server/internal/infra/db"
	"github.com/cache-22/cache-22-server/internal/infra/igdb"
	"github.com/cache-22/cache-22-server/internal/infra/scanner"
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
	health := db.NewHealthRepository()
	users := db.NewUserRepository(dbConn)
	sessions := db.NewSessionRepository(dbConn)
	covers := db.NewCoverRepository(dbConn)
	uc := gameusecase.New(games, health)
	authSvc := authusecase.New(users, sessions, cfg.AdminToken)
	coverSvc := coverusecase.New(games, covers, igdb.New(cfg.IGDBClient, cfg.IGDBSecret), cfg.CoverDir)
	sc := scanner.New(cfg.LibraryPath, games)
	h := httphandler.New(uc, sc, authSvc, coverSvc)
	wh := webhandler.New(uc, sc, authSvc)
	return &Dependencies{Handler: h, Web: wh, Scanner: sc, Games: games, DB: dbConn}, nil
}

func StartHTTPServer(cfg config.Config, d *Dependencies) error {
	mux := http.NewServeMux()
	d.Handler.Routes(mux)
	d.Web.Routes(mux)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	addr := fmt.Sprintf("0.0.0.0:%d", cfg.HTTPPort)
	return http.ListenAndServe(addr, withCORS(mux))
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
