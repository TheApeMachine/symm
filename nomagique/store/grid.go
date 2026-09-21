package store

import (
	"context"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
GridServer is the virtual grid. Raw market data is written to it, and the
metrics registered with it receive only the fields they declared an interest
in.

The grid holds no values of its own: a metric is not a cell holding a number
but a registration saying which fields it needs, and the grid's work is
deciding what to deliver to whom. A metric whose interests the written data
does not carry observes nothing, so a metric is never handed a frame it
cannot read, and never has to recognise one it should ignore.

Registration and delivery are concurrent because the sources writing to a
grid read on their own goroutines, so the registry is guarded. The guard
covers the registry, never a metric's own work.
*/
type GridServer struct {
	*runtime.System
	mutex     sync.RWMutex
	metrics   map[string][]string
	order     []string
	out       []byte
	metric    string
	delivered int64
}

func NewGrid(ctx context.Context) *GridServer {
	server := &GridServer{
		System:  runtime.NewSystem(ctx, "store.grid"),
		metrics: make(map[string][]string),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write registers a metric's interests, distributes written data to the metrics
that declared an interest in it, or both.
*/
func (server *GridServer) Write(ctx context.Context, call Grid_write) error {
	metric, err := call.Args().Metric()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[store.grid.Write] failed to read metric argument",
			err,
		))
	}

	interests, err := call.Args().Interests()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[store.grid.Write] failed to read interests argument",
			err,
		))
	}

	if metric != "" && interests != "" {
		server.register(metric, interests)
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
	server.metric = ""
	server.delivered = 0

	if len(payload) == 0 {
		return nil
	}

	return server.distribute(payload)
}

/*
Done reports what the grid resolved for the metric it delivered to.
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

	server.mutex.RLock()
	registered := int64(len(server.metrics))
	server.mutex.RUnlock()

	results.SetStatus(runtime.Status(server.Status()))
	results.SetMetrics(registered)
	results.SetDelivered(server.delivered)

	if err := results.SetMetric(server.metric); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[store.grid.Done] failed to set metric",
			err,
		))
	}

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
register records the fields one metric needs. Registering again replaces what
that metric asked for, so a metric's interests are whatever it last declared.
*/
func (server *GridServer) register(metric, interests string) {
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

	server.mutex.Lock()
	defer server.mutex.Unlock()

	if _, known := server.metrics[metric]; !known {
		server.order = append(server.order, metric)
		server.Info("metric %s registered %d interests", metric, len(declared))
	}

	server.metrics[metric] = declared
}

/*
distribute resolves the written data against every registration and retains
what the first satisfied metric asked for.
*/
func (server *GridServer) distribute(payload []byte) error {
	var document any

	if err := sonic.Unmarshal(payload, &document); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[store.grid.distribute] written data is not a structure",
			err,
		))
	}

	server.mutex.RLock()
	defer server.mutex.RUnlock()

	for _, metric := range server.order {
		resolved, satisfied := resolveInterests(document, server.metrics[metric])

		if !satisfied {
			continue
		}

		encoded, err := sonic.Marshal(resolved)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[store.grid.distribute] failed to encode delivery",
				err,
			))
		}

		server.out = encoded
		server.metric = metric
		server.delivered++

		return nil
	}

	return nil
}

/*
resolveInterests collects the fields one metric declared, reporting whether
the data carried all of them. A metric that asked for a field the data does
not carry is not delivered to, because a partial reading is not a reading.
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

/*
walkInterest resolves one dotted interest key against the written data.
*/
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
