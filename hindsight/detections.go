package hindsight

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
)

/*
Detection names what one "detections" command asks for (data/schema.go,
the Historical runs tab): Run 0 for the epochs detect labelled; a Run for its
excursions; a Run, Symbol, Class and CTick for that excursion's trades.
*/
type Detection struct {
	Run    uint64
	Symbol string
	Class  string
	CTick  uint64
}

/*
Detections serves the hub's "detections" command (ui.Detector): the frames
Detection{run, symbol, class, cTick} asks for.
*/
func (service *Service) Detections(ctx context.Context, w io.Writer, run uint64, symbol, class string, cTick uint64) error {
	return service.Detect(ctx, w, Detection{Run: run, Symbol: symbol, Class: class, CTick: cTick})
}

/*
Detect writes the frames detection asks for to w, batched like Stream.
A failure is written as an error frame and returned; a canceled ctx (a
newer request, a closed client) returns ctx.Err() with no frame.
*/
func (service *Service) Detect(ctx context.Context, w io.Writer, detection Detection) error {
	out := &batch{dst: w, buf: make([]byte, 0, batchBytes)}
	var err error

	switch {
	case detection.Run == 0:
		err = service.detectionRuns(ctx, out)
	case detection.Symbol == "":
		err = service.excursions(ctx, out, detection.Run)
	default:
		err = service.trades(ctx, out, detection)
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if err != nil {
		failure := detectionFrame(data.HindsightError, detection.Run, detection.Symbol, detection.CTick)
		failure.AppendJSON([]byte(err.Error()), data.LayerHindsightError)

		if _, copyErr := io.Copy(out, failure); copyErr != nil {
			err = errors.Join(err, copyErr)
		}
	}

	return errors.Join(err, out.Flush())
}

/*
excursionEpoch is the epoch segment of an excursion key
<detector>/<class>/<symbol>/<epoch>/<file>: the second segment from the end,
since a symbol (BTC/USD) holds slashes itself.
*/
func excursionEpoch(key string) (uint64, error) {
	segments := strings.Split(key, "/")

	if len(segments) < 5 || len(segments[len(segments)-2]) != 19 {
		return 0, errnie.Err(errnie.UnprocessableContent, "[hindsight] excursion key "+key+" has no %019d epoch segment", nil)
	}

	epoch, err := strconv.ParseUint(segments[len(segments)-2], 10, 64)

	if err != nil {
		return 0, errnie.Err(errnie.UnprocessableContent, "[hindsight] excursion key "+key+" has no %019d epoch segment", err)
	}

	return epoch, nil
}

/*
excursionKeys lists every stored excursion key of each class, by epoch,
listing only (nothing is downloaded).
*/
func (service *Service) excursionKeys() (map[uint64]map[string][]string, error) {
	byEpoch := make(map[uint64]map[string][]string)

	for _, class := range data.ExcursionClasses {
		for _, key := range service.client.ListKeys(data.ExcursionOrigin + "/" + class + "/") {
			epoch, err := excursionEpoch(key)

			if err != nil {
				return nil, err
			}

			if byEpoch[epoch] == nil {
				byEpoch[epoch] = make(map[string][]string)
			}

			byEpoch[epoch][class] = append(byEpoch[epoch][class], key)
		}
	}

	return byEpoch, nil
}

/*
detectionRuns writes one HindsightDetectionRun per epoch with a detector
watermark (every epoch detect labelled to completion), newest first.
*/
func (service *Service) detectionRuns(ctx context.Context, w io.Writer) error {
	epochs, err := store.Epochs(service.client, data.Prefix(data.ExcursionOrigin, data.DetectorWatermarkRole, data.PolicyScope))

	if err != nil {
		return err
	}

	byEpoch, err := service.excursionKeys()

	if err != nil {
		return err
	}

	total := 0.0

	for _, epoch := range slices.Backward(epochs) {
		if err := ctx.Err(); err != nil {
			return err
		}

		var watermark *data.DetectorWatermark

		for value, err := range store.Values(service.client, data.DetectorWatermarkPrefix(epoch)) {
			if err != nil {
				return err
			}

			if watermark != nil {
				return errnie.Err(errnie.UnprocessableContent, fmt.Sprintf("[hindsight] epoch %d stores more than one detector watermark", epoch), nil)
			}

			read, err := data.ReadDetectorWatermark(value)

			if err != nil {
				return err
			}

			watermark = &read
		}

		if watermark == nil {
			return errnie.Err(errnie.NotFound, fmt.Sprintf("[hindsight] epoch %d lists a watermark it does not store", epoch), nil)
		}

		vector := []float64{
			float64(watermark.FirstTick), float64(watermark.LastTick),
			float64(watermark.Ticks), float64(watermark.Symbols),
		}

		for _, class := range data.ExcursionClasses {
			count := float64(len(byEpoch[epoch][class]))
			vector = append(vector, count)
			total += count
		}

		run := detectionFrame(data.HindsightDetectionRun, epoch, "", 0)
		data.AppendVector(run, vector, data.DetectionRunKeys...)

		if _, err := io.Copy(w, run); err != nil {
			return fmt.Errorf("writing detection run %d: %w", epoch, err)
		}
	}

	return detectionDone(w, 0, "", map[string]float64{"excursions": total})
}

/*
excursions writes every stored excursion of run, verbatim, class by class.
*/
func (service *Service) excursions(ctx context.Context, w io.Writer, run uint64) error {
	byEpoch, err := service.excursionKeys()

	if err != nil {
		return err
	}

	symbols := make(map[string]bool)
	total := 0.0

	for _, class := range data.ExcursionClasses {
		for value, err := range store.Download(service.client, byEpoch[run][class]) {
			if err != nil {
				return err
			}

			if err := ctx.Err(); err != nil {
				return err
			}

			if _, err := data.DetectExcursionFields(value); err != nil {
				return err
			}

			symbols[value.Scope] = true

			if err := detectionForward(w, value, run, value.Scope); err != nil {
				return err
			}

			total++
		}
	}

	return detectionDone(w, run, "", map[string]float64{"excursions": total, "symbols": float64(len(symbols))})
}

/*
tradePoint is one trade of the symbol, before its seq is known.
*/
type tradePoint struct {
	tick, at uint64
	price    float64
}

/*
trades writes the trades of one stored excursion from its A tick through its
D tick (the stored ticks that exist: D may lie past the epoch's last), with
the trade index detect gave each. The index is anchored at B: the trade of B's
tick at b_at and b_price is trade b_idx, and the ones before and after it
count from there, as detect counted the symbol's trades in key order; C must
then land on c_idx at c_at, or the stream is an error.
*/
func (service *Service) trades(ctx context.Context, w io.Writer, detection Detection) error {
	fields, err := service.excursion(detection)

	if err != nil {
		return err
	}

	aTick, dTick := uint64(fields["a_tick"]), uint64(fields["d_tick"])
	bTick, cTick := uint64(fields["b_tick"]), uint64(fields["c_tick"])
	window := make([]string, 0)

	for _, key := range service.client.ListKeys(data.TickPrefix(detection.Run)) {
		tick, err := tickOfKey(key)

		if err != nil {
			return err
		}

		if tick >= aTick && tick <= dTick {
			window = append(window, key)
		}
	}

	// Trades before B wait for B's index; from B on they are written as
	// their tick arrives.
	pending := make([][]tradePoint, 0)
	pendingAt := make([]uint64, 0)
	var next int64 // the seq of the next trade, once B anchored it
	cChecked := false

	emit := func(tick, timestamp uint64, points []tradePoint, first int64) error {
		seqs := make([]float64, 0, len(points))
		prices := make([]float64, 0, len(points))
		ats := make([]float64, 0, len(points))

		for offset, point := range points {
			seq := first + int64(offset)

			if seq < int64(fields["a_idx"]) || seq > int64(fields["d_idx"]) {
				continue
			}

			if point.tick == cTick && float64(point.at) == fields["c_at"] && point.price == fields["c_price"] {
				if float64(seq) != fields["c_idx"] {
					return errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
						"[hindsight] %s %s excursion at tick %d: C counts as trade %d from B, detect stored c_idx %g",
						detection.Symbol, detection.Class, detection.CTick, seq, fields["c_idx"],
					), nil)
				}

				cChecked = true
			}

			seqs, prices, ats = append(seqs, float64(seq)), append(prices, point.price), append(ats, float64(point.at))
		}

		if len(seqs) == 0 {
			return nil
		}

		out := detectionFrame(data.HindsightTrades, detection.Run, detection.Symbol, tick)
		out.Timestamp = timestamp

		for idx, vector := range [][]float64{seqs, prices, ats} {
			data.AppendVector(out, vector, data.DetectionTradeLayers[idx])
		}

		if _, err := io.Copy(w, out); err != nil {
			return fmt.Errorf("writing trades of tick %d: %w", tick, err)
		}

		return nil
	}

	for value, err := range store.Download(service.client, window) {
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

		index := slices.IndexFunc(ticks, func(tick *broker.SymbolTick) bool { return tick.Symbol == detection.Symbol })

		if index < 0 {
			continue
		}

		points := make([]tradePoint, 0, len(ticks[index].Trades))

		for _, trade := range ticks[index].Trades {
			points = append(points, tradePoint{tick: value.Tick, at: uint64(trade.Timestamp.UnixNano()), price: trade.Price.Float64()})
		}

		if next == 0 && value.Tick == bTick {
			anchor := slices.IndexFunc(points, func(point tradePoint) bool {
				return float64(point.at) == fields["b_at"] && point.price == fields["b_price"]
			})

			if anchor < 0 {
				return errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
					"[hindsight] %s %s excursion at tick %d: tick %d holds no trade at b_at %g and b_price %g",
					detection.Symbol, detection.Class, detection.CTick, bTick, fields["b_at"], fields["b_price"],
				), nil)
			}

			first := int64(fields["b_idx"]) - int64(anchor)

			for idx := len(pending) - 1; idx >= 0; idx-- {
				first -= int64(len(pending[idx]))
			}

			for idx, earlier := range pending {
				if err := emit(earlier[0].tick, pendingAt[idx], earlier, first); err != nil {
					return err
				}

				first += int64(len(earlier))
			}

			pending = nil
			next = first
		}

		if next == 0 {
			pending = append(pending, points)
			pendingAt = append(pendingAt, value.Timestamp)
			continue
		}

		if err := emit(value.Tick, value.Timestamp, points, next); err != nil {
			return err
		}

		next += int64(len(points))
	}

	if next == 0 {
		return errnie.Err(errnie.NotFound, fmt.Sprintf(
			"[hindsight] %s %s excursion at tick %d: B's tick %d is not stored under %s",
			detection.Symbol, detection.Class, detection.CTick, bTick, data.TickPrefix(detection.Run),
		), nil)
	}

	if !cChecked {
		return errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
			"[hindsight] %s %s excursion at tick %d: no trade of C's tick %d is at c_at %g and c_price %g",
			detection.Symbol, detection.Class, detection.CTick, cTick, fields["c_at"], fields["c_price"],
		), nil)
	}

	return detectionDone(w, detection.Run, detection.Symbol, map[string]float64{"ticks": float64(len(window)), "excursions": 1})
}

/*
excursion reads the one stored excursion detection names; none, or more than
one, is an error.
*/
func (service *Service) excursion(detection Detection) (map[string]float64, error) {
	if !slices.Contains(data.ExcursionClasses, detection.Class) {
		return nil, errnie.Err(errnie.Validation, fmt.Sprintf("[hindsight] %q is not an excursion class %v", detection.Class, data.ExcursionClasses), nil)
	}

	var found map[string]float64

	for value, err := range store.Values(service.client, data.Prefix(data.ExcursionOrigin, detection.Class, detection.Symbol, detection.Run)) {
		if err != nil {
			return nil, err
		}

		if value.Tick != detection.CTick {
			continue
		}

		if found != nil {
			return nil, errnie.Err(errnie.UnprocessableContent, fmt.Sprintf(
				"[hindsight] more than one %s %s excursion of run %d ends at tick %d", detection.Symbol, detection.Class, detection.Run, detection.CTick,
			), nil)
		}

		fields, err := data.DetectExcursionFields(value)

		if err != nil {
			return nil, err
		}

		found = fields
	}

	if found == nil {
		return nil, errnie.Err(errnie.NotFound, fmt.Sprintf(
			"[hindsight] no %s %s excursion of run %d ends at tick %d", detection.Symbol, detection.Class, detection.Run, detection.CTick,
		), nil)
	}

	return found, nil
}

/*
tickOfKey is the tick of a stored tick key <prefix>/<epoch>/<%019d tick>.bin.
*/
func tickOfKey(key string) (uint64, error) {
	name := key[strings.LastIndex(key, "/")+1:]
	tick, err := strconv.ParseUint(strings.TrimSuffix(name, ".bin"), 10, 64)

	if err != nil || !strings.HasSuffix(name, ".bin") {
		return 0, errnie.Err(errnie.UnprocessableContent, "[hindsight] tick key "+key+" has no %019d.bin tick", err)
	}

	return tick, nil
}

/*
detectionFrame is an empty detections Value of role for symbol in run: the
Historical runs tab's own origin, so no frame of it is taken for a Hindsight
replay's (data.DetectionsOrigin).
*/
func detectionFrame(role string, run uint64, symbol string, tick uint64) *data.Value {
	value := data.NewValue(data.DetectionsOrigin, role, symbol)
	value.Epoch, value.Tick = run, tick
	value.FormatPrefix()
	return value
}

/*
detectionForward copies a stored excursion to w as a detections frame:
Origin DetectionsOrigin, Role the store origin, Scope the symbol, Epoch the
run; its layers, whose indices name the stored key, untouched.
*/
func detectionForward(w io.Writer, value *data.Value, run uint64, symbol string) error {
	value.Role, value.Origin, value.Scope, value.Epoch = value.Origin, data.DetectionsOrigin, symbol, run

	if _, err := io.Copy(w, value); err != nil {
		return fmt.Errorf("forwarding %s: %w", value.Prefix, err)
	}

	return nil
}

func detectionDone(w io.Writer, run uint64, symbol string, counts map[string]float64) error {
	vector := make([]float64, len(data.HindsightDoneKeys))

	for idx, key := range data.HindsightDoneKeys {
		vector[idx] = counts[key]
	}

	closing := detectionFrame(data.HindsightDone, run, symbol, 0)
	data.AppendVector(closing, vector, data.HindsightDoneKeys...)

	if _, err := io.Copy(w, closing); err != nil {
		return fmt.Errorf("writing done: %w", err)
	}

	return nil
}
