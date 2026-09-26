package store

import (
	"context"

	"github.com/theapemachine/errnie"
)

/* LatestServer owns the current causal cross section of named observations. */
type LatestServer struct {
	positions map[string]int
	keys      []string
	values    []float64
	sequences []int64
	epoch     int64
	sequence  int64
}

/* NewLatest constructs an empty cohort without invented members or readings. */
func NewLatest() *LatestServer {
	return &LatestServer{positions: make(map[string]int)}
}

/* Write replaces exactly one member, rejecting observations from the past. */
func (server *LatestServer) Write(ctx context.Context, call Latest_write) error {
	args := call.Args()
	key, err := args.Key()

	if err != nil {
		return errnie.Error(err)
	}

	if key == "" || args.Epoch() < 0 || args.Sequence() < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "latest: a member and nonnegative causal stamps are required", nil))
	}

	if args.Epoch() < server.epoch || args.Epoch() == server.epoch && args.Sequence() < server.sequence {
		return errnie.Error(errnie.Err(errnie.Validation, "latest: observation regressed", nil))
	}

	if args.Epoch() != server.epoch {
		clear(server.positions)
		server.keys, server.values, server.sequences = nil, nil, nil
	}
	server.epoch, server.sequence = args.Epoch(), args.Sequence()
	position, found := server.positions[key]

	if !found {
		position = len(server.keys)
		server.positions[key] = position
		server.keys = append(server.keys, key)
		server.values = append(server.values, 0)
		server.sequences = append(server.sequences, 0)
	}
	server.values[position], server.sequences[position] = args.Value(), args.Sequence()
	return nil
}

/* Done emits current members together with each member's own last update. */
func (server *LatestServer) Done(ctx context.Context, call Latest_done) error {
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	keys, err := result.NewKeys(int32(len(server.keys)))

	if err != nil {
		return errnie.Error(err)
	}
	values, err := result.NewValues(int32(len(server.values)))

	if err != nil {
		return errnie.Error(err)
	}
	sequences, err := result.NewSequences(int32(len(server.sequences)))

	if err != nil {
		return errnie.Error(err)
	}
	result.SetEpoch(server.epoch)
	for index, key := range server.keys {
		if err := keys.Set(index, key); err != nil {
			return errnie.Error(err)
		}
		values.Set(index, server.values[index])
		sequences.Set(index, server.sequences[index])
	}
	return nil
}
