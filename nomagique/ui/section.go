package ui

import (
	"context"

	"github.com/theapemachine/errnie"
)

type UISectionServer struct {
	top    int64
	right  int64
	bottom int64
	left   int64
}

func NewUISection() *UISectionServer {
	return &UISectionServer{}
}

func (server *UISectionServer) Write(ctx context.Context, call UISection_write) error {
	server.top = call.Args().Top()
	server.right = call.Args().Right()
	server.bottom = call.Args().Bottom()
	server.left = call.Args().Left()
	return nil
}

func (server *UISectionServer) Done(ctx context.Context, call UISection_done) error {
	return nil
}
