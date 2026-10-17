package hindsight

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
)

/*
Run is one stored epoch of collected ticks. ID is the epoch in decimal: an
epoch exceeds the integers a JSON number carries exactly in the browser.
Policies are the policy epochs train published from these ticks, newest
first, each with the gate it was trained under (a gated and an ungated
training of the same ticks are two entries); Trained is whether any exist.
*/
type Run struct {
	ID        string       `json:"id"`
	StartedAt string       `json:"startedAt"`
	Status    string       `json:"status"`
	Trained   bool         `json:"trained"`
	Policies  []PolicyInfo `json:"policies"`
}

/*
PolicyInfo is one policy epoch trained on a run and its data.PolicyGate.
*/
type PolicyInfo struct {
	Epoch        string  `json:"epoch"`
	Gate         bool    `json:"gate"`
	SurpriseZ    float64 `json:"surpriseZ"`
	Warmup       string  `json:"warmup"`
	LearningRate float64 `json:"learningRate"`
}

/*
Service replays what the commands stored for one run (data/schema.go):
the ticks collect wrote, the excursions detect found, the policies train
published and the paper records traded under them. It reads through a
store.Reader only, so it cannot write storage. The run list is its one REST
route; everything else streams as hindsight Values over the hub websocket
(Stream), on the client's "hindsight" command.
*/
type Service struct {
	ctx    context.Context
	reader *store.Reader
}

func NewService(ctx context.Context, reader *store.Reader) (*Service, error) {
	if reader == nil {
		return nil, errnie.Err(errnie.Validation, "[hindsight] a store reader is required", nil)
	}

	return &Service{ctx: ctx, reader: reader}, nil
}

/*
Register mounts GET /hindsight/runs on router.
*/
func (service *Service) Register(router fiber.Router) {
	router.Get("/hindsight/runs", func(ctx fiber.Ctx) error {
		runs, err := service.Runs()

		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, errnie.Error(err).Error())
		}

		return ctx.JSON(runs)
	})
}

/*
Runs lists every epoch with stored ticks, newest first.
*/
func (service *Service) Runs() ([]Run, error) {
	epochs, err := service.reader.Epochs(data.TickPrefix())

	if err != nil {
		return nil, err
	}

	gates, err := service.reader.PolicyGates()

	if err != nil {
		return nil, err
	}

	trained := make(map[uint64][]PolicyInfo)

	for _, gate := range slices.Backward(gates) {
		trained[gate.TickEpoch] = append(trained[gate.TickEpoch], PolicyInfo{
			Epoch: strconv.FormatUint(gate.Epoch, 10), Gate: gate.Enabled, SurpriseZ: gate.SurpriseZ,
			Warmup: gate.Warmup.String(), LearningRate: gate.LearningRate,
		})
	}

	runs := make([]Run, 0, len(epochs))

	for _, epoch := range slices.Backward(epochs) {
		policies := trained[epoch]

		if policies == nil {
			policies = []PolicyInfo{}
		}

		runs = append(runs, Run{
			ID:        strconv.FormatUint(epoch, 10),
			StartedAt: time.Unix(0, int64(epoch)).UTC().Format(time.RFC3339Nano),
			Status:    "recorded",
			Trained:   len(policies) > 0,
			Policies:  policies,
		})
	}

	return runs, nil
}

/*
batchBytes bounds one websocket message of hindsight frames. Frames are
whole serialized Values back to back, which the dashboard parses as a batch.
*/
const batchBytes = 256 << 10

/*
batch gathers whole Values (one Write each, data.Value.WriteTo) into
messages of about batchBytes, each a fresh buffer handed to dst.
*/
type batch struct {
	dst io.Writer
	buf []byte
}

func (batch *batch) Write(p []byte) (int, error) {
	batch.buf = append(batch.buf, p...)

	if len(batch.buf) >= batchBytes {
		if err := batch.Flush(); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}

func (batch *batch) Flush() error {
	if len(batch.buf) == 0 {
		return nil
	}

	out := batch.buf
	batch.buf = make([]byte, 0, batchBytes)

	_, err := batch.dst.Write(out)
	return err
}

/*
Stream writes run's hindsight frames to w. With a symbol: the policies of
the newest policy epoch trained on the run's ticks, the symbol's excursions,
the paper records traded under that policy epoch, then one tick frame per stored tick that traded the symbol. With
no symbol: one summary frame per symbol the run traded (Scope "", the
symbol in its key). A done frame closes
a complete stream. A failure is written as an error frame and returned; a
canceled ctx (a newer request, a closed client) returns ctx.Err() with no
frame.
*/
func (service *Service) Stream(ctx context.Context, w io.Writer, run uint64, symbol string) error {
	out := &batch{dst: w, buf: make([]byte, 0, batchBytes)}
	var err error

	if symbol == "" {
		err = service.summaries(ctx, out, run)
	} else {
		err = service.symbol(ctx, out, run, symbol)
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if err != nil {
		failure := frame(data.HindsightError, run, symbol, 0)
		failure.AppendJSON([]byte(err.Error()), data.LayerHindsightError)

		if _, copyErr := io.Copy(out, failure); copyErr != nil {
			err = errors.Join(err, copyErr)
		}
	}

	return errors.Join(err, out.Flush())
}

/*
frame is an empty hindsight Value of role for symbol in run.
*/
func frame(role string, run uint64, symbol string, tick uint64) *data.Value {
	value := data.NewValue(data.HindsightOrigin, role, symbol)
	value.Epoch, value.Tick = run, tick
	value.FormatPrefix()
	return value
}

/*
forward copies a stored Value to w as a hindsight frame of the requested
run: Origin becomes HindsightOrigin, Role the store origin it came from,
Scope the symbol and Epoch the run (a paper record's own run epoch stays in
its key). Its layers, whose indices name the stored key, are untouched.
*/
/*
policies are the Values of policy epoch, none when epoch is 0.
*/
func (service *Service) policies(epoch uint64) iter.Seq2[*data.Value, error] {
	if epoch == 0 {
		return func(func(*data.Value, error) bool) {}
	}

	return service.reader.Values(data.PolicyPrefixes(epoch)...)
}

func forward(w io.Writer, value *data.Value, run uint64, symbol string) error {
	value.Role, value.Origin, value.Scope, value.Epoch = value.Origin, data.HindsightOrigin, symbol, run

	if _, err := io.Copy(w, value); err != nil {
		return fmt.Errorf("forwarding %s: %w", value.Prefix, err)
	}

	return nil
}

/*
newestPolicy is the newest policy epoch trained on run's ticks, 0 when none.
*/
func (service *Service) newestPolicy(run uint64) (uint64, error) {
	gates, err := service.reader.PolicyGates()

	if err != nil {
		return 0, err
	}

	var newest uint64

	for _, gate := range gates {
		if gate.TickEpoch == run {
			newest = max(newest, gate.Epoch)
		}
	}

	return newest, nil
}

func (service *Service) symbol(ctx context.Context, w io.Writer, run uint64, symbol string) error {
	counts := map[string]float64{}

	// The newest training of these ticks; with none, no policy and no paper
	// record is streamed.
	policyEpoch, err := service.newestPolicy(run)

	if err != nil {
		return err
	}

	for value, err := range service.policies(policyEpoch) {
		if err != nil {
			return err
		}

		if err := forward(w, value, run, symbol); err != nil {
			return err
		}

		counts["policies"]++
	}

	for value, err := range service.reader.Values(data.ExcursionPrefixes(symbol, run)...) {
		if err != nil {
			return err
		}

		if _, err := data.ExcursionFields(value); err != nil {
			return err
		}

		if err := forward(w, value, run, symbol); err != nil {
			return err
		}

		counts["excursions"]++
	}

	for value, err := range service.reader.Values(data.PaperPrefixes(symbol)...) {
		if err != nil {
			return err
		}

		record, err := data.PaperFields(value)

		if err != nil {
			return err
		}

		if policyEpoch == 0 || record.PolicyEpoch != policyEpoch {
			continue
		}

		if err := forward(w, value, run, symbol); err != nil {
			return err
		}

		counts["papers"]++
	}

	for value, err := range service.reader.Values(data.TickPrefix(run)) {
		if err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		ticks, err := broker.ReadTick(value)

		if err != nil {
			return err
		}

		index := slices.IndexFunc(ticks, func(tick *broker.SymbolTick) bool { return tick.Symbol == symbol })

		if index < 0 {
			continue
		}

		tick := frame(data.HindsightTick, run, symbol, value.Tick)
		tick.Timestamp = value.Timestamp
		data.AppendVector(tick, tickVector(ticks[index]), data.HindsightTickKeys...)

		if _, err := io.Copy(w, tick); err != nil {
			return fmt.Errorf("writing tick %d: %w", value.Tick, err)
		}

		counts["ticks"]++
	}

	if counts["ticks"] == 0 {
		return errnie.Err(errnie.NotFound, fmt.Sprintf("[hindsight] run %d has no stored tick trading %s", run, symbol), nil)
	}

	counts["symbols"] = 1
	return done(w, run, symbol, counts)
}

/*
tickVector is a symbol's HindsightTickKeys at one tick: the touch the tick
recorded (NaN where the price layer holds no side: undefined, not zero),
the last trade's price, the traded quantity and the trade count.
*/
func tickVector(tick *broker.SymbolTick) []float64 {
	side := func(price interface{ Float64() float64 }, held bool) float64 {
		if !held {
			return math.NaN()
		}

		return price.Float64()
	}

	qty := 0.0

	for _, trade := range tick.Trades {
		qty += trade.Qty
	}

	last := tick.Trades[len(tick.Trades)-1]

	return []float64{
		side(tick.Price.Bid, tick.Price.Bid != nil),
		side(tick.Price.Ask, tick.Price.Ask != nil),
		last.Price.Float64(),
		qty,
		float64(len(tick.Trades)),
	}
}

type summary struct {
	ticks, touched      float64
	trades              float64
	firstTick, lastTick uint64
	firstAt, lastAt     uint64
	low, high           float64
}

func (service *Service) summaries(ctx context.Context, w io.Writer, run uint64) error {
	order := make([]string, 0)
	symbols := make(map[string]*summary)
	ticks := 0.0

	for value, err := range service.reader.Values(data.TickPrefix(run)) {
		if err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		symbolTicks, err := broker.ReadTick(value)

		if err != nil {
			return err
		}

		ticks++

		for _, tick := range symbolTicks {
			entry, ok := symbols[tick.Symbol]

			if !ok {
				entry = &summary{firstTick: value.Tick, firstAt: value.Timestamp, low: math.Inf(1), high: math.Inf(-1)}
				symbols[tick.Symbol] = entry
				order = append(order, tick.Symbol)
			}

			entry.ticks++

			if tick.Price.Bid != nil && tick.Price.Ask != nil {
				entry.touched++
			}
			entry.trades += float64(len(tick.Trades))
			entry.lastTick, entry.lastAt = value.Tick, value.Timestamp

			for _, trade := range tick.Trades {
				price := trade.Price.Float64()
				entry.low, entry.high = min(entry.low, price), max(entry.high, price)
			}
		}
	}

	if ticks == 0 {
		return errnie.Err(errnie.NotFound, "[hindsight] no stored ticks under "+data.TickPrefix(run), nil)
	}

	excursions := 0.0

	for _, symbol := range order {
		entry := symbols[symbol]
		classes := map[string]float64{}
		top := 0.0

		for value, err := range service.reader.Values(data.ExcursionPrefixes(symbol, run)...) {
			if err != nil {
				return err
			}

			classes[value.Role]++
			excursions++

			if value.Role != "up" && value.Role != "down" {
				continue
			}

			fields, err := data.ExcursionFields(value)

			if err != nil {
				return err
			}

			if fields["b_price"] <= 0 {
				return errnie.Err(errnie.UnprocessableContent, "[hindsight] excursion "+value.Prefix+" has no positive b_price", nil)
			}

			if move := (fields["c_price"] - fields["b_price"]) / fields["b_price"]; math.Abs(move) > math.Abs(top) {
				top = move
			}
		}

		summary := frame(data.HindsightSymbol, run, symbol, 0)
		data.AppendVector(summary, []float64{
			entry.ticks, entry.touched, entry.trades,
			float64(entry.firstTick), float64(entry.lastTick),
			float64(entry.firstAt), float64(entry.lastAt),
			entry.low, entry.high,
			classes["up"], classes["down"], top,
		}, data.HindsightSymbolKeys...)

		// The run index answers the request for symbol "": its frames carry
		// that Scope, and the summarized symbol stays in their key.
		summary.Scope = ""

		if _, err := io.Copy(w, summary); err != nil {
			return fmt.Errorf("writing %s summary: %w", symbol, err)
		}
	}

	return done(w, run, "", map[string]float64{
		"ticks": ticks, "symbols": float64(len(order)), "excursions": excursions,
	})
}

func done(w io.Writer, run uint64, symbol string, counts map[string]float64) error {
	vector := make([]float64, len(data.HindsightDoneKeys))

	for idx, key := range data.HindsightDoneKeys {
		vector[idx] = counts[key]
	}

	closing := frame(data.HindsightDone, run, symbol, 0)
	data.AppendVector(closing, vector, data.HindsightDoneKeys...)

	if _, err := io.Copy(w, closing); err != nil {
		return fmt.Errorf("writing done: %w", err)
	}

	return nil
}
