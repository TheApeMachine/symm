package hindsight

import (
	"context"
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
)

/*
includedRetrace is the deepest counter-move inside prices, as a fraction of
the move from the first print to the last. That is the dip a committed
detection kept inside one swing.
*/
func includedRetrace(prices []float64) float64 {
	if len(prices) < 2 || prices[0] <= 0 {
		return 0
	}

	start, end := prices[0], prices[len(prices)-1]
	swing := math.Abs(end-start) / start

	if swing <= 1e-9 {
		return 0
	}

	up := end >= start
	extreme := start
	worst := 0.0

	for _, price := range prices {
		var counter float64

		if up {
			if price > extreme {
				extreme = price
			}

			counter = (extreme - price) / start
		} else {
			if price < extreme {
				extreme = price
			}

			counter = (price - extreme) / start
		}

		if fraction := counter / swing; fraction > worst {
			worst = fraction
		}
	}

	if worst > 0.9 {
		return 0.9
	}

	return worst
}

func excursionComplete(value *data.Value) error {
	if value == nil {
		return errnie.Err(errnie.Validation, "[hindsight] draft excursion is empty", nil)
	}

	metrics := data.Metrics(value)

	for _, key := range data.ExcursionKeys {
		if _, ok := metrics[key]; !ok {
			return errnie.Err(errnie.UnprocessableContent, "hindsight: excursion "+value.Prefix+" has no "+key, nil)
		}
	}

	return nil
}

func (service *Service) newDetector() *strategy.Detector {
	price := broker.NewPrice(service.ctx, nil, nil, nil, nil)
	cash := 200.0

	if system.Cfg != nil && system.Cfg.Market != nil && system.Cfg.Market.Balance > 0 {
		cash = float64(system.Cfg.Market.Balance)
	}

	price.SetReferenceCash(decimal.NewFromFloat64(cash))
	price.SetDefaultFee(kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.8)})

	return strategy.NewDetector(service.ctx, price)
}

func (service *Service) loadRetrace(run uint64) float64 {
	for value, err := range store.Values(service.client, data.Prefix(data.DraftOrigin, "tolerance", data.PolicyScope, run)) {
		if err != nil || value == nil {
			return 0
		}

		return data.Metrics(value)["retrace"]
	}

	return 0
}

func (service *Service) saveRetrace(run uint64, ratio float64) error {
	if ratio <= 0 {
		return nil
	}

	value := data.NewValue(data.DraftOrigin, "tolerance", data.PolicyScope)
	value.Epoch = run
	value.Tick = 1
	value.FormatPrefix()
	data.AppendVector(value, []float64{ratio}, "retrace")

	_, err := io.Copy(service.client, value)

	return err
}

/*
clearDrafts removes the run's working predictions. The tolerance object lives
under a different role and stays.
*/
func (service *Service) clearDrafts(run uint64) error {
	for _, class := range data.ExcursionClasses {
		for _, key := range service.client.ListKeys(data.DraftOrigin + "/" + class + "/") {
			got, err := excursionEpoch(key)

			if err != nil || got != run {
				continue
			}

			if err := service.client.DeletePrefix(key); err != nil {
				return err
			}
		}
	}

	return nil
}

func (service *Service) writeDraft(value *data.Value) error {
	fields, err := data.DetectExcursionFields(value)

	if err != nil {
		return err
	}

	if fields["a_tick"] < 1 {
		fields["a_tick"] = fields["b_tick"]
		fields["a_idx"] = fields["b_idx"]
	}

	vector := []float64{
		fields["a_idx"], fields["a_tick"], fields["b_idx"], fields["b_tick"],
		fields["c_idx"], fields["c_tick"], fields["d_idx"], fields["d_tick"],
		fields["b_price"], fields["c_price"], fields["b_price"], fields["c_price"],
		0, 0,
		fields["b_idx"], fields["b_tick"], fields["c_idx"], fields["c_tick"],
		fields["b_price"], fields["c_price"], 0, 0,
	}

	draft := data.NewValue(data.DraftOrigin, value.Role, value.Scope)
	draft.Epoch = value.Epoch
	draft.Tick = value.Tick
	draft.FormatPrefix()
	data.AppendVector(draft, vector, data.ExcursionKeys...)

	_, err = io.Copy(service.client, draft)

	return err
}

/*
rescan walks the run once per symbol. Each symbol keeps its own detector for
the whole run, so a trade of another symbol cannot wipe its swing.
*/
func (service *Service) rescan(ctx context.Context, run uint64, retrace float64) (int, error) {
	if err := service.clearDrafts(run); err != nil {
		return 0, err
	}

	detectors := map[string]*strategy.Detector{}
	written := 0
	seen := 0

	errnie.Info(fmt.Sprintf("[hindsight] scanning run %d one symbol at a time", run))

	for frame, err := range store.Values(service.client, data.TickPrefix(run)) {
		if err != nil {
			return written, err
		}

		if err := ctx.Err(); err != nil {
			return written, err
		}

		seen++

		if seen%20000 == 0 {
			errnie.Info(fmt.Sprintf("[hindsight] scan of run %d has read %d ticks", run, seen))
		}

		ticks, err := broker.ReadTick(frame)

		if err != nil {
			return written, err
		}

		for _, tick := range ticks {
			if tick == nil || tick.Symbol == "" {
				continue
			}

			detector := detectors[tick.Symbol]

			if detector == nil {
				detector = service.newDetector()
				detector.SetRetrace(retrace)
				detectors[tick.Symbol] = detector
			}

			for _, value := range detector.Replay(frame, tick) {
				if err := service.writeDraft(value); err != nil {
					return written, err
				}

				written++
			}
		}
	}

	for _, detector := range detectors {
		for _, value := range detector.Finish() {
			if err := service.writeDraft(value); err != nil {
				return written, err
			}

			written++
		}
	}

	errnie.Info(fmt.Sprintf("[hindsight] scan of run %d wrote %d draft detections across %d symbols", run, written, len(detectors)))

	return written, nil
}

func (service *Service) symbolPrices(ctx context.Context, run uint64, symbol string, from, to uint64) ([]float64, error) {
	prices := make([]float64, 0)

	for frame, err := range store.Values(service.client, data.TickPrefix(run)) {
		if err != nil {
			return nil, err
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if frame.Tick < from || frame.Tick > to {
			continue
		}

		ticks, err := broker.ReadTick(frame)

		if err != nil {
			return nil, err
		}

		for _, tick := range ticks {
			if tick == nil || tick.Symbol != symbol {
				continue
			}

			for _, trade := range tick.Trades {
				price := trade.Price.Float64()

				if price > 0 {
					prices = append(prices, price)
				}
			}
		}
	}

	return prices, nil
}

func (service *Service) beginScan(ctx context.Context, run uint64) (int, error) {
	if !service.scanMu.TryLock() {
		return 0, errnie.Err(errnie.NotAcceptable, "[hindsight] a scan is already running", nil)
	}

	defer service.scanMu.Unlock()

	return service.rescan(ctx, run, service.loadRetrace(run))
}

func (service *Service) commit(ctx context.Context, run uint64, symbol, class string, marks [4]uint64) (int, error) {
	if !service.scanMu.TryLock() {
		return 0, errnie.Err(errnie.NotAcceptable, "[hindsight] a scan is already running", nil)
	}

	defer service.scanMu.Unlock()

	prices, err := service.symbolPrices(ctx, run, symbol, marks[1], marks[2])

	if err != nil {
		return 0, err
	}

	retrace := math.Max(service.loadRetrace(run), includedRetrace(prices))

	if _, err := service.storeMarks(ctx, data.DetectionsOrigin, run, symbol, class, marks); err != nil {
		return 0, err
	}

	if err := service.saveRetrace(run, retrace); err != nil {
		return 0, err
	}

	return service.rescan(ctx, run, retrace)
}

func (service *Service) commitAll(ctx context.Context, run uint64) (int, error) {
	if !service.scanMu.TryLock() {
		return 0, errnie.Err(errnie.NotAcceptable, "[hindsight] a scan is already running", nil)
	}

	defer service.scanMu.Unlock()

	written := 0

	for _, class := range data.ExcursionClasses {
		if err := ctx.Err(); err != nil {
			return written, err
		}

		for value, err := range store.Values(service.client, data.DraftOrigin+"/"+class+"/") {
			if err != nil {
				return written, err
			}

			if value.Epoch != run {
				continue
			}

			value.Origin = data.DetectionsOrigin
			value.FormatPrefix()

			if _, err := io.Copy(service.client, value); err != nil {
				return written, err
			}

			written++
		}
	}

	return written, nil
}

func (service *Service) scanRoute(ctx fiber.Ctx) error {
	run, err := runOf(ctx.Body())

	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	written, err := service.beginScan(ctx.Context(), run)

	if err != nil {
		return fiber.NewError(fiber.StatusConflict, err.Error())
	}

	return ctx.JSON(struct {
		Drafts int `json:"drafts"`
	}{Drafts: written})
}

func (service *Service) commitRoute(ctx fiber.Ctx) error {
	var request annotationRequest

	if err := sonic.Unmarshal(ctx.Body(), &request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "commit body is not JSON")
	}

	run, err := strconv.ParseUint(request.Run, 10, 64)

	if err != nil || request.Symbol == "" {
		return fiber.NewError(fiber.StatusBadRequest, "commit needs a run and a symbol")
	}

	marks := [4]uint64{}

	for idx, raw := range []string{request.A, request.B, request.C, request.D} {
		marks[idx], err = strconv.ParseUint(raw, 10, 64)

		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "commit ticks must be decimal strings")
		}
	}

	written, err := service.commit(ctx.Context(), run, request.Symbol, request.Class, marks)

	if err != nil {
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	}

	return ctx.JSON(struct {
		Drafts int `json:"drafts"`
	}{Drafts: written})
}

func (service *Service) commitAllRoute(ctx fiber.Ctx) error {
	run, err := runOf(ctx.Body())

	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	written, err := service.commitAll(ctx.Context(), run)

	if err != nil {
		return fiber.NewError(fiber.StatusConflict, err.Error())
	}

	return ctx.JSON(struct {
		Committed int `json:"committed"`
	}{Committed: written})
}

func runOf(body []byte) (uint64, error) {
	var request struct {
		Run string `json:"run"`
	}

	if err := sonic.Unmarshal(body, &request); err != nil {
		return 0, errnie.Err(errnie.Validation, "body is not JSON", err)
	}

	run, err := strconv.ParseUint(request.Run, 10, 64)

	if err != nil || run == 0 {
		return 0, errnie.Err(errnie.Validation, "run is required", err)
	}

	return run, nil
}
