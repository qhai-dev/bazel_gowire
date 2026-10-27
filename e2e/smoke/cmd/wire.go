//go:build wireinject
// +build wireinject

package main

import (
	"github.com/google/wire"

	"github.com/qhai-dev/bazel_gowire/e2e/framework"
	"github.com/qhai-dev/bazel_gowire/e2e/smoke/internal/adapter"
	"github.com/qhai-dev/bazel_gowire/e2e/smoke/internal/application"
	"github.com/qhai-dev/bazel_gowire/e2e/smoke/internal/infra"
)

func InitializeApp() (*framework.App, error) {
	panic(wire.Build(
		framework.New,
		infra.ProviderSet,
		application.ProviderSet,
		adapter.ProviderSet,
	))
}
