// Package app is the composition root. It defines the App struct that bundles
// the constructed dependencies and the Wire provider set used to build them.
// The actual wiring is generated into wire_gen.go by `wire` (see wire.go); this
// file only declares the pieces.
package app

import (
	"context"

	"github.com/dzovi/leave-management/internal/config"
	"github.com/dzovi/leave-management/internal/handlers"
	"github.com/dzovi/leave-management/internal/server"
	"github.com/dzovi/leave-management/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/google/wire"
)

// App holds the fully-wired application dependencies. The CLI commands read what
// they need off it (serve → Router + Config; anything DB-only → Store).
type App struct {
	Config config.Config
	Store  *store.Store
	Router *gin.Engine
}

// ProviderSet is the full dependency graph: config → store → handlers → router,
// assembled into an App. Wire resolves the order by matching types.
var ProviderSet = wire.NewSet(
	config.Load,
	provideStore,
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
func provideStore(ctx context.Context, cfg config.Config) (*store.Store, func(), error) {
	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	return st, func() { st.Close() }, nil
}
