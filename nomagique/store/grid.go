package store

import (
	"context"
	"strings"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
GridServer is the virtual grid. Raw market data is written to it and the
metrics wired into it observe the fields the grid was told to deliver.

The grid holds no values of its own. A metric is the value its last operation
produced. Wiring another metric in is what makes the grid wider, which is why
nothing here enumerates them by name.

A metric that asked for a field the written data does not carry observes
nothing, so a metric is never handed a frame it cannot read and never has to
recognise one it should ignore.
*/
type GridServer struct {
	*runtime.System
	interests []string
	declared  string
	metrics   capnp.Float64List
	values    []float64
	present   []bool
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

	server.declare(interests)

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

	feeds, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[store.grid.Write] failed to read data argument",
			err,
		))
	}

	server.out = nil
	server.values = nil
	server.present = nil
	server.delivered = 0

	if !feeds.IsValid() {
		return nil
	}

	// Every feed that landed is resolved in turn, because a grid observes all
	// of them rather than whichever one happened to arrive last.
	for index := range feeds.Len() {
		payload, err := feeds.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.BadRequest,
				"[store.grid.Write] failed to read a written feed",
				err,
			))
		}

		if len(payload) == 0 {
			continue
		}

		if err := server.resolve(payload); err != nil {
			return err
		}

		if server.delivered > 0 {
			return nil
		}
	}

	return nil
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

	if err := server.deliver(results); err != nil {
		return err
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
declare records the fields this grid delivers. Declaring again replaces what it
delivers, so a grid's interests are whatever it was last told.

The same interests arrive on every observation, because they are configuration
rather than news. Only a change is worth saying out loud.
*/
func (server *GridServer) declare(interests string) {
	if interests == "" || interests == server.declared {
		return
	}

	fields := make([]string, 0, 4)

	for _, interest := range strings.Split(interests, ",") {
		interest = strings.TrimSpace(interest)

		if interest != "" {
			fields = append(fields, interest)
		}
	}

	if len(fields) == 0 {
		return
	}

	server.interests = fields
	server.declared = interests
	server.Info("registered %d field interests", len(fields))
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

	resolved := resolveInterests(document, server.interests)

	if len(resolved) == 0 {
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
	server.values = make([]float64, len(server.interests))
	server.present = make([]bool, len(server.interests))

	// Every declared interest keeps its own slot whether or not this record
	// carried it, so a metric reads the slot it asked for rather than
	// whichever field happened to land ahead of it.
	for index, interest := range server.interests {
		value, numeric := readInterest(resolved, interest)

		if !numeric {
			continue
		}

		server.values[index] = value
		server.present[index] = true
	}

	return nil
}

/*
deliver hands back one slot per declared interest, and says which of them this
record carried.
*/
func (server *GridServer) deliver(results Grid_done_Results) error {
	if len(server.values) == 0 {
		return nil
	}

	delivered, err := results.NewValues(int32(len(server.values)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[store.grid.deliver] failed to allocate delivered values",
			err,
		))
	}

	carried, err := results.NewPresent(int32(len(server.present)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[store.grid.deliver] failed to allocate delivered presence",
			err,
		))
	}

	for index, value := range server.values {
		delivered.Set(index, value)
		carried.Set(index, server.present[index])
	}

	server.values = nil
	server.present = nil
	return nil
}

/*
resolveInterests collects whichever of the declared fields this data carries.

One record comes from one feed and carries that feed's fields, so asking for
all of them at once and refusing anything less would mean nothing is ever
delivered. What the record does carry is a reading; what it does not is simply
not in it.
*/
func resolveInterests(document any, interests []string) map[string]any {
	resolved := make(map[string]any, len(interests))

	for _, interest := range interests {
		field, _, _ := strings.Cut(interest, "=")
		value, found := walkInterest(document, strings.Split(field, "."))

		if !found {
			continue
		}

		resolved[field] = value
	}

	return resolved
}

/*
readInterest reads one declared interest as a number.

A field carrying a number is that number. A field carrying anything else can
still be asked a question: an interest written field=literal reads as one when
the field holds that literal and zero when it holds something else, so which
of several kinds of record arrived is an observation like any other rather
than something a metric has to go parsing the document for.
*/
func readInterest(resolved map[string]any, interest string) (float64, bool) {
	field, literal, asked := strings.Cut(interest, "=")

	if !asked {
		value, numeric := resolved[interest].(float64)
		return value, numeric
	}

	held, found := resolved[field]

	if !found {
		return 0, false
	}

	text, textual := held.(string)

	if !textual {
		return 0, false
	}

	if text != literal {
		return 0, true
	}

	return 1, true
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

		// A channel-tagged record has the same declared path as its named
		// envelope: ticker.data.last addresses channel=ticker, data.last.
		if channel, tagged := object["channel"].(string); tagged && channel == segment {
			continue
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
