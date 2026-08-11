package pumpdump

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/viper"
	spotbook "github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
	"golang.org/x/sync/errgroup"
)

/*
Signal owns pump-cycle measurements derived from executed trade lift and the
reconstructed book's midpoint and spread. It reads each market
fact from its authoritative stream without treating them as independent
corroborating signals.
*/
type Signal struct {
	status     atomic.Value
	ctx        context.Context
	cancel     context.CancelFunc
	api        *websocket.API
	algo       *equation.Ignition
	algorithms *sync.Map
	capacity   int
	ui         chan []byte
	lastTrade  *sync.Map
}

type tradeCursor struct {
	at  time.Time
	ids map[int64]struct{}
}

/*
NewSignal creates an empty per-symbol pump state whose baseline capacity is the
same explicit retention bound used by the production market feed.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	capacity := viper.GetViper().GetInt("signals.pumpdump.baselineCapacity")
	signal := &Signal{
		ctx:        ctx,
		cancel:     cancel,
		api:        api,
		algorithms: &sync.Map{},
		capacity:   capacity,
		ui:         ui,
		lastTrade:  &sync.Map{},
	}

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourcePumpDump)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourcePumpDump
}

func (signal *Signal) Status() types.Status {
	return signal.status.Load().(types.Status)
}

/*
Measure produces the Measurements for the pumpdump signal.
*/
func (signal *Signal) Measure(
	thesis *types.Thesis,
) ([]*types.Measurement, bool) {
	measurements := make([]*types.Measurement, 0)
	out := make([]*types.Measurement, 0)

	trades := thesis.MarketTrades(types.SourcePumpDump)

	if len(trades) == 0 || signal.api == nil {
		return measurements, false
	}

	tradeBatches := make(map[string][]kraken.TradeData)
	symbols := make([]string, 0)
	results := &sync.Map{}
	errorsBySymbol := &sync.Map{}

	for _, trade := range trades {
		if !validTrade(trade) {
			continue
		}

		if _, exists := tradeBatches[trade.Symbol]; !exists {
			symbols = append(symbols, trade.Symbol)
		}

		tradeBatches[trade.Symbol] = append(tradeBatches[trade.Symbol], trade)
	}

	sort.Strings(symbols)

	group, _ := errgroup.WithContext(signal.ctx)

	if signal.algorithms == nil {
		signal.algorithms = &sync.Map{}
	}

	if signal.algo != nil && len(symbols) > 0 {
		signal.algorithms.LoadOrStore(symbols[0], signal.algo)
		signal.algo = nil
	}

	for _, symbol := range symbols {
		symbolTrades := tradeBatches[symbol]
		sort.SliceStable(symbolTrades, func(leftIndex, rightIndex int) bool {
			left := symbolTrades[leftIndex]
			right := symbolTrades[rightIndex]

			if left.Timestamp.Equal(right.Timestamp) {
				return left.TradeID < right.TradeID
			}

			return left.Timestamp.Before(right.Timestamp)
		})

		group.Go(func() error {
			symbolMeasurements := make([]*types.Measurement, 0)
			algo := signal.algorithm(symbol)

			for _, trade := range symbolTrades {
				if signal.seenTrade(trade) {
					continue
				}

				var askPrice, bidPrice float64
				var askAt, bidAt time.Time
				bookReady := false
				signal.api.Book(trade.Symbol, func(book *spotbook.Book) {
					ask, bid := book.BestAsk(), book.BestBid()

					if ask == nil || bid == nil || ask.Price == nil || bid.Price == nil {
						return
					}

					askPrice = ask.Price.Float64()
					bidPrice = bid.Price.Float64()
					askAt = ask.Timestamp
					bidAt = bid.Timestamp
					bookReady = true
				})

				if !bookReady {
					continue
				}

				if askAt.After(trade.Timestamp) || bidAt.After(trade.Timestamp) {
					continue
				}

				mid := (askPrice + bidPrice) / 2

				if bidPrice <= 0 || askPrice <= bidPrice {
					continue
				}

				output, ready, maturity, err := algo.Measure(equation.IgnitionInput{
					At:     trade.Timestamp,
					Symbol: trade.Symbol,
					Last:   trade.Price.Float64(),
					Volume: trade.Qty,
					Ask:    askPrice,
					Bid:    bidPrice,
				})

				if err != nil {
					errorsBySymbol.Store(symbol, err)
					return nil
				}

				signal.commitTrade(trade)

				measurement := &types.Measurement{
					ID:       uuid.NewString(),
					Source:   types.SourcePumpDump,
					Symbol:   trade.Symbol,
					At:       trade.Timestamp,
					Maturity: maturity,
					Metrics: map[string]types.MetricSample{
						types.MetricKey(types.MetricBestPrice, types.SideBuy): {
							Raw:  bidPrice,
							Unit: types.UnitQuoteCurrency,
						},
						types.MetricKey(types.MetricBestPrice, types.SideSell): {
							Raw:  askPrice,
							Unit: types.UnitQuoteCurrency,
						},
						types.MetricKey(types.MetricMidpoint, types.SideNone): {
							Raw:  mid,
							Unit: types.UnitQuoteCurrency,
						},
						types.MetricKey(types.MetricTradePrice, types.SideNone): {
							Raw:  trade.Price.Float64(),
							Unit: types.UnitQuoteCurrency,
						},
						types.MetricKey(types.MetricTradeQuantity, types.SideNone): {
							Raw:  trade.Qty,
							Unit: types.UnitBaseCurrency,
						},
						types.MetricKey(types.MetricRVOL, types.SideNone): {
							Raw:        output.RVOL,
							Normalized: normalizedIgnitionEvidence(types.MetricRVOL, output.RVOL, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricPrecursor, types.SideNone): {
							Raw:        output.Precursor,
							Normalized: normalizedIgnitionEvidence(types.MetricPrecursor, output.Precursor, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricSpread, types.SideNone): {
							Raw:        output.Spread,
							Normalized: normalizedSpread(output.Spread, mid),
							Unit:       types.UnitQuoteCurrency,
						},
						types.MetricKey(types.MetricCompression, types.SideNone): {
							Raw:        output.Compression,
							Normalized: normalizedIgnitionEvidence(types.MetricCompression, output.Compression, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricIgnition, types.SideNone): {
							Raw:        output.Ignition,
							Normalized: normalizedIgnitionEvidence(types.MetricIgnition, output.Ignition, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricTrend, types.SideNone): {
							Raw:        output.Trend,
							Normalized: normalizedIgnitionEvidence(types.MetricTrend, output.Trend, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricExhaustion, types.SideNone): {
							Raw:        output.Exhaustion,
							Normalized: normalizedIgnitionEvidence(types.MetricExhaustion, output.Exhaustion, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricStrength, types.SideNone): {
							Raw:        output.Strength,
							Normalized: normalizedIgnitionEvidence(types.MetricStrength, output.Strength, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricPrecursor, types.SideBuy): {
							Raw:        output.Buy.Precursor,
							Normalized: normalizedIgnitionEvidence(types.MetricPrecursor, output.Buy.Precursor, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricCompression, types.SideBuy): {
							Raw:        output.Buy.Compression,
							Normalized: normalizedIgnitionEvidence(types.MetricCompression, output.Buy.Compression, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricIgnition, types.SideBuy): {
							Raw:        output.Buy.Ignition,
							Normalized: normalizedIgnitionEvidence(types.MetricIgnition, output.Buy.Ignition, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricTrend, types.SideBuy): {
							Raw:        output.Buy.Trend,
							Normalized: normalizedIgnitionEvidence(types.MetricTrend, output.Buy.Trend, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricExhaustion, types.SideBuy): {
							Raw:        output.Buy.Exhaustion,
							Normalized: normalizedIgnitionEvidence(types.MetricExhaustion, output.Buy.Exhaustion, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricStrength, types.SideBuy): {
							Raw:        output.Buy.Strength,
							Normalized: normalizedIgnitionEvidence(types.MetricStrength, output.Buy.Strength, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricPrecursor, types.SideSell): {
							Raw:        output.Sell.Precursor,
							Normalized: normalizedIgnitionEvidence(types.MetricPrecursor, output.Sell.Precursor, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricCompression, types.SideSell): {
							Raw:        output.Sell.Compression,
							Normalized: normalizedIgnitionEvidence(types.MetricCompression, output.Sell.Compression, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricIgnition, types.SideSell): {
							Raw:        output.Sell.Ignition,
							Normalized: normalizedIgnitionEvidence(types.MetricIgnition, output.Sell.Ignition, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricTrend, types.SideSell): {
							Raw:        output.Sell.Trend,
							Normalized: normalizedIgnitionEvidence(types.MetricTrend, output.Sell.Trend, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricExhaustion, types.SideSell): {
							Raw:        output.Sell.Exhaustion,
							Normalized: normalizedIgnitionEvidence(types.MetricExhaustion, output.Sell.Exhaustion, ready),
							Unit:       types.UnitDimensionless,
						},
						types.MetricKey(types.MetricStrength, types.SideSell): {
							Raw:        output.Sell.Strength,
							Normalized: normalizedIgnitionEvidence(types.MetricStrength, output.Sell.Strength, ready),
							Unit:       types.UnitDimensionless,
						},
					},
				}
				snr, snrReady := types.MeasurementSignalNoiseRatio(
					types.SourcePumpDump,
					measurement.Metrics,
				)
				snrSample := types.MetricSample{
					Raw:  snr,
					Unit: types.UnitDimensionless,
				}

				if snrReady {
					snrSample.Normalized = &snr
				}

				measurement.PutMetric(types.MetricSNR, types.SideNone, snrSample)

				symbolMeasurements = append(symbolMeasurements, measurement)
			}

			results.Store(symbol, symbolMeasurements)

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"pumpdump: parallel measurement failed",
			err,
		))
	}

	errorsBySymbol.Range(func(key, value any) bool {
		err := value.(error)
		errnie.Error(errnie.Err(
			errnie.Validation,
			"pumpdump: failed to measure ignition",
			err,
		))
		return true
	})

	for _, symbol := range symbols {
		raw, exists := results.Load(symbol)

		if !exists {
			continue
		}

		symbolMeasurements := raw.([]*types.Measurement)
		measurements = append(measurements, symbolMeasurements...)

		if symbol == types.Focus() {
			out = append(out, symbolMeasurements...)
		}
	}

	if len(out) > 0 {
		utils.Publish(signal.ui, datura.NewMap("measurements", out))
	}

	return measurements, true
}

func (signal *Signal) algorithm(symbol string) *equation.Ignition {
	capacity := signal.capacity

	if capacity <= 0 {
		capacity = viper.GetViper().GetInt("signals.pumpdump.baselineCapacity")
	}

	raw, _ := signal.algorithms.LoadOrStore(symbol, equation.NewIgnition(capacity))

	return raw.(*equation.Ignition)
}

/*
normalizedIgnitionEvidence accepts only ready empirical ignition evidence.
Unbounded baseline ratios use parity as their domain scale and map to their
share against parity; bounded scores retain their calculated value. Before the
volume-clock baselines mature, raw placeholders cannot enter normalized math.
*/
func normalizedIgnitionEvidence(
	metric types.MetricType,
	raw float64,
	ready bool,
) *float64 {
	if !ready {
		return nil
	}

	if raw < 0 {
		return nil
	}

	value := raw

	if metric == types.MetricRVOL || metric == types.MetricPrecursor ||
		metric == types.MetricIgnition || metric == types.MetricTrend ||
		metric == types.MetricStrength {
		value = raw / (1 + raw)
	} else if raw > 1 {
		return nil
	}

	return &value
}

/*
normalizedSpread reports executable spread as a fraction of the authoritative
book midpoint observed no later than the trade.
*/
func normalizedSpread(raw, midpoint float64) *float64 {
	if raw <= 0 || midpoint <= 0 {
		return nil
	}

	value := raw / midpoint

	return &value
}

func validTrade(row kraken.TradeData) bool {
	price := row.Price.Float64()

	return row.Symbol != "" && !row.Timestamp.IsZero() && price > 0 && row.Qty > 0 &&
		(row.Side == "buy" || row.Side == "sell")
}

func (signal *Signal) seenTrade(row kraken.TradeData) bool {
	if signal.lastTrade == nil {
		return false
	}

	raw, exists := signal.lastTrade.Load(row.Symbol)

	if !exists {
		return false
	}

	previous := raw.(tradeCursor)

	if row.Timestamp.Before(previous.at) {
		return true
	}

	if row.Timestamp.After(previous.at) {
		return false
	}

	_, seen := previous.ids[row.TradeID]

	return seen
}

func (signal *Signal) commitTrade(row kraken.TradeData) {
	if signal.lastTrade == nil {
		signal.lastTrade = &sync.Map{}
	}

	previous := tradeCursor{}
	raw, exists := signal.lastTrade.Load(row.Symbol)

	if exists {
		previous = raw.(tradeCursor)
	}

	if row.Timestamp.After(previous.at) {
		previous = tradeCursor{at: row.Timestamp, ids: make(map[int64]struct{})}
	}

	if previous.ids == nil {
		previous.ids = make(map[int64]struct{})
	}

	previous.ids[row.TradeID] = struct{}{}
	signal.lastTrade.Store(row.Symbol, previous)
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
