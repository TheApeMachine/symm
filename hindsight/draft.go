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

func scanningDetector() *strategy.Detector {
	price := broker.NewPrice(context.Background(), nil, nil, nil, nil)
	cash := 200.0
	fee := 0.8

	if system.Cfg != nil && system.Cfg.Market != nil {
		if system.Cfg.Market.Balance > 0 {
			cash = float64(system.Cfg.Market.Balance)
		}

		if system.Cfg.Market.TakerFee > 0 {
			fee = system.Cfg.Market.TakerFee
		}
	}

	price.SetReferenceCash(decimal.NewFromFloat64(cash))
	price.SetDefaultFee(kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(fee)})

	return strategy.NewDetector(context.Background(), price)
}

/*
LearnedRetrace is the pullback the last commit taught the detector. The next
detect run reads this one object and starts from it.
*/
func LearnedRetrace(client *store.Client) float64 {
	for value, err := range store.Values(client, data.Prefix(data.DraftOrigin, "params", "learned", 1)) {
		if err != nil || value == nil {
			return 0
		}

		return data.Metrics(value)["retrace"]
	}

	return 0
}

func saveLearnedRetrace(client *store.Client, ratio float64) error {
	if ratio <= 0 {
		return nil
	}

	value := data.NewValue(data.DraftOrigin, "params", "learned")
	value.Epoch = 1
	value.Tick = 1
	value.FormatPrefix()
	data.AppendVector(value, []float64{ratio}, "retrace")

	_, err := io.Copy(client, value)

	return err
}

/*
clearDrafts removes the run's working predictions. The tolerance object lives
under a different role and stays.
*/
func (service *Service) clearDrafts(run uint64) error {
	return clearDraftsForRun(service.client, run)
}

func (service *Service) loadRetrace() float64 {
	return LearnedRetrace(service.client)
}

func (service *Service) saveRetrace(ratio float64) error {
	return saveLearnedRetrace(service.client, ratio)
}

func (service *Service) writeDraft(value *data.Value) error {
	return writeDraftTo(service.client, value)
}

/*
StoreDrafts replaces run's working predictions with drafts. detect uses this
after it has stitched the run and scanned each symbol.
*/
func StoreDrafts(client *store.Client, run uint64, drafts []*data.Value) error {
	if err := clearDraftsForRun(client, run); err != nil {
		return err
	}

	for _, value := range drafts {
		if err := writeDraftTo(client, value); err != nil {
			return err
		}
	}

	return nil
}

func writeDraftTo(client *store.Client, value *data.Value) error {
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

	_, err = io.Copy(client, draft)

	return err
}

func clearDraftsForSymbols(client *store.Client, run uint64, symbols map[string]struct{}) error {
	if len(symbols) == 0 {
		return clearDraftsForRun(client, run)
	}

	for symbol := range symbols {
		for _, class := range data.ExcursionClasses {
			for _, key := range client.ListKeys(data.Prefix(data.DraftOrigin, class, symbol, run)) {
				if err := client.DeletePrefix(key); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func clearDraftsForRun(client *store.Client, run uint64) error {
	for _, class := range data.ExcursionClasses {
		for _, key := range client.ListKeys(data.DraftOrigin + "/" + class + "/") {
			got, err := excursionEpoch(key)

			if err != nil || got != run {
				continue
			}

			if err := client.DeletePrefix(key); err != nil {
				return err
			}
		}
	}

	return nil
}

func (service *Service) replaceDrafts(run uint64, symbols map[string]struct{}, drafts []*data.Value) error {
	if err := clearDraftsForSymbols(service.client, run, symbols); err != nil {
		return err
	}

	for _, value := range drafts {
		if err := service.writeDraft(value); err != nil {
			return err
		}
	}

	return nil
}

/*
rescan reruns every symbol tape already in memory and replaces the run's drafts.
*/
func (service *Service) rescan(ctx context.Context, run uint64, retrace float64) ([]*data.Value, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	loaded, err := service.tape(ctx, run)

	if err != nil {
		return nil, err
	}

	drafts := scanSymbols(loaded, retrace, nil)

	if err := service.replaceDrafts(run, nil, drafts); err != nil {
		return nil, err
	}

	errnie.Info(fmt.Sprintf("[hindsight] scan of run %d wrote %d draft detections across %d symbols", run, len(drafts), len(loaded.bySymbol)))

	return drafts, nil
}

/*
rescanSymbols replays only the given symbols. commit uses this when learned
retrace did not change, because only the edited symbol's drafts need refresh.
*/
func (service *Service) rescanSymbols(ctx context.Context, run uint64, retrace float64, symbols []string) ([]*data.Value, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	loaded, err := service.tape(ctx, run)

	if err != nil {
		return nil, err
	}

	want := make(map[string]struct{}, len(symbols))

	for _, symbol := range symbols {
		want[symbol] = struct{}{}
	}

	drafts := make([]*data.Value, 0)

	for symbol := range want {
		drafts = append(drafts, scanSymbol(loaded, symbol, retrace)...)
	}

	if err := service.replaceDrafts(run, want, drafts); err != nil {
		return nil, err
	}

	errnie.Info(fmt.Sprintf("[hindsight] rescan of run %d refreshed %d symbols (%d drafts)", run, len(want), len(drafts)))

	return drafts, nil
}

func widestDraft(drafts []*data.Value, symbol, class string) (a, b, c, d uint64, ok bool) {
	best := 0.0

	for _, value := range drafts {
		if value == nil || value.Scope != symbol || value.Role != class {
			continue
		}

		fields, err := data.DetectExcursionFields(value)

		if err != nil || fields["b_price"] <= 0 {
			continue
		}

		span := math.Abs(fields["c_price"] - fields["b_price"])

		if ok && span <= best {
			continue
		}

		if fields["a_tick"] < 1 {
			fields["a_tick"] = fields["b_tick"]
		}

		best = span
		a = uint64(fields["a_tick"])
		b = uint64(fields["b_tick"])
		c = uint64(fields["c_tick"])
		d = uint64(fields["d_tick"])
		ok = true
	}

	return a, b, c, d, ok
}

func (service *Service) beginScan(ctx context.Context, run uint64) (int, error) {
	if !service.scanMu.TryLock() {
		return 0, errnie.Err(errnie.NotAcceptable, "[hindsight] a scan is already running", nil)
	}

	defer service.scanMu.Unlock()

	retrace := service.loadRetrace()
	drafts, err := service.rescan(ctx, run, retrace)

	if err != nil {
		return 0, err
	}

	if err := WriteScanStamp(service.client, run, retrace, len(drafts)); err != nil {
		return 0, err
	}

	return len(drafts), nil
}

func (service *Service) commit(ctx context.Context, run uint64, symbol, class string, marks [4]uint64) (int, [4]uint64, bool, error) {
	none := [4]uint64{}

	if !service.scanMu.TryLock() {
		return 0, none, false, errnie.Err(errnie.NotAcceptable, "[hindsight] a scan is already running", nil)
	}

	defer service.scanMu.Unlock()

	loaded, err := service.tape(ctx, run)

	if err != nil {
		return 0, none, false, err
	}

	priorRetrace := service.loadRetrace()
	retrace := math.Max(priorRetrace, includedRetrace(loaded.prices(symbol, marks[1], marks[2])))
	retraceChanged := retrace > priorRetrace+1e-12

	if _, err := service.writeMarks(data.DetectionsOrigin, run, symbol, class, marks, markedTrades(loaded, symbol, marks[0], marks[3])); err != nil {
		return 0, none, false, err
	}

	if err := service.saveRetrace(retrace); err != nil {
		return 0, none, false, err
	}

	var drafts []*data.Value

	if retraceChanged {
		drafts, err = service.rescan(ctx, run, retrace)
	} else {
		drafts, err = service.rescanSymbols(ctx, run, retrace, []string{symbol})
	}

	if err != nil {
		return 0, none, false, err
	}

	if err := WriteScanStamp(service.client, run, retrace, len(drafts)); err != nil {
		return 0, none, false, err
	}

	a, b, c, d, ok := widestDraft(drafts, symbol, class)

	return len(drafts), [4]uint64{a, b, c, d}, ok, nil
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

	written, next, ok, err := service.commit(ctx.Context(), run, request.Symbol, request.Class, marks)

	if err != nil {
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	}

	response := struct {
		Drafts int `json:"drafts"`
		Marker *struct {
			A string `json:"a"`
			B string `json:"b"`
			C string `json:"c"`
			D string `json:"d"`
		} `json:"marker,omitempty"`
	}{Drafts: written}

	if ok {
		response.Marker = &struct {
			A string `json:"a"`
			B string `json:"b"`
			C string `json:"c"`
			D string `json:"d"`
		}{
			A: strconv.FormatUint(next[0], 10),
			B: strconv.FormatUint(next[1], 10),
			C: strconv.FormatUint(next[2], 10),
			D: strconv.FormatUint(next[3], 10),
		}
	}

	return ctx.JSON(response)
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
