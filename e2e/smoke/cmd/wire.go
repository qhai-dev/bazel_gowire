//go:build wireinject
// +build wireinject

package main

import (
	"github.com/google/wire"

	"github.com/qhai-dev/e2e/library"
	"github.com/qhai-dev/e2e/smoke/internal/adapter"
	"github.com/qhai-dev/e2e/smoke/internal/application"
	"github.com/qhai-dev/e2e/smoke/internal/infra"
)

func InitializeApp() (*library.App, error) {
	panic(wire.Build(
		infra.ProviderSet,
		application.ProviderSet,
		adapter.ProviderSet,
		library.New,
	))
}
