package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/x1nx3r/cache-22-server/internal/config"
	"github.com/x1nx3r/cache-22-server/internal/xmodule"
)

func main() {
	cfg := config.New()
	deps, err := xmodule.Init(cfg)
	if err != nil {
		log.Fatalf("init: %v", err)
	}
	defer deps.DB.Close()

	if _, err := deps.Scanner.Run(context.Background()); err != nil {
		log.Printf("initial scan: %v", err)
	}

	srv := xmodule.NewHTTPServer(cfg, deps)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("%s listening on :%d (library=%s db=%s)", cfg.ServiceName, cfg.HTTPPort, cfg.LibraryPath, cfg.DBURL)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	case <-ctx.Done():
		log.Printf("shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}
}
