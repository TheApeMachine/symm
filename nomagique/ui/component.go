package ui

import (
	"context"

	"github.com/theapemachine/errnie"
)

type UIComponentServer struct {
	componentType ComponentType
	variant       Variant
	hasValue      bool
}

func NewUIComponent() *UIComponentServer {
	return &UIComponentServer{}
}

func (server *UIComponentServer) Write(ctx context.Context, call UIComponent_write) error {
	server.componentType = call.Args().Type()
	server.variant = call.Args().Variant()
	server.hasValue = true
	return nil
}

func (server *UIComponentServer) Done(ctx context.Context, call UIComponent_done) error {
	server.hasValue = false
	return nil
}
