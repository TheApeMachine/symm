package store

import (
	"bytes"
	"context"
	"encoding/json"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
QueueServer owns the values it has been offered and the order they wait in.
*/
type QueueServer struct {
	*runtime.System
	known   [][]byte
	seen    map[string]bool
	waiting [][]byte
	out     []byte
	ready   bool
}

func NewQueue(ctx context.Context) *QueueServer {
	return &QueueServer{
		System: runtime.NewSystem(ctx, "store.queue"),
		seen:   make(map[string]bool),
	}
}

func (server *QueueServer) Write(ctx context.Context, call Queue_write) error {
	server.out = nil
	args := call.Args()
	rewind, err := args.Rewind()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Validation, "queue: rewind", err,
		))
	}

	if arrivals(rewind) > 0 {
		server.waiting = append(server.waiting[:0], server.known...)
	}

	offer, err := args.Offer()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Validation, "queue: offer", err,
		))
	}

	if err := server.each(offer, server.join); err != nil {
		return server.Error(err)
	}

	retry, err := args.Retry()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Validation, "queue: retry", err,
		))
	}

	if err := server.each(retry, server.requeue); err != nil {
		return server.Error(err)
	}

	release, err := args.Release()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Validation, "queue: release", err,
		))
	}

	// A release says the consumer is ready for the next value. Readiness is a
	// state, not a count: it holds until something is handed out, however many
	// releases arrived meanwhile, so a release that finds the queue empty is
	// not lost and an idle stretch cannot bank a burst.
	if arrivals(release) > 0 {
		server.ready = true
	}

	// One value per evaluation keeps each hand-out its own observation.
	if server.ready && len(server.waiting) > 0 {
		server.out = server.waiting[0]
		server.waiting = server.waiting[1:]
		server.ready = false
	}

	return nil
}

func (server *QueueServer) join(value []byte) {
	if server.seen[string(value)] {
		return
	}

	server.seen[string(value)] = true
	server.known = append(server.known, value)
	server.waiting = append(server.waiting, value)
}

func (server *QueueServer) requeue(value []byte) {
	if !server.seen[string(value)] {
		server.seen[string(value)] = true
		server.known = append(server.known, value)
	}

	server.waiting = append(server.waiting, value)
}

func arrivals(list capnp.DataList) int {
	count := 0

	for index := range list.Len() {
		value, err := list.At(index)

		if err == nil && len(value) > 0 {
			count++
		}
	}

	return count
}

/*
each hands every element of every arrived JSON array to apply, as its canonical JSON.
*/
func (server *QueueServer) each(list capnp.DataList, apply func([]byte)) error {
	for index := range list.Len() {
		payload, err := list.At(index)

		if err != nil {
			return server.Error(errnie.Err(
				errnie.Validation, "queue: arrival", err,
			))
		}

		if len(payload) == 0 {
			continue
		}

		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoder.UseNumber()

		var values []json.RawMessage

		if err := decoder.Decode(&values); err != nil {
			return server.Error(errnie.Err(
				errnie.Validation, "queue: an arrival must be a JSON array", err,
			))
		}

		for _, value := range values {
			compact := new(bytes.Buffer)

			if err := json.Compact(compact, value); err != nil {
				return server.Error(errnie.Err(
					errnie.Validation, "queue: value", err,
				))
			}

			apply(compact.Bytes())
		}
	}

	return nil
}

func (server *QueueServer) Done(ctx context.Context, call Queue_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Internal, "queue: allocate result", err,
		))
	}

	results.SetWaiting(uint64(len(server.waiting)))
	results.SetKnown(uint64(len(server.known)))

	if server.out == nil {
		results.SetEmpty()
		return nil
	}

	if err := results.SetOut(server.out); err != nil {
		return server.Error(errnie.Err(
			errnie.Internal, "queue: emit", err,
		))
	}

	server.out = nil
	return nil
}
