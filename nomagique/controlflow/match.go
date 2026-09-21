package controlflow

import (
	"bytes"
	"context"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
MatchServer matches streaming inbound data frames against a declared pattern.
When a match is found, it passes the data frame to out and emits matched=true.
*/
type MatchServer struct {
	*runtime.System
	pattern string
	data    []byte
	matched bool
}

func NewMatch(ctx context.Context) *MatchServer {
	server := &MatchServer{
		System: runtime.NewSystem(ctx, "controlflow.match"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write accepts an incoming data payload and/or pattern string to match against.
*/
func (server *MatchServer) Write(ctx context.Context, call Match_write) error {
	pattern, err := call.Args().Pattern()

	if err == nil && len(pattern) > 0 {
		server.pattern = pattern
	}

	data, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"controlflow.match: failed to read data arg",
			err,
		))
	}

	server.matched = false

	if len(data) == 0 {
		return nil
	}

	isMatch := len(server.pattern) == 0 || strings.Contains(string(data), server.pattern)

	if isMatch {
		server.data = bytes.Clone(data)
		server.matched = true
	}

	return nil
}

/*
Done emits the matched payload and matched boolean flag.
*/
func (server *MatchServer) Done(ctx context.Context, call Match_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"controlflow.match: failed to allocate done results",
			err,
		))
	}

	results.SetMatched(server.matched)

	if server.matched && len(server.data) > 0 {
		if err := results.SetOut(server.data); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"controlflow.match: failed to set out",
				err,
			))
		}
	}

	return nil
}
