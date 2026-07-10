// Command leave-management is the entrypoint. Phase 9 shrinks main.go to a thin
// bootstrap: load config, open the store, initialise the Vite asset layer, wire
// the router, and serve. All real logic lives under internal/.
package main

import (
	"context"
	"log"
	"os"

	"github.com/dzovi/leave-management/assets"
	"github.com/dzovi/leave-management/internal/config"
	"github.com/dzovi/leave-management/internal/handlers"
	"github.com/dzovi/leave-management/internal/server"
	"github.com/dzovi/leave-management/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()
	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect to postgres: %v", err)
	}
	defer st.Close()

	// Best-effort cleanup of expired sessions on boot.
	_ = st.DeleteExpiredSessions(ctx)

	// Phase 8: dev mode (VITE_DEV=true) points the layout at the Vite dev server
	// for HMR; otherwise we read the built manifest to emit hashed asset tags.
	if err := assets.Init(cfg.ViteDev); err != nil {
		log.Fatalf("load asset manifest (did you run `npm run build` in web/?): %v", err)
	}

	h := handlers.New(st, cfg)
	r := server.New(h, st)

	log.Printf("listening on %s", cfg.Addr)
	if err := r.Run(cfg.Addr); err != nil {
		log.Fatalf("server: %v", err)
		os.Exit(1)
	}
}
