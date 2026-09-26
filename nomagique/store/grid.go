package store

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
GridServer is the virtual grid. Raw market data is written to it and the
metrics wired into it observe the fields the grid was told to deliver.

The grid retains explicitly held input fields and reported metric observations
for each named series. Metric calculations remain owned by the wired nodes;
this projector does not enumerate or calculate them.

A metric that asked for a field the written data does not carry observes
nothing, so a metric is never handed a frame it cannot read and never has to
recognise one it should ignore.
*/
type GridServer struct {
	*runtime.System
	*gridSeries
	series    map[string]*gridSeries
	scope     string
	scopePath string
	interests []string
	declared  string
	values    []float64
	present   []bool
	out       []byte
	raw       []byte
	delivered int64
}

/* gridSeries owns only the retained readings for one named market. */
type gridSeries struct {
	metrics  []float64
	observed []bool
	held     map[string]float64
}

func NewGrid(ctx context.Context) *GridServer {
	server := &GridServer{
		System: runtime.NewSystem(ctx, "store.grid"),
		series: make(map[string]*gridSeries),
	}

	server.selectScope("")
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
	scopePath, err := call.Args().ScopePath()

	if err != nil {
		return errnie.Error(err)
	}
	server.scopePath = scopePath

	if err := server.enter(call.Args()); err != nil {
		return err
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
	server.raw = nil
	server.values = nil
	server.present = nil
	server.delivered = 0

	if !feeds.IsValid() {
		return server.observe(call.Args())
	}

	// Each Workspace observation carries one record. The native sequencer
	// owns concurrent feed admission, so a Grid never hides a second queue.
	arrivals := 0
	for index := range feeds.Len() {
		payload, err := feeds.At(index)
		if err != nil {
			return errnie.Error(err)
		}
		if len(payload) > 0 {
			arrivals++
		}
	}
	if arrivals > 1 {
		return errnie.Error(errnie.Err(errnie.Validation, "grid: multiple records need separate Workspace observations", nil))
	}
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
			return server.observe(call.Args())
		}
	}

	return server.observe(call.Args())
}

/* observe applies reported metrics after selecting their market from the record. */
func (server *GridServer) observe(args Grid_write_Params) error {
	metrics, err := args.Metrics()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[store.grid.Write] failed to read metrics argument",
			err,
		))
	}

	present, err := args.Present()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "grid: metric presence", err))
	}

	if metrics.IsValid() {
		if len(server.metrics) != metrics.Len() {
			server.metrics = make([]float64, metrics.Len())
			server.observed = make([]bool, metrics.Len())
		}

		// Observed marks what reported since the grid last handed its
		// readings out; done clears it. A metric that did not report is
		// unknown, however recently it last reported: its last value stays in
		// metrics but is never handed out as a fresh reading.
		for index := range metrics.Len() {
			if present.IsValid() && index < present.Len() && !present.At(index) {
				continue
			}

			server.metrics[index] = metrics.At(index)
			server.observed[index] = true
		}
	}

	return nil
}

/*
enter moves the grid to the series its written data belongs to. A new series
hands out nothing observed under the previous one.
*/
func (server *GridServer) enter(args Grid_write_Params) error {
	scopes, err := args.Scope()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[store.grid.enter] failed to read scope argument",
			err,
		))
	}

	if scopes.Len() == 0 {
		return nil
	}

	scope, err := scopes.At(0)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.BadRequest, "[store.grid.enter] failed to read a scope", err))
	}

	for index := 1; index < scopes.Len(); index++ {
		other, err := scopes.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.BadRequest, "[store.grid.enter] failed to read a scope", err))
		}

		if other != scope {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[store.grid.enter] data written together belongs to different scopes: "+scope+", "+other,
				nil,
			))
		}
	}

	if scope == server.scope {
		return nil
	}

	server.selectScope(scope)
	return nil
}

/* selectScope resumes the retained readings owned by exactly this market. */
func (server *GridServer) selectScope(scope string) {
	series, found := server.series[scope]

	if !found {
		series = &gridSeries{}
		server.series[scope] = series
	}
	server.gridSeries, server.scope = series, scope
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
	if err := results.SetData(server.raw); err != nil {
		return errnie.Error(err)
	}

	if err := results.SetScope(server.scope); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[store.grid.Done] failed to set scope",
			err,
		))
	}
	results.SetMetrics(int64(len(server.metrics)))

	if err := server.deliver(results); err != nil {
		return err
	}

	if len(server.metrics) > 0 {
		observations, err := results.NewObservations(int32(len(server.metrics)))

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[store.grid.Done] failed to allocate observations",
				err,
			))
		}

		observed, err := results.NewObserved(int32(len(server.observed)))

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[store.grid.Done] failed to allocate observed",
				err,
			))
		}

		// A reading is handed out once. The next evaluation sees it as
		// observed only if a new commit reported it again.
		for index, value := range server.metrics {
			observations.Set(index, value)
			observed.Set(index, server.observed[index])
			server.observed[index] = false
		}
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

	if server.scopePath != "" {
		value, found := walkInterest(document, strings.Split(server.scopePath, "."))
		scope, textual := value.(string)

		if !found || !textual || scope == "" {
			return errnie.Error(errnie.Err(errnie.Validation, "grid: record requires a nonempty Text scope at "+server.scopePath, nil))
		}
		server.selectScope(scope)
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
	server.raw = bytes.Clone(payload)
	server.delivered = int64(len(resolved))
	server.values = make([]float64, len(server.interests))
	server.present = make([]bool, len(server.interests))

	// Every declared interest keeps its own slot whether or not this record
	// carried it, so a metric reads the slot it asked for rather than
	// whichever field happened to land ahead of it.
	for index, interest := range server.interests {
		field, keep := strings.CutPrefix(interest, heldPrefix)
		value, numeric := readInterest(resolved, field)

		if keep {
			if numeric {
				if server.held == nil {
					server.held = make(map[string]float64)
				}

				server.held[field] = value
			}

			value, numeric = server.held[field]
		}

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
heldPrefix marks an interest whose last reading the grid keeps: held:field is
delivered with every record once any record has carried field, within its own
scope, including after another scope has been processed. It is how one feed's reading is read beside
another's, which arrive on different records.
*/
const heldPrefix = "held:"

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
		field, _, _ := strings.Cut(strings.TrimPrefix(interest, heldPrefix), "=")
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
		switch value := resolved[interest].(type) {
		case float64:
			return value, true
		case []any:
			sum := 0.0

			for _, item := range value {
				order, ok := item.(map[string]any)

				if ok {
					qty, hasQty := order["order_qty"].(float64)

					if hasQty {
						sum += qty
					}
				}
			}

			if sum > 0 {
				return sum, true
			}

			return float64(len(value)), true
		case string:
			// A time is read as the instant it names, in nanoseconds since
			// the epoch, so a metric asking for a timestamp gets a number.
			instant, err := time.Parse(time.RFC3339Nano, value)

			if err != nil {
				return 0, false
			}

			return float64(instant.UnixNano()), true
		}

		return 0, false
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

			if ok && position < len(elements) {
				current = elements[position]
				continue
			}

			if len(elements) == 1 {
				current = elements[0]
			}

			if len(elements) != 1 && (!ok || position >= len(elements)) {
				return nil, false
			}
		}

		object, ok := current.(map[string]any)

		if !ok {
			return nil, false
		}

		// A replayed record carries the venue's frame under market beside the
		// capture that received it; a live one carries both at its root. A
		// field is looked for on the record first and then in its frame.
		market, hasMarket := object["market"].(map[string]any)
		channel, tagged := object["channel"].(string)

		if !tagged && hasMarket {
			channel, tagged = market["channel"].(string)
		}

		// A channel-tagged record has the same declared path as its named
		// envelope: ticker.data.last addresses channel=ticker, data.last.
		if tagged && channel == segment {
			continue
		}

		next, found := object[segment]

		if !found && segment == "bid" {
			bids, ok := object["bids"].([]any)

			if ok && len(bids) > 0 {
				first, isObj := bids[0].(map[string]any)

				if isObj {
					next, found = first["limit_price"]
				}
			}
		}

		if !found && segment == "bid_qty" {
			bids, ok := object["bids"].([]any)

			if ok && len(bids) > 0 {
				first, isObj := bids[0].(map[string]any)

				if isObj {
					next, found = first["order_qty"]
				}
			}
		}

		if !found && segment == "ask" {
			asks, ok := object["asks"].([]any)

			if ok && len(asks) > 0 {
				first, isObj := asks[0].(map[string]any)

				if isObj {
					next, found = first["limit_price"]
				}
			}
		}

		if !found && segment == "ask_qty" {
			asks, ok := object["asks"].([]any)

			if ok && len(asks) > 0 {
				first, isObj := asks[0].(map[string]any)

				if isObj {
					next, found = first["order_qty"]
				}
			}
		}

		if !found && hasMarket {
			next, found = market[segment]
		}

		if !found {
			return nil, false
		}

		current = next
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
