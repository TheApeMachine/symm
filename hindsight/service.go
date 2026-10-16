package hindsight

import (
	"context"
	"fmt"
	"iter"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy"
)

/*
Run is one stored epoch.
*/
type Run struct {
	Epoch     uint64    `json:"epoch"`
	StartedAt time.Time `json:"started_at"`
	Status    string    `json:"status"`
}

/*
Service serves the stored ticks (data.TickPrefix), the excursions detect
wrote and the policies train published, read through the store only by the
prefixes their writers key them under. A missing store is 503; a missing or
malformed query parameter is 400; a stored Value that does not read is 500.
*/
type Service struct {
	ctx         context.Context
	client      *store.Client
	fragmentsMu sync.Mutex
	fragments   map[uint64][]TrainedFragmentResponse
	excursions  map[int64]*data.Value
}

func NewService(ctx context.Context, client *store.Client) *Service {
	return &Service{
		ctx:        ctx,
		client:     client,
		fragments:  make(map[uint64][]TrainedFragmentResponse),
		excursions: make(map[int64]*data.Value),
	}
}

func (service *Service) Register(router fiber.Router) {
	if service == nil || router == nil {
		return
	}

	router.Get("/hindsight/runs", service.runs)

	router.Use("/hindsight/timeline", func(ctx fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(ctx) {
			ctx.Locals("allowed", true)
			return ctx.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	router.Get("/hindsight/timeline", websocket.New(service.timeline, websocket.Config{
		Origins: []string{"*"},
	}))

	router.Get("/hindsight/symbols", service.symbols)
	router.Get("/hindsight/excursions", service.excursionsHandler)
	router.Get("/hindsight/data", service.data)
	router.Get("/hindsight/captures", service.captures)
	router.Get("/hindsight/envelope", service.envelope)

	router.Get("/training/fragments", service.trainingFragments)
	router.Get("/training/fragments/:id/points", service.trainingFragmentPoints)

	router.Get("/hindsight/gaps", service.gaps)
	router.Get("/hindsight/lifecycle", service.lifecycle)
	router.Get("/hindsight/states", service.states)
	router.Get("/hindsight/state", service.state)
	router.Get("/hindsight/metric-map", service.metricMap)
}

/*
metricMap has no source: no metric registry exists to serve. It answers 501
rather than an empty map that reads as "no metrics".
*/
func (service *Service) metricMap(ctx fiber.Ctx) error {
	return fiber.NewError(fiber.StatusNotImplemented, "no metric map is published")
}

func (service *Service) available() error {
	if service.client == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
	}

	return nil
}

func badRequest(format string, args ...any) error {
	return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf(format, args...))
}

func readFailure(err error) error {
	return fiber.NewError(fiber.StatusInternalServerError, errnie.Error(err).Error())
}

/*
queryUint parses name; absent is (0, false, nil) unless required, which is
a 400, as is a value that does not parse.
*/
func queryUint(raw, name string, required bool) (uint64, bool, error) {
	if raw == "" {
		if required {
			return 0, false, badRequest("%s is required", name)
		}

		return 0, false, nil
	}

	value, err := strconv.ParseUint(raw, 10, 64)

	if err != nil {
		return 0, false, badRequest("%s %q is not an unsigned integer", name, raw)
	}

	return value, true, nil
}

/*
queryEpoch reads the required run (or epoch) parameter.
*/
func queryEpoch(ctx fiber.Ctx) (uint64, error) {
	run := ctx.Query("run")

	if run == "" {
		run = ctx.Query("epoch")
	}

	epoch, _, err := queryUint(run, "run (or epoch)", true)
	return epoch, err
}

func querySymbol(ctx fiber.Ctx) (string, error) {
	symbol := ctx.Query("symbol")

	if symbol == "" {
		return "", badRequest("symbol is required")
	}

	return symbol, nil
}

/*
ticks yields epoch's stored ticks in order, only those that trade symbol when
symbol is set, each with its decoded symbols.
*/
func (service *Service) ticks(epoch uint64, symbol string) iter.Seq2[*data.Value, error] {
	return func(yield func(*data.Value, error) bool) {
		for value := range store.Values(service.client, data.TickPrefix(epoch)) {
			if symbol != "" {
				symbolTicks, err := broker.ReadTick(value)

				if err != nil {
					yield(nil, err)
					return
				}

				if !slices.ContainsFunc(symbolTicks, func(tick *broker.SymbolTick) bool { return tick.Symbol == symbol }) {
					continue
				}
			}

			if !yield(value, nil) {
				return
			}
		}
	}
}

/*
tickAt downloads the stored tick with number tick in epoch, by its key.
*/
func (service *Service) tickAt(epoch, tick uint64) (*data.Value, error) {
	key := (&data.Value{Origin: data.TickOrigin, Role: data.TickRole, Scope: data.TickScope, Epoch: epoch, Tick: tick}).FormatPrefix()
	payload, err := service.client.Download(key)

	if err != nil {
		return nil, fiber.NewError(fiber.StatusNotFound, "no stored tick "+key)
	}

	value := data.New()

	if _, err := value.Write(payload); err != nil {
		return nil, readFailure(errnie.Err(errnie.UnprocessableContent, key+" does not decode", err))
	}

	return value, nil
}

func (service *Service) runs(ctx fiber.Ctx) error {
	if err := service.available(); err != nil {
		return err
	}

	epochs, err := store.Epochs(service.client, data.TickPrefix())

	if err != nil {
		return readFailure(err)
	}

	runs := make([]Run, 0, len(epochs))

	for _, epoch := range epochs {
		runs = append(runs, Run{Epoch: epoch, StartedAt: time.Unix(0, int64(epoch)).UTC(), Status: "recorded"})
	}

	return ctx.JSON(runs)
}

func (service *Service) timeline(conn *websocket.Conn) {
	if service.client == nil {
		errnie.Error(errnie.Err(errnie.Validation, "[hindsight] timeline: capture store unavailable", nil))
		return
	}

	run := conn.Query("run")

	if run == "" {
		run = conn.Query("epoch")
	}

	epoch, _, err := queryUint(run, "run (or epoch)", true)

	if err != nil {
		errnie.Error(err)
		return
	}

	fromTick, _, err := queryUint(conn.Query("from"), "from", false)

	if err != nil {
		errnie.Error(err)
		return
	}

	toTick, bounded, err := queryUint(conn.Query("to"), "to", false)

	if err != nil {
		errnie.Error(err)
		return
	}

	const timelineBatchSize = 256
	batch := make([]*data.Value, 0, timelineBatchSize)

	flush := func() bool {
		payload, err := sonic.Marshal(batch)

		if err != nil {
			errnie.Error(err)
			return false
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
			errnie.Error(err)
			return false
		}

		batch = batch[:0]
		return true
	}

	for value, err := range service.ticks(epoch, conn.Query("symbol")) {
		if err != nil {
			errnie.Error(err)
			return
		}

		if value.Tick < fromTick {
			continue
		}

		if bounded && value.Tick > toTick {
			break
		}

		batch = append(batch, value)

		if len(batch) == timelineBatchSize && !flush() {
			return
		}
	}

	if len(batch) > 0 {
		flush()
	}
}

func (service *Service) symbols(ctx fiber.Ctx) error {
	if err := service.available(); err != nil {
		return err
	}

	epoch, err := queryEpoch(ctx)

	if err != nil {
		return err
	}

	symbols, err := service.epochSymbols(epoch)

	if err != nil {
		return readFailure(err)
	}

	return ctx.JSON(symbols)
}

/*
epochSymbols is every symbol epoch's ticks trade, in first-trade order.
*/
func (service *Service) epochSymbols(epoch uint64) ([]string, error) {
	symbols := make([]string, 0)

	for value, err := range service.ticks(epoch, "") {
		if err != nil {
			return nil, err
		}

		symbolTicks, err := broker.ReadTick(value)

		if err != nil {
			return nil, err
		}

		for _, tick := range symbolTicks {
			if !slices.Contains(symbols, tick.Symbol) {
				symbols = append(symbols, tick.Symbol)
			}
		}
	}

	return symbols, nil
}

func (service *Service) excursionsHandler(ctx fiber.Ctx) error {
	if err := service.available(); err != nil {
		return err
	}

	epoch, err := queryEpoch(ctx)

	if err != nil {
		return err
	}

	symbol, err := querySymbol(ctx)

	if err != nil {
		return err
	}

	excursions := make([]*data.Value, 0)

	for value := range store.Values(service.client, strategy.ExcursionPrefixes(symbol, epoch)...) {
		excursions = append(excursions, value)
	}

	return ctx.JSON(excursions)
}

func (service *Service) data(ctx fiber.Ctx) error {
	if err := service.available(); err != nil {
		return err
	}

	epoch, err := queryEpoch(ctx)

	if err != nil {
		return err
	}

	limit, limited, err := queryUint(ctx.Query("limit"), "limit", false)

	if err != nil {
		return err
	}

	values := make([]*data.Value, 0)

	for value, err := range service.ticks(epoch, "") {
		if err != nil {
			return readFailure(err)
		}

		values = append(values, value)

		if limited && uint64(len(values)) >= limit {
			break
		}
	}

	return ctx.JSON(values)
}

func receivedAt(value *data.Value) string {
	return time.Unix(0, int64(value.Timestamp)).UTC().Format("2006-01-02T15:04:05.000Z07:00")
}

func identity(value *data.Value) HindsightCaptureIdentity {
	return HindsightCaptureIdentity{
		Run:            strconv.FormatUint(value.Epoch, 10),
		Sequence:       value.Tick,
		Stream:         value.Origin,
		StreamEpoch:    value.Epoch,
		StreamSequence: value.Tick,
	}
}

func (service *Service) captures(ctx fiber.Ctx) error {
	if err := service.available(); err != nil {
		return err
	}

	epoch, err := queryEpoch(ctx)

	if err != nil {
		return err
	}

	after, _, err := queryUint(ctx.Query("after"), "after", false)

	if err != nil {
		return err
	}

	const maxCaptures = 64
	captures := make([]map[string]any, 0, maxCaptures)

	for value, err := range service.ticks(epoch, ctx.Query("symbol")) {
		if err != nil {
			return readFailure(err)
		}

		if value.Tick < after {
			continue
		}

		captures = append(captures, map[string]any{
			"identity":   identity(value),
			"kind":       "tick",
			"endpoint":   value.Origin,
			"receivedAt": receivedAt(value),
		})

		if len(captures) >= maxCaptures {
			break
		}
	}

	return ctx.JSON(captures)
}

func (service *Service) envelope(ctx fiber.Ctx) error {
	if err := service.available(); err != nil {
		return err
	}

	epoch, err := queryEpoch(ctx)

	if err != nil {
		return err
	}

	tick, _, err := queryUint(ctx.Query("seq"), "seq (tick)", true)

	if err != nil {
		return err
	}

	found, err := service.tickAt(epoch, tick)

	if err != nil {
		return err
	}

	payload, err := sonic.Marshal(found)

	if err != nil {
		return readFailure(err)
	}

	return ctx.JSON(HindsightEnvelope{Payload: string(payload)})
}

type HindsightEnvelope struct {
	Payload string `json:"payload"`
}

type HindsightGap struct {
	RunId    string `json:"runId"`
	Encoding string `json:"encoding"`
	Sequence uint64 `json:"sequence"`
	Detail   string `json:"detail"`
}

type HindsightLifecycleEvent struct {
	DecisionId string `json:"decisionId"`
	Symbol     string `json:"symbol"`
	Kind       string `json:"kind"`
	Action     string `json:"action"`
	At         string `json:"at"`
	CaptureSeq uint64 `json:"captureSeq"`
}

type HindsightCaptureIdentity struct {
	Run            string `json:"run"`
	Sequence       uint64 `json:"sequence"`
	Stream         string `json:"stream"`
	StreamEpoch    uint64 `json:"streamEpoch"`
	StreamSequence uint64 `json:"streamSequence"`
}

type HindsightStateEnvelope struct {
	Origin  HindsightCaptureIdentity `json:"origin"`
	Ordinal int                      `json:"ordinal"`
}

type HindsightState struct {
	Envelope HindsightStateEnvelope `json:"envelope"`
	Payload  string                 `json:"payload"`
}

/*
gaps reports jumps in the stored tick numbers of an epoch: every trade frame
is one tick, so a jump is frames that were never stored.
*/
func (service *Service) gaps(ctx fiber.Ctx) error {
	if err := service.available(); err != nil {
		return err
	}

	epoch, err := queryEpoch(ctx)

	if err != nil {
		return err
	}

	gaps := make([]HindsightGap, 0)
	var previous uint64
	started := false

	for value, err := range service.ticks(epoch, "") {
		if err != nil {
			return readFailure(err)
		}

		if started && value.Tick > previous+1 {
			gaps = append(gaps, HindsightGap{
				RunId:    strconv.FormatUint(epoch, 10),
				Encoding: "tick_gap",
				Sequence: previous + 1,
				Detail:   fmt.Sprintf("Tick jump from %d to %d (missing %d ticks)", previous, value.Tick, value.Tick-previous-1),
			})
		}

		previous, started = value.Tick, true
	}

	return ctx.JSON(gaps)
}

func (service *Service) lifecycle(ctx fiber.Ctx) error {
	if err := service.available(); err != nil {
		return err
	}

	epoch, err := queryEpoch(ctx)

	if err != nil {
		return err
	}

	symbol, err := querySymbol(ctx)

	if err != nil {
		return err
	}

	events := make([]HindsightLifecycleEvent, 0)

	for value := range store.Values(service.client, strategy.ExcursionPrefixes(symbol, epoch)...) {
		events = append(events, HindsightLifecycleEvent{
			DecisionId: value.Prefix,
			Symbol:     value.Scope,
			Kind:       "detector",
			Action:     value.Role,
			At:         receivedAt(value),
			CaptureSeq: value.Tick,
		})
	}

	for value := range store.Values(service.client, strategy.PolicyPrefixes(epoch)...) {
		events = append(events, HindsightLifecycleEvent{
			DecisionId: value.Prefix,
			Symbol:     value.Scope,
			Kind:       "policy",
			Action:     value.Role,
			CaptureSeq: value.Tick,
		})
	}

	return ctx.JSON(events)
}

func (service *Service) states(ctx fiber.Ctx) error {
	if err := service.available(); err != nil {
		return err
	}

	epoch, err := queryEpoch(ctx)

	if err != nil {
		return err
	}

	const maxStates = 64
	states := make([]HindsightState, 0, maxStates)

	for value, err := range service.ticks(epoch, ctx.Query("symbol")) {
		if err != nil {
			return readFailure(err)
		}

		payload, err := sonic.Marshal(value)

		if err != nil {
			return readFailure(errnie.Err(errnie.UnprocessableContent, "[hindsight] state marshal failed for "+value.Prefix, err))
		}

		states = append(states, HindsightState{
			Envelope: HindsightStateEnvelope{Origin: identity(value)},
			Payload:  string(payload),
		})

		if len(states) >= maxStates {
			break
		}
	}

	return ctx.JSON(states)
}

func (service *Service) state(ctx fiber.Ctx) error {
	if err := service.available(); err != nil {
		return err
	}

	epoch, err := queryEpoch(ctx)

	if err != nil {
		return err
	}

	tick, _, err := queryUint(ctx.Query("seq"), "seq (tick)", true)

	if err != nil {
		return err
	}

	ordinal, _, err := queryUint(ctx.Query("ordinal"), "ordinal", false)

	if err != nil {
		return err
	}

	found, err := service.tickAt(epoch, tick)

	if err != nil {
		return err
	}

	payload, err := sonic.Marshal(found)

	if err != nil {
		return readFailure(err)
	}

	return ctx.JSON(HindsightState{
		Envelope: HindsightStateEnvelope{Origin: identity(found), Ordinal: int(ordinal)},
		Payload:  string(payload),
	})
}

type FragmentPoint struct {
	X    int     `json:"x"`
	Y    float64 `json:"y"`
	Seq  uint64  `json:"seq"`
	Time int64   `json:"time"`
}

/*
TrainedFragmentResponse is one detected excursion: marks A, B and C are
ticks, entry and exit the touch it crosses (ask at B, bid at C).
*/
type TrainedFragmentResponse struct {
	Id         int64           `json:"id"`
	Symbol     string          `json:"symbol"`
	Epoch      uint64          `json:"epoch"`
	MarkA      int64           `json:"mark_a"`
	MarkB      int64           `json:"mark_b"`
	MarkC      int64           `json:"mark_c"`
	EntryPrice float64         `json:"entry_price"`
	ExitPrice  float64         `json:"exit_price"`
	Magnitude  float64         `json:"magnitude"`
	Direction  string          `json:"direction"`
	Class      string          `json:"class"`
	Points     []FragmentPoint `json:"points"`
	EntryIdx   int64           `json:"entry_idx"`
	ExitIdx    int64           `json:"exit_idx"`
	LearnedAt  string          `json:"learned_at"`
}

func excursionFields(value *data.Value) (map[string]float64, error) {
	metrics := data.Metrics(value)
	fields := make(map[string]float64, len(strategy.ExcursionKeys))

	for _, key := range strategy.ExcursionKeys {
		field, ok := metrics[key]

		if !ok {
			return nil, errnie.Err(errnie.UnprocessableContent, value.Prefix+" has no "+key, nil)
		}

		fields[key] = field
	}

	return fields, nil
}

/*
fragmentsOf is every excursion of epoch, over the symbols its ticks trade.
Ids are stable within the process: one per excursion key.
*/
func (service *Service) fragmentsOf(epoch uint64) ([]TrainedFragmentResponse, error) {
	service.fragmentsMu.Lock()
	defer service.fragmentsMu.Unlock()

	if cached, ok := service.fragments[epoch]; ok {
		return cached, nil
	}

	symbols, err := service.epochSymbols(epoch)

	if err != nil {
		return nil, err
	}

	fragments := make([]TrainedFragmentResponse, 0)

	for _, symbol := range symbols {
		for value := range store.Values(service.client, strategy.ExcursionPrefixes(symbol, epoch)...) {
			fields, err := excursionFields(value)

			if err != nil {
				return nil, err
			}

			id := int64(len(service.excursions) + 1)
			service.excursions[id] = value

			entry, exit := fields["b_ask"], fields["c_bid"]
			direction := "up"

			if exit < entry {
				direction = "down"
			}

			fragments = append(fragments, TrainedFragmentResponse{
				Id:         id,
				Symbol:     symbol,
				Epoch:      epoch,
				MarkA:      int64(fields["a_tick"]),
				MarkB:      int64(fields["b_tick"]),
				MarkC:      int64(fields["c_tick"]),
				EntryPrice: entry,
				ExitPrice:  exit,
				Magnitude:  (exit - entry) / entry,
				Direction:  direction,
				Class:      value.Role,
				Points:     []FragmentPoint{},
				EntryIdx:   int64(fields["b_idx"] - fields["a_idx"]),
				ExitIdx:    int64(fields["c_idx"] - fields["a_idx"]),
				LearnedAt:  receivedAt(value),
			})
		}
	}

	service.fragments[epoch] = fragments
	return fragments, nil
}

func (service *Service) trainingFragments(ctx fiber.Ctx) error {
	if err := service.available(); err != nil {
		return err
	}

	epoch, err := queryEpoch(ctx)

	if err != nil {
		return err
	}

	fragments, err := service.fragmentsOf(epoch)

	if err != nil {
		return readFailure(err)
	}

	return ctx.JSON(fragments)
}

/*
trainingFragmentPoints is the traded price path of one fragment's symbol
from mark A to mark C.
*/
func (service *Service) trainingFragmentPoints(ctx fiber.Ctx) error {
	if err := service.available(); err != nil {
		return err
	}

	id, err := strconv.ParseInt(ctx.Params("id"), 10, 64)

	if err != nil {
		return badRequest("fragment id %q is not an integer", ctx.Params("id"))
	}

	service.fragmentsMu.Lock()
	excursion := service.excursions[id]
	service.fragmentsMu.Unlock()

	if excursion == nil {
		return fiber.NewError(fiber.StatusNotFound, "fragment not loaded: list /training/fragments for its epoch first")
	}

	fields, err := excursionFields(excursion)

	if err != nil {
		return readFailure(err)
	}

	startTick, endTick := uint64(max(fields["a_tick"], 0)), uint64(fields["c_tick"])
	points := make([]FragmentPoint, 0)

	for value, err := range service.ticks(excursion.Epoch, excursion.Scope) {
		if err != nil {
			return readFailure(err)
		}

		if value.Tick < startTick {
			continue
		}

		if value.Tick > endTick {
			break
		}

		symbolTicks, err := broker.ReadTick(value)

		if err != nil {
			return readFailure(err)
		}

		for _, tick := range symbolTicks {
			if tick.Symbol != excursion.Scope {
				continue
			}

			for _, trade := range tick.Trades {
				points = append(points, FragmentPoint{
					X:    len(points),
					Y:    trade.Price.Float64(),
					Seq:  value.Tick,
					Time: trade.Timestamp.UnixMilli(),
				})
			}
		}
	}

	return ctx.JSON(points)
}
