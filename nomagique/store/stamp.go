package store

import (
	"bytes"
	"context"
	"math"
)

// StampServer owns per-run publication ordinals. Source identities in data are
// preserved verbatim; this ordinal identifies the downstream computation.
type StampServer struct {
	sequences map[string]int64
	run string
	sequence int64
	data []byte
}

func NewStamp() *StampServer {
	return &StampServer{sequences: make(map[string]int64)}
}

func (server *StampServer) Write(ctx context.Context, call Stamp_write) error {
	server.data = nil
	payload, err := call.Args().Data()

	if err != nil {
		return boundaryError("stamp: read observation", err)
	}

	if len(payload) == 0 {
		return nil
	}

	run, err := call.Args().Run()

	if err != nil || run == "" {
		return boundaryError("stamp: run identity is required", err)
	}

	if server.sequences[run] == math.MaxInt64 {
		return boundaryError("stamp: sequence representation exhausted", nil)
	}

	server.sequences[run]++
	server.run, server.sequence = run, server.sequences[run]
	server.data = bytes.Clone(payload)
	return nil
}

func (server *StampServer) Done(ctx context.Context, call Stamp_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return boundaryError("stamp: allocate result", err)
	}

	defer func() { server.data = nil }()

	if len(server.data) == 0 {
		results.SetIdle()
		return nil
	}

	results.SetItem()
	results.Item().SetSequence(server.sequence)

	if err := results.Item().SetRun(server.run); err != nil {
		return boundaryError("stamp: publish run", err)
	}

	if err := results.Item().SetData(server.data); err != nil {
		return boundaryError("stamp: publish observation", err)
	}

	return nil
}
