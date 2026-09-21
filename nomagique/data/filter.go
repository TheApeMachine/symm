package data

import (
	"context"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
FilterServer passes a structure downstream only while the value at a path
satisfies a comparison, and reports the verdict on passed. A structure that
does not satisfy it is withheld rather than replaced, so downstream nodes
observe nothing instead of observing a substitute.

A path the structure does not carry does not pass: an absent value cannot
satisfy a comparison, and treating it as one would invent evidence.
*/
type FilterServer struct {
	*runtime.System
	path      string
	operator  string
	threshold float64
	out       []byte
	passed    bool
}

func NewFilter(ctx context.Context) *FilterServer {
	server := &FilterServer{
		System:   runtime.NewSystem(ctx, "data.filter"),
		operator: "!=",
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write evaluates the comparison against the inbound structure.
*/
func (server *FilterServer) Write(ctx context.Context, call Filter_write) error {
	path, err := call.Args().Path()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.filter.Write] failed to read path argument",
			err,
		))
	}

	if len(path) > 0 {
		server.path = path
	}

	operator, err := call.Args().Operator()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.filter.Write] failed to read operator argument",
			err,
		))
	}

	if len(operator) > 0 {
		server.operator = operator
	}

	if threshold := call.Args().Threshold(); threshold != 0 {
		server.threshold = threshold
	}

	if server.path == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.filter.Write] path is not defined",
			nil,
		))
	}

	payload, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.filter.Write] failed to read data argument",
			err,
		))
	}

	server.passed = false
	server.out = nil

	if len(payload) == 0 {
		return nil
	}

	value, found, err := readPath(payload, server.path)

	if err != nil {
		return err
	}

	if !found {
		return nil
	}

	passed, err := compare(value, server.operator, server.threshold)

	if err != nil {
		return err
	}

	if !passed {
		return nil
	}

	server.passed = true
	server.out = payload

	return nil
}

/*
Done emits the structure when it satisfied the comparison.
*/
func (server *FilterServer) Done(ctx context.Context, call Filter_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.filter.Done] failed to allocate results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))
	results.SetPassed(server.passed)

	if !server.passed {
		return nil
	}

	if err := results.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.filter.Done] failed to set out",
			err,
		))
	}

	server.passed = false
	server.out = nil

	return nil
}

/*
compare evaluates one comparison, reporting an operator it does not implement
rather than silently admitting or rejecting the structure.
*/
func compare(value float64, operator string, threshold float64) (bool, error) {
	switch strings.TrimSpace(operator) {
	case "==":
		return value == threshold, nil
	case "!=":
		return value != threshold, nil
	case "<":
		return value < threshold, nil
	case "<=":
		return value <= threshold, nil
	case ">":
		return value > threshold, nil
	case ">=":
		return value >= threshold, nil
	}

	return false, errnie.Error(errnie.Err(
		errnie.Validation,
		"[data.filter] operator "+operator+" is not defined",
		nil,
	))
}
