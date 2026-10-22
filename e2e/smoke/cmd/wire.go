//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.
package main

import "github.com/google/wire"

func InitializeApp() {
	panic(wire.Build())
}