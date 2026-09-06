package main

import (
	"context"
	"log"

	"github.com/cache-22/cache-22-server/internal/config"
	"github.com/cache-22/cache-22-server/internal/xmodule"
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
	log.Printf("%s listening on :%d (library=%s db=%s)", cfg.ServiceName, cfg.HTTPPort, cfg.LibraryPath, cfg.DBURL)
	if err := xmodule.StartHTTPServer(cfg, deps); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
