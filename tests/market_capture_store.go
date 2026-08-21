package tests

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/backtest"
	"github.com/theapemachine/symm/kraken"
	testtypes "github.com/theapemachine/symm/tests/types"
)

type capturedActiveFee struct {
	pair  string
	taker float64
	maker float64
}

/*
CaptureSymbolsFromStoredFrames reconstructs the exact subscribed venue
profiles from a legacy SQLite capture. The recorded TradeVolume batches and
trade subscription acknowledgements share the order of the original broker
batch, which preserves Kraken's legacy REST pair aliases without guessing
their names from WebSocket symbols.
*/
func CaptureSymbolsFromStoredFrames(
	frames func() (backtest.Frame, bool),
	depth int,
) ([]*testtypes.Symbol, error) {
	if frames == nil || depth <= 0 {
		return nil, fmt.Errorf("market: stored frames and positive depth required")
	}

	instruments := make(map[string]kraken.InstrumentPair)
	compactSymbols := make(map[string]string)
	starts := make(map[string]capturedStart)
	fees := make([]capturedActiveFee, 0)
	symbols := make([]string, 0)
	seenSymbols := make(map[string]struct{})
	feeBatchSize := 0
	target := 0
	record := 0

	for {
		frame, ok := frames()

		if !ok {
			break
		}

		record++

		if frame.Endpoint == "/0/private/TradeVolume" {
			if len(symbols) != len(fees) {
				return nil, fmt.Errorf(
					"market: fee batch at record %d arrived with %d unmatched schedules",
					record,
					len(fees)-len(symbols),
				)
			}

			batch, err := storedCaptureFees(frame.Payload)
			if err != nil {
				return nil, fmt.Errorf("market: fee batch at record %d: %w", record, err)
			}

			if feeBatchSize == 0 {
				feeBatchSize = len(batch)
			}

			if len(batch) > feeBatchSize || target != 0 {
				return nil, fmt.Errorf("market: inconsistent fee batch at record %d", record)
			}

			fees = append(fees, batch...)

			if len(batch) < feeBatchSize {
				target = len(fees)
			}

			continue
		}

		if frame.Endpoint != "public" {
			continue
		}

		var header struct {
			Channel string `json:"channel"`
			Type    string `json:"type"`
			Method  string `json:"method"`
			Success bool   `json:"success"`
			Result  struct {
				Channel string `json:"channel"`
				Symbol  string `json:"symbol"`
			} `json:"result"`
		}

		if err := json.Unmarshal(frame.Payload, &header); err != nil {
			return nil, fmt.Errorf("market: public frame %d: %w", record, err)
		}

		if header.Channel == "instrument" && header.Type == "snapshot" {
			var instrument kraken.Instrument

			if err := json.Unmarshal(frame.Payload, &instrument); err != nil {
				return nil, fmt.Errorf("market: instrument snapshot: %w", err)
			}

			for _, pair := range instrument.Data.Pairs {
				if _, exists := instruments[pair.Symbol]; exists {
					return nil, fmt.Errorf("market: duplicate instrument %s", pair.Symbol)
				}

				instruments[pair.Symbol] = pair
				compactSymbols[strings.ReplaceAll(pair.Symbol, "/", "")] = pair.Symbol
			}
		}

		if header.Channel == "ticker" &&
			(header.Type == "snapshot" || header.Type == "update") {
			var ticker kraken.Ticker

			if err := json.Unmarshal(frame.Payload, &ticker); err != nil {
				return nil, fmt.Errorf("market: ticker at record %d: %w", record, err)
			}

			for _, point := range ticker.Data {
				if _, exists := starts[point.Symbol]; exists {
					continue
				}

				if point.Bid == nil || point.Ask == nil ||
					point.Bid.Sign() <= 0 || point.Ask.Cmp(point.Bid) <= 0 {
					return nil, fmt.Errorf("market: invalid first ticker for %s", point.Symbol)
				}

				start := capturedStart{
					bid: point.Bid.Float64(),
					ask: point.Ask.Float64(),
				}

				if point.Last != nil && point.Last.Sign() > 0 {
					start.last = point.Last.Float64()
				}

				starts[point.Symbol] = start
			}
		}

		if header.Method == "subscribe" && header.Success &&
			header.Result.Channel == "trade" {
			if header.Result.Symbol == "" {
				return nil, fmt.Errorf("market: unnamed trade subscription at record %d", record)
			}

			if _, exists := seenSymbols[header.Result.Symbol]; exists {
				return nil, fmt.Errorf("market: duplicate trade subscription %s", header.Result.Symbol)
			}

			if len(symbols) >= len(fees) {
				return nil, fmt.Errorf("market: trade subscription has no fee row for %s", header.Result.Symbol)
			}

			pair, exists := instruments[header.Result.Symbol]
			if !exists {
				return nil, fmt.Errorf("market: subscribed instrument %s is missing", header.Result.Symbol)
			}

			fee := fees[len(symbols)]
			directIdentifier := strings.ReplaceAll(header.Result.Symbol, "/", "")

			if fee.pair != directIdentifier {
				if bound, exists := compactSymbols[fee.pair]; exists && bound != pair.Symbol {
					return nil, fmt.Errorf(
						"market: fee row %s belongs to %s, not %s",
						fee.pair,
						bound,
						pair.Symbol,
					)
				}

				if !strings.HasSuffix(fee.pair, pair.Quote) {
					return nil, fmt.Errorf("market: fee row %s does not quote %s", fee.pair, pair.Quote)
				}
			}

			seenSymbols[header.Result.Symbol] = struct{}{}
			symbols = append(symbols, header.Result.Symbol)
		}

		if target != 0 && len(symbols) == target && storedCaptureStartsReady(symbols, starts) {
			break
		}
	}

	if len(symbols) == 0 || len(symbols) != len(fees) {
		return nil, fmt.Errorf(
			"market: incomplete subscriptions: %d symbols for %d fee rows",
			len(symbols),
			len(fees),
		)
	}

	configured := make([]*testtypes.Symbol, 0, len(symbols))

	for index, name := range symbols {
		pair := instruments[name]
		start, exists := starts[name]

		if !exists {
			return nil, fmt.Errorf("market: first ticker missing for %s", name)
		}

		if err := validateStoredInstrument(pair); err != nil {
			return nil, err
		}

		midpoint := (start.bid + start.ask) / 2
		startPrice := start.last

		// Kraken can report no last trade for an otherwise executable market.
		// In that explicit state, the first executable midpoint is its mark.
		if startPrice == 0 {
			startPrice = midpoint
		}

		fee := fees[index]
		symbol := testtypes.NewSymbol(name, startPrice, int64(index+1))
		symbol.PriceIncrement = pair.PriceIncrement.Float64()
		symbol.PricePrecision = pair.PricePrecision
		symbol.QuantityPrecision = pair.QtyPrecision
		symbol.BaseSpreadFraction = (start.ask - start.bid) / midpoint
		symbol.TakerFeePercent = fee.taker
		symbol.MakerFeePercent = fee.maker
		symbol.OrderMinimum = pair.QtyMin.Float64()
		symbol.CostMinimum = pair.CostMin.Float64()
		symbol.BookDepthLevels = depth
		configured = append(configured, symbol)
	}

	return configured, nil
}

func storedCaptureFees(payload []byte) ([]capturedActiveFee, error) {
	var response kraken.TradeVolume

	if err := json.Unmarshal(payload, &response); err != nil {
		return nil, fmt.Errorf("decode TradeVolume: %w", err)
	}

	if len(response.Error) != 0 || len(response.Result.Schedules) == 0 {
		return nil, fmt.Errorf("successful TradeVolume schedules are required")
	}

	fees := make([]capturedActiveFee, 0, len(response.Result.Schedules))
	seen := make(map[string]struct{}, len(response.Result.Schedules))

	for _, schedule := range response.Result.Schedules {
		if schedule.Pair == "" {
			return nil, fmt.Errorf("TradeVolume schedule pair is required")
		}

		if _, exists := seen[schedule.Pair]; exists {
			return nil, fmt.Errorf("duplicate TradeVolume schedule %s", schedule.Pair)
		}

		var active *kraken.TradeVolumeTier

		for index := range schedule.Tiers {
			if !schedule.Tiers[index].Active {
				continue
			}

			if active != nil {
				return nil, fmt.Errorf("multiple active fee tiers for %s", schedule.Pair)
			}

			active = &schedule.Tiers[index]
		}

		taker, takerExists := response.Result.Fees[schedule.Pair]
		maker, makerExists := response.Result.FeesMaker[schedule.Pair]

		if active == nil || active.TakerFee == nil || active.MakerFee == nil ||
			!takerExists || !makerExists || taker.Fee == nil || maker.Fee == nil ||
			active.TakerFee.Sign() < 0 || active.MakerFee.Sign() < 0 ||
			active.TakerFee.Cmp(taker.Fee) != 0 || active.MakerFee.Cmp(maker.Fee) != 0 {
			return nil, fmt.Errorf("inconsistent active fee tier for %s", schedule.Pair)
		}

		seen[schedule.Pair] = struct{}{}
		fees = append(fees, capturedActiveFee{
			pair: schedule.Pair, taker: taker.Fee.Float64(), maker: maker.Fee.Float64(),
		})
	}

	return fees, nil
}

func storedCaptureStartsReady(
	symbols []string,
	starts map[string]capturedStart,
) bool {
	for _, symbol := range symbols {
		if _, exists := starts[symbol]; !exists {
			return false
		}
	}

	return true
}

func validateStoredInstrument(pair kraken.InstrumentPair) error {
	if pair.Symbol == "" || pair.Status != "online" || pair.PricePrecision < 0 ||
		pair.QtyPrecision < 0 || pair.PriceIncrement.Sign() <= 0 ||
		pair.TickSize.Sign() <= 0 || pair.PriceIncrement.Cmp(&pair.TickSize) != 0 ||
		pair.QtyIncrement == nil || pair.QtyMin == nil || pair.CostMin == nil ||
		pair.QtyIncrement.Sign() <= 0 || pair.QtyMin.Sign() <= 0 ||
		pair.CostMin.Sign() <= 0 {
		return fmt.Errorf("market: incomplete instrument profile for %s", pair.Symbol)
	}

	expectedIncrement, err := decimal.NewFromString(decimalUnit(pair.QtyPrecision))
	if err != nil {
		return fmt.Errorf("market: quantity precision for %s: %w", pair.Symbol, err)
	}

	if pair.QtyIncrement.Cmp(expectedIncrement) != 0 {
		return fmt.Errorf("market: quantity increment disagrees with precision for %s", pair.Symbol)
	}

	return nil
}

func decimalUnit(precision int) string {
	if precision == 0 {
		return "1"
	}

	return "0." + strings.Repeat("0", precision-1) + "1"
}
