//go:build wireinject
// +build wireinject

// This file is the Wire injector template — it is NOT compiled into the app
// (build tag `wireinject`). Running `wire ./internal/app` reads the wire.Build
// calls here and generates wire_gen.go with the real wiring.
package app

import (
	"context"

	"github.com/meddhiazoghlami/leave-management/internal/store"

	"github.com/google/wire"
)

// InitializeApp builds the full application (config → store → handlers → router)
// and returns it plus a cleanup func (closes the DB pool).
func InitializeApp(ctx context.Context) (*App, func(), error) {
	wire.Build(ProviderSet)
	return nil, nil, nil
}

// InitializeStore builds just the data layer, for DB-only commands like seed.
func InitializeStore(ctx context.Context) (*store.Store, func(), error) {
	wire.Build(StoreSet)
	return nil, nil, nil
}
