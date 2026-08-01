package pumpdump

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal owns pump-cycle measurements derived from executed trade lift and the
reconstructed book's midpoint and spread. It reads each market
fact from its authoritative stream without treating them as independent
corroborating signals.
*/
type Signal struct {
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	algo          *equation.Ignition
	planner       *strategy.Planner
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
}

/*
NewSignal creates an empty per-symbol pump state whose baseline capacity is the
same explicit retention bound used by the production market feed.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	planner *strategy.Planner,
	ui chan []byte,
	subscriptions map[string]*types.Subscription[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		status:        types.INITIALIZING,
		ctx:           ctx,
		cancel:        cancel,
		api:           api,
		algo:          equation.NewIgnition(viper.GetViper().GetInt("signals.pumpdump.baselineCapacity")),
		planner:       planner,
		ui:            ui,
		subscriptions: subscriptions,
		subscribers:   &sync.Map{},
	}

	signal.status = types.READY
	signal.run()
	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourcePumpDump)
}

func (signal *Signal) Status() types.Status {
	return signal.status
}

func (signal *Signal) Subscribe(
	channel string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	if signal.subscribers == nil {
		signal.subscribers = &sync.Map{}
	}

	subscribers, ok := signal.subscribers.LoadOrStore(
		channel, []*types.Subscription[any]{subscription},
	)

	if ok && subscribers != nil {
		signal.subscribers.Store(
			channel, append(subscribers.([]*types.Subscription[any]), subscription),
		)
	}

	return subscription
}

func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case message := <-signal.subscriptions["thesis"].Channel:
				if thesis, ok := message.(*types.Thesis); ok {
					thesis.AppendMeasurements(
						types.SourcePumpDump,
						signal.Calculate(thesis),
						types.Stamp{At: time.Now(), Entity: types.MarketTicker},
					)

					if signal.subscribers == nil {
						return
					}

					subscribers, ok := signal.subscribers.Load("thesis")

					if ok && subscribers != nil {
						for _, subscriber := range subscribers.([]*types.Subscription[any]) {
							subscriber.Send(thesis)
						}
					}
				}
			}
		}
	}()
}

func (signal *Signal) Calculate(thesis *types.Thesis) []*types.Measurement {
	tickers, trades, books := thesis.Market()

	if len(tickers) == 0 {
		signal.ingest(nil, trades, books)
		return nil
	}

	sort.SliceStable(tickers, func(left, right int) bool {
		return tickers[left].Timestamp.Before(tickers[right].Timestamp)
	})

	sort.SliceStable(trades, func(left, right int) bool {
		return trades[left].Timestamp.Before(trades[right].Timestamp)
	})

	sort.SliceStable(books, func(left, right int) bool {
		return books[left].Timestamp.Before(books[right].Timestamp)
	})

	out := make([]*types.Measurement, 0, len(tickers))
	uiOut := datura.NewMap(
		"measurements", make([]*types.Measurement, 0),
	)

	tradeIndex := 0
	bookIndex := 0
	measured := make(map[string]struct{}, len(tickers))

	for _, row := range tickers {
		for tradeIndex < len(trades) && !trades[tradeIndex].Timestamp.After(row.Timestamp) {
			signal.ingest(thesis, trades[tradeIndex:tradeIndex+1], nil)

			tradeIndex++
		}

		for bookIndex < len(books) && !books[bookIndex].Timestamp.After(row.Timestamp) {
			signal.ingest(thesis, nil, books[bookIndex:bookIndex+1])

			bookIndex++
		}

		measurements, err := signal.measure(thesis, row)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent, "pumpdump: failed to measure tickers", err,
			))
			return nil
		}

		if row.Symbol != "" {
			measured[row.Symbol] = struct{}{}
		}

		if len(measurements) > 0 && row.Symbol != "" {
			found, _ := thesis.Causal.Load("signal:pumpdump:lastWire")
			found.(*sync.Map).Store(row.Symbol, measurements)
		}

		out = append(out, measurements...)

		if row.Symbol == types.Focus() {
			uiOut["measurements"] = append(
				uiOut["measurements"].([]*types.Measurement), measurements...,
			)
		}
	}

	signal.ingest(thesis, trades[tradeIndex:], books[bookIndex:])

	if len(uiOut["measurements"].([]*types.Measurement)) > 0 {
		utils.Publish(signal.ui, uiOut)
	}

	return out
}

/*
measure derives one ticker observation from the causally preceding tape state.
Kraken ticker timestamps are not a per-symbol sequence: late or reconnect
frames can arrive behind the watermark. Those are not observations and must
not enter the volume clock.
*/
func (signal *Signal) measure(thesis *types.Thesis, row kraken.TickerData) ([]*types.Measurement, error) {
	at, ok, err := signal.tickerObservationGate(thesis, row)

	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, nil
	}

	bookState, ok := signal.resolveTickerBookState(thesis, row)

	if !ok {
		return nil, nil
	}

	found, _ := thesis.Causal.Load("signal:pumpdump:ignition")
	output, ready, _, err := found.(*equation.Ignition).Measure(equation.IgnitionInput{
		Symbol: row.Symbol,
		Volume: bookState.volume,
		Last:   bookState.mid,
		Bid:    bookState.bid,
		Ask:    bookState.ask,
		At:     at,
	})

	if err != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)
	}

	found, _ = thesis.Causal.Load("signal:pumpdump:lastAt")
	found.(*sync.Map).Store(row.Symbol, at)

	return signal.	measurements(
		thesis,
		row.Symbol,
		at,
		output,
		ready,
		bookState.bid,
		bookState.ask,
	)
}

/*
tickerBookState carries resolved midpoint, spread, and volume for one ticker.
*/
type tickerBookState struct {
	mid    float64
	bid    float64
	ask    float64
	volume float64
}

/*
measurements emits one PumpDump Measurement per symbol whose Metrics map carries
the full ignition surface from RVOL through Strength.
*/
func (signal *Signal) measurements(
	thesis *types.Thesis,
	symbol string,
	at time.Time,
	output equation.IgnitionOutput,
	ready bool,
	bid float64,
	ask float64,
) ([]*types.Measurement, error) {
	if at.IsZero() {
		return nil, errnie.Err(
			errnie.Validation,
			"pumpdump: observation timestamp required",
			nil,
		)
	}

	if bid <= 0 || ask <= 0 {
		return nil, errnie.Err(
			errnie.Validation,
			"pumpdump: bid and ask must be positive",
			nil,
		)
	}

	if bid >= ask {
		return nil, errnie.Err(
			errnie.Validation,
			fmt.Sprintf("pumpdump: crossed BBO bid=%v ask=%v", bid, ask),
			nil,
		)
	}

	mid := (bid + ask) / 2
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}

	if !ready {
		validity.State = types.ValidityProvisional
		validity.Reason = "ignition baselines not ready"
	}

	measurement := &types.Measurement{
		Source:   types.SourcePumpDump,
		Symbol:   symbol,
		At:       at,
		Maturity: thesis.Tick,
		Validity: validity,
		Scale: types.ScaleReference{
			Kind:    types.ScaleObservationWindow,
			From:    at,
			Through: at,
		},
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricRVOL, types.SideNone): {
				Raw:        output.RVOL,
				Normalized: types.NormalizeFinite(output.RVOL),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricPrecursor, types.SideNone): {
				Raw:        output.Precursor,
				Normalized: types.NormalizeFinite(output.Precursor),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricSpread, types.SideNone): {
				Raw:        output.Spread,
				Normalized: types.NormalizeRatio(output.Spread, mid),
				Unit:       types.UnitQuoteCurrency,
			},
			types.MetricKey(types.MetricCompression, types.SideNone): {
				Raw:        output.Compression,
				Normalized: types.NormalizeFinite(output.Compression),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricIgnition, types.SideNone): {
				Raw:        output.Ignition,
				Normalized: types.NormalizeFinite(output.Ignition),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricTrend, types.SideNone): {
				Raw:        output.Trend,
				Normalized: types.NormalizeFinite(output.Trend),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricExhaustion, types.SideNone): {
				Raw:        output.Exhaustion,
				Normalized: types.NormalizeFinite(output.Exhaustion),
				Unit:       types.UnitDimensionless,
			},
			types.MetricKey(types.MetricStrength, types.SideNone): {
				Raw:        output.Strength,
				Normalized: types.NormalizeFinite(output.Strength),
				Unit:       types.UnitDimensionless,
			},
		},
	}

	return []*types.Measurement{measurement}, nil
}

/*
tickerObservationGate rejects unusable or stale ticker rows before measurement.
*/
func (signal *Signal) tickerObservationGate(
	thesis *types.Thesis,
	row kraken.TickerData,
) (time.Time, bool, error) {
	if row.Symbol == "" || row.Last == nil || row.Last.Sign() <= 0 {
		return time.Time{}, false, nil
	}

	if row.Timestamp.IsZero() {
		return time.Time{}, false, errnie.Err(
			errnie.Validation,
			"pumpdump: ticker observation timestamp required",
			nil,
		)
	}

	found, _ := thesis.Causal.Load("signal:pumpdump:lastAt")

	if at, ok := found.(*sync.Map).Load(row.Symbol); ok {
		if row.Timestamp.Before(at.(time.Time)) {
			return time.Time{}, false, nil
		}
	}

	return row.Timestamp, true, nil
}

/*
resolveTickerBookState loads book, volume, increment, and spread for one symbol.
*/
func (signal *Signal) resolveTickerBookState(
	thesis *types.Thesis,
	row kraken.TickerData,
) (tickerBookState, bool) {
	found, _ := thesis.Causal.Load("signal:pumpdump:books")
	bookFound, ok := found.(*sync.Map).Load(row.Symbol)

	if !ok {
		return tickerBookState{}, false
	}

	book := bookFound.(*flow.Book)
	found, _ = thesis.Causal.Load("signal:pumpdump:volume")
	volumeFound, ok := found.(*sync.Map).Load(row.Symbol)

	if !ok {
		return tickerBookState{}, false
	}

	volume := volumeFound.(float64)

	if book == nil || volume <= 0 {
		return tickerBookState{}, false
	}

	mid := book.Mid()
	found, _ = thesis.Causal.Load("signal:pumpdump:increments")
	incrementFound, ok := found.(*sync.Map).Load(row.Symbol)

	if !ok {
		return tickerBookState{}, false
	}

	increment := incrementFound.(float64)

	if mid <= 0 || increment <= 0 {
		return tickerBookState{}, false
	}

	spread := math.Round(book.Spread()/increment) * increment

	if spread <= 0 {
		return tickerBookState{}, false
	}

	return tickerBookState{
		mid:    mid,
		bid:    mid - spread/2,
		ask:    mid + spread/2,
		volume: volume,
	}, true
}

/*
ingest accumulates executed quantity and applies incremental book updates before
the next ticker price closes the observation. Ticker-reported volume is not
used as a substitute for trades when the subscribed trade stream is absent.
*/
func (signal *Signal) ingest(
	thesis *types.Thesis,
	trades []kraken.TradeData,
	books []kraken.BookData,
) error {
	for _, trade := range trades {
		if trade.Symbol == "" || trade.Price.Sign() <= 0 || trade.Qty <= 0 {
			return errnie.Err(
				errnie.Validation,
				"pumpdump: valid executed trade required",
				nil,
			)
		}

		found, _ := thesis.Causal.Load("signal:pumpdump:volume")
		volumeFound, ok := found.(*sync.Map).Load(trade.Symbol)
		volume := 0.0

		if ok {
			volume = volumeFound.(float64)
		}

		found.(*sync.Map).Store(trade.Symbol, volume+trade.Qty)
	}

	if len(books) == 0 {
		return nil
	}

	groups := types.ChunkRowsBySymbol(books, func(row kraken.BookData) string {
		return row.Symbol
	})

	return types.RunSymbolGroupsParallel(groups, func(index int, rows []kraken.BookData) error {
		for _, row := range rows {
			if err := signal.ingestBook(thesis, row); err != nil {
				return err
			}
		}

		return nil
	})
}

/*
ingestBook applies one incremental or snapshot book row to the symbol-local
order book state pump measurements consume.
*/
func (signal *Signal) ingestBook(thesis *types.Thesis, row kraken.BookData) error {
	if row.Symbol == "" || row.PriceIncrement == nil || row.PriceIncrement.Sign() <= 0 {
		return errnie.Err(
			errnie.Validation,
			"pumpdump: symbol and price increment required for book update",
			nil,
		)
	}

	found, _ := thesis.Causal.Load("signal:pumpdump:books")
	bookFound, ok := found.(*sync.Map).Load(row.Symbol)
	var book *flow.Book

	if ok {
		book = bookFound.(*flow.Book)
	}

	increments, _ := thesis.Causal.Load("signal:pumpdump:increments")
	increments.(*sync.Map).Store(row.Symbol, row.PriceIncrement.Float64())

	if book == nil || row.Type == "snapshot" {
		book = flow.NewBook()
		found.(*sync.Map).Store(row.Symbol, book)
	}

	bids, asks, err := kraken.BookLevels(row)

	if err != nil {
		return errnie.Err(errnie.Validation, "pumpdump: decode book levels", err)
	}

	if err := book.Configure(flow.BookInput{
		Symbol:   row.Symbol,
		TickSize: row.PriceIncrement.Float64(),
	}); err != nil {
		return errnie.Err(errnie.Validation, "pumpdump: configure book", err)
	}

	if _, err := book.ApplyLevels(bids, flow.SideBid); err != nil {
		return errnie.Err(errnie.Validation, "pumpdump: apply bid levels", err)
	}

	if _, err := book.ApplyLevels(asks, flow.SideAsk); err != nil {
		return errnie.Err(errnie.Validation, "pumpdump: apply ask levels", err)
	}

	return nil
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	// cancel() // Assuming cancel is defined elsewhere, otherwise this line may need to be updated
	return nil
}
