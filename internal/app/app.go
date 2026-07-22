// Package app is the composition root. It defines the App struct that bundles
// the constructed dependencies and the Wire provider set used to build them.
// The actual wiring is generated into wire_gen.go by `wire` (see wire.go); this
// file only declares the pieces.
package app

import (
	"context"
	"fmt"
	"log"
	"log/slog"

	"github.com/meddhiazoghlami/leave-management/internal/auth"
	"github.com/meddhiazoghlami/leave-management/internal/config"
	"github.com/meddhiazoghlami/leave-management/internal/handlers"
	"github.com/meddhiazoghlami/leave-management/internal/migrate"
	"github.com/meddhiazoghlami/leave-management/internal/obs"
	"github.com/meddhiazoghlami/leave-management/internal/server"
	"github.com/meddhiazoghlami/leave-management/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/google/wire"
)

// App holds the fully-wired application dependencies. The CLI commands read what
// they need off it (serve → Router + Config + Logger; anything DB-only → Store).
type App struct {
	Config config.Config
	Store  *store.Store
	Router *gin.Engine
	Logger *slog.Logger
}

// ProviderSet is the full dependency graph: config → store + observability →
// handlers → router, assembled into an App. Wire resolves the order by matching
// types. The two wire.Bind lines teach Wire that the concrete *store.Store
// satisfies the consumer-side interfaces handlers.New and server.New ask for.
//
// obs.NewLogger and obs.InitTracing each return a cleanup, which Wire folds into
// the injector's returned func alongside the store's — so `defer cleanup()`
// closes the pool, flushes Loki, and shuts the tracer down in one shot.
var ProviderSet = wire.NewSet(
	config.Load,
	provideStore,
	obs.NewLogger,
	obs.InitTracing,
	wire.Bind(new(handlers.Store), new(*store.Store)),
	wire.Bind(new(auth.SessionStore), new(*store.Store)),
	handlers.New,
	server.New,
	wire.Struct(new(App), "*"),
)

// StoreSet is the DB-only subset (config → store), for commands that don't need
// the HTTP router (e.g. seed).
var StoreSet = wire.NewSet(
	config.Load,
	provideStore,
)

// provideStore opens the connection pool from the loaded config and returns a
// cleanup that closes it. Wire aggregates cleanups into the injector's returned
// func, so callers just `defer cleanup()`.
//
// When cfg.AutoMigrate is set it first brings the schema up to date, so both
// `serve` and `seed` "just work" against a fresh or out-of-date dev database.
// This runs before store.New so no query ever hits a missing table.
func provideStore(ctx context.Context, cfg config.Config) (*store.Store, func(), error) {
	if cfg.AutoMigrate {
		log.Print("AUTO_MIGRATE=true — applying any pending migrations")
		if err := migrate.Up(cfg.DatabaseURL); err != nil {
			return nil, nil, fmt.Errorf("auto-migrate: %w", err)
		}
	}
	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	return st, func() { st.Close() }, nil
}
