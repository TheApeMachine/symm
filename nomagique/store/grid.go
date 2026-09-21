package store

import (
	"context"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
GridServer is the virtual grid. Raw market data is written to it and the
metrics wired into it observe the fields the grid was told to deliver.

The grid holds no values of its own. A metric is not a cell holding a number
but a capability the grid can call, so reading the grid is asking the metrics
wired into it for their current state. Wiring another metric in is what makes
the grid wider, which is why nothing here enumerates them by name.

A metric that asked for a field the written data does not carry observes
nothing, so a metric is never handed a frame it cannot read and never has to
recognise one it should ignore.
*/
type GridServer struct {
	*runtime.System
	interests []string
	metrics   data.MetricService_List
	out       []byte
	delivered int64
}

func NewGrid(ctx context.Context) *GridServer {
	server := &GridServer{
		System: runtime.NewSystem(ctx, "store.grid"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write resolves the written data against the fields this grid delivers.
*/
func (server *GridServer) Write(ctx context.Context, call Grid_write) error {
	interests, err := call.Args().Interests()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[store.grid.Write] failed to read interests argument",
			err,
		))
	}

	if interests != "" {
		server.declare(interests)
	}

	metrics, err := call.Args().Metrics()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[store.grid.Write] failed to read metrics argument",
			err,
		))
	}

	if metrics.IsValid() {
		server.metrics = metrics
	}

	payload, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[store.grid.Write] failed to read data argument",
			err,
		))
	}

	server.out = nil
	server.delivered = 0

	if len(payload) == 0 {
		return nil
	}

	return server.resolve(payload)
}

/*
Done reports what the grid resolved and how many metrics are wired into it.
*/
func (server *GridServer) Done(ctx context.Context, call Grid_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[store.grid.Done] failed to allocate results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))
	results.SetDelivered(server.delivered)
	results.SetMetrics(int64(server.metrics.Len()))

	if len(server.out) == 0 {
		return nil
	}

	if err := results.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[store.grid.Done] failed to set out",
			err,
		))
	}

	server.out = nil
	return nil
}

/*
declare records the fields this grid delivers. Declaring again replaces what
it delivers, so a grid's interests are whatever it was last told.
*/
func (server *GridServer) declare(interests string) {
	declared := make([]string, 0, 4)

	for _, interest := range strings.Split(interests, ",") {
		interest = strings.TrimSpace(interest)

		if interest != "" {
			declared = append(declared, interest)
		}
	}

	if len(declared) == 0 {
		return
	}

	server.interests = declared
	server.Info("delivering %d fields to %d metrics", len(declared), server.metrics.Len())
}

/*
resolve collects the declared fields out of the written data.
*/
func (server *GridServer) resolve(payload []byte) error {
	var document any

	if err := sonic.Unmarshal(payload, &document); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[store.grid.resolve] written data is not a structure",
			err,
		))
	}

	resolved, satisfied := resolveInterests(document, server.interests)

	if !satisfied {
		return nil
	}

	encoded, err := sonic.Marshal(resolved)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[store.grid.resolve] failed to encode delivery",
			err,
		))
	}

	server.out = encoded
	server.delivered = int64(len(resolved))

	return nil
}

/*
resolveInterests collects the fields the grid declared, reporting whether the
data carried all of them. Data missing a declared field is not delivered,
because a partial reading is not a reading.
*/
func resolveInterests(document any, interests []string) (map[string]any, bool) {
	resolved := make(map[string]any, len(interests))

	for _, interest := range interests {
		value, found := walkInterest(document, strings.Split(interest, "."))

		if !found {
			return nil, false
		}

		resolved[interest] = value
	}

	return resolved, len(resolved) > 0
}
func walkInterest(document any, segments []string) (any, bool) {
	current := document

	for _, segment := range segments {
		if elements, indexed := current.([]any); indexed {
			position, ok := interestIndex(segment)

			if !ok || position >= len(elements) {
				return nil, false
			}

			current = elements[position]
			continue
		}

		object, ok := current.(map[string]any)

		if !ok {
			return nil, false
		}

		current, ok = object[segment]

		if !ok {
			return nil, false
		}
	}

	return current, true
}
func interestIndex(segment string) (int, bool) {
	position := 0

	for _, character := range segment {
		if character < '0' || character > '9' {
			return 0, false
		}

		position = position*10 + int(character-'0')
	}

	return position, segment != ""
}
