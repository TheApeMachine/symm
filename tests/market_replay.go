package tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/kraken"
	testtypes "github.com/theapemachine/symm/tests/types"
)

/*
Replay streams captured Kraken frames unchanged through the existing Market
connections. Arrival time orders execution mechanics; payload event timestamps
remain the production system's evidence clock.
*/
func (market *Market) Replay(reader io.Reader) error {
	decoder := json.NewDecoder(reader)
	previousAt := time.Time{}
	records := 0

	for record := 1; ; record++ {
		frame := map[string]json.RawMessage{}
		err := decoder.Decode(&frame)

		if errors.Is(err, io.EOF) {
			if records == 0 {
				return fmt.Errorf("market: replay requires at least one frame")
			}

			if market.stack != nil {
				if err = market.stack.Sync(market.ctx, previousAt); err != nil {
					return fmt.Errorf("market: synchronize replay: %w", err)
				}
			}

			return nil
		}

		if err != nil {
			return fmt.Errorf("market: decode replay record %d: %w", record, err)
		}

		var endpoint string
		var receivedAt time.Time

		if err = json.Unmarshal(frame["endpoint"], &endpoint); err != nil {
			return fmt.Errorf("market: decode replay endpoint %d: %w", record, err)
		}

		if err = json.Unmarshal(frame["received_at"], &receivedAt); err != nil {
			return fmt.Errorf("market: decode replay arrival %d: %w", record, err)
		}

		if receivedAt.IsZero() ||
			(!previousAt.IsZero() && receivedAt.Before(previousAt)) {
			return fmt.Errorf("market: replay record %d has invalid arrival time", record)
		}

		if err = market.replayFrame(
			endpoint,
			frame["payload"],
			receivedAt,
		); err != nil {
			return fmt.Errorf("market: replay record %d: %w", record, err)
		}

		if market.stack != nil {
			if err = market.stack.Sync(market.ctx, receivedAt); err != nil {
				return fmt.Errorf("market: synchronize replay record %d: %w", record, err)
			}
		}

		previousAt = receivedAt
		records++
	}
}

func (market *Market) replayFrame(
	endpoint string,
	payload []byte,
	receivedAt time.Time,
) error {
	header := map[string]json.RawMessage{}

	if err := json.Unmarshal(payload, &header); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}

	var channel string

	if err := json.Unmarshal(header["channel"], &channel); err != nil {
		return fmt.Errorf("decode channel: %w", err)
	}

	var symbols []string

	switch {
	case endpoint == "public" && channel == "ticker":
		ticker := &kraken.Ticker{}

		if err := json.Unmarshal(payload, ticker); err != nil {
			return fmt.Errorf("decode ticker: %w", err)
		}

		symbols = market.replayTicker(ticker)
		market.tick++
	case endpoint == "public" && channel == "trade":
		trade := &kraken.Trade{}

		if err := json.Unmarshal(payload, trade); err != nil {
			return fmt.Errorf("decode trade: %w", err)
		}

		symbols = market.replayTrade(trade)
	case endpoint == "level3" && channel == "level3":
		level3 := &kraken.Level3{}

		if err := json.Unmarshal(payload, level3); err != nil {
			return fmt.Errorf("decode level3: %w", err)
		}

		for _, data := range level3.Data {
			symbols = append(symbols, data.Symbol)
		}

		if err := market.Level3.WaitReady(market.ctx); err != nil {
			return fmt.Errorf("level3 connection not ready: %w", err)
		}

		if !market.Level3.Publish(channel, payload) {
			return fmt.Errorf("level3 frame was not delivered")
		}

		if err := market.waitForLevel3(level3); err != nil {
			return err
		}

		return market.processReplayOrders(symbols, receivedAt)
	default:
		return fmt.Errorf("unsupported %s/%s frame", endpoint, channel)
	}

	if err := market.Public.WaitReady(market.ctx); err != nil {
		return fmt.Errorf("public connection not ready: %w", err)
	}

	if !market.Public.Publish(channel, payload) {
		return fmt.Errorf("public %s frame was not delivered", channel)
	}

	return market.processReplayOrders(symbols, receivedAt)
}

func (market *Market) replayTicker(ticker *kraken.Ticker) []string {
	symbols := make([]string, 0, len(ticker.Data))
	market.sampleMu.Lock()
	defer market.sampleMu.Unlock()

	for _, data := range ticker.Data {
		if data.Bid == nil || data.Ask == nil || data.Last == nil ||
			data.Low == nil || data.High == nil || data.Change == nil {
			continue
		}

		if known, found := market.latest[data.Symbol]; found {
			market.previous[data.Symbol] = known
		}

		market.latest[data.Symbol] = testtypes.Sample{
			Symbol: data.Symbol, Bid: data.Bid.Float64(), BidQty: data.BidQty,
			Ask: data.Ask.Float64(), AskQty: data.AskQty, Last: data.Last.Float64(),
			Volume: data.Volume, VWAP: data.Vwap, Low: data.Low.Float64(),
			High: data.High.Float64(), Change: data.Change.Float64(),
			ChangePct: data.ChangePct, Timestamp: data.Timestamp,
		}
		market.Private.transport.setPrice(data.Symbol, data.Bid.Float64())
		symbols = append(symbols, data.Symbol)
	}

	return symbols
}

func (market *Market) replayTrade(trade *kraken.Trade) []string {
	symbols := make([]string, 0, len(trade.Data))
	market.sampleMu.Lock()
	defer market.sampleMu.Unlock()

	for _, data := range trade.Data {
		sample, found := market.latest[data.Symbol]

		if found {
			sample.Last = data.Price.Float64()
			sample.StepVolume = data.Qty
			sample.AggressorSide = data.Side
			market.latest[data.Symbol] = sample
		}

		symbols = append(symbols, data.Symbol)
	}

	return symbols
}

func (market *Market) processReplayOrders(
	symbols []string,
	receivedAt time.Time,
) error {
	if !market.autoFill || market.execution == nil {
		return nil
	}

	for _, symbol := range symbols {
		sample, found := market.LastSample(symbol)

		if !found {
			continue
		}

		sample.Timestamp = receivedAt
		sample.Bids = nil
		sample.Asks = nil
		bookFound := false
		market.private.Book(symbol, func(managed *spotbook.Book) {
			bookFound = true

			for level := managed.BestBid(); level != nil &&
				len(sample.Bids) < market.Config.Execution.DepthLevels; level = level.Lower {
				sample.Bids = append(sample.Bids, testtypes.DepthLevel{
					Price: level.Price.Float64(), Quantity: level.Quantity.Float64(),
				})
			}

			for level := managed.BestAsk(); level != nil &&
				len(sample.Asks) < market.Config.Execution.DepthLevels; level = level.Higher {
				sample.Asks = append(sample.Asks, testtypes.DepthLevel{
					Price: level.Price.Float64(), Quantity: level.Quantity.Float64(),
				})
			}
		})

		if !bookFound || len(sample.Bids) == 0 || len(sample.Asks) == 0 {
			return fmt.Errorf("executable level3 book required for %s", symbol)
		}

		sample.Bid, sample.BidQty = sample.Bids[0].Price, sample.Bids[0].Quantity
		sample.Ask, sample.AskQty = sample.Asks[0].Price, sample.Asks[0].Quantity
		market.execution.Process(sample, market.states[symbol])
	}

	return nil
}

func (market *Market) waitForLevel3(payload *kraken.Level3) error {
	timeout := time.NewTimer(market.Config.BookApplyTimeout)
	defer timeout.Stop()
	poll := time.NewTicker(market.Config.BookPollInterval)
	defer poll.Stop()

	for {
		complete := true

		for _, data := range payload.Data {
			matched := false
			checksumMatch := false
			market.private.Book(data.Symbol, func(managed *spotbook.Book) {
				matched = level3DataApplied(managed, data, payload.Type)

				if matched {
					checksumMatch = managed.L3Checksum(strconv.FormatUint(
						uint64(data.Checksum), 10,
					)).Match
				}
			})

			if matched && !checksumMatch {
				return fmt.Errorf("level3 checksum mismatch for %s", data.Symbol)
			}

			complete = complete && matched
		}

		if complete {
			return nil
		}

		select {
		case <-market.ctx.Done():
			return market.ctx.Err()
		case <-poll.C:
		case <-timeout.C:
			return fmt.Errorf("level3 frame was not applied before timeout")
		}
	}
}

func level3DataApplied(
	managed *spotbook.Book,
	data kraken.Level3Data,
	typeName string,
) bool {
	if managed == nil {
		return false
	}

	expected := len(data.Bids) + len(data.Asks)
	observed := 0

	for sideIndex, orders := range [][]kraken.Level3Order{data.Bids, data.Asks} {
		side := managed.Bids
		seen := make(map[string]struct{}, len(orders))

		if sideIndex == 1 {
			side = managed.Asks
		}

		for index := len(orders) - 1; index >= 0; index-- {
			order := orders[index]

			if _, found := seen[order.OrderID]; found {
				continue
			}

			seen[order.OrderID] = struct{}{}
			level := side.Levels[order.LimitPrice.String()]
			found := false

			if level == nil {
				for _, candidate := range side.Levels {
					if candidate.Price.Cmp(order.LimitPrice) == 0 {
						level = candidate
						break
					}
				}
			}

			if level != nil {
				for _, queued := range level.Queue() {
					if queued.ID != order.OrderID {
						continue
					}

					found = order.Event != "delete" &&
						queued.Quantity.Cmp(order.OrderQty) == 0 &&
						queued.Timestamp.Equal(order.Timestamp)
					break
				}
			}

			if order.Event == "delete" && level3OrderExists(level, order.OrderID) {
				return false
			}

			if order.Event != "delete" && !found {
				return false
			}
		}
	}

	if typeName != "snapshot" {
		return true
	}

	for _, side := range []*spotbook.Side{managed.Bids, managed.Asks} {
		for _, level := range side.Levels {
			observed += len(level.Queue())
		}
	}

	return observed == expected
}

func level3OrderExists(level *spotbook.Level, orderID string) bool {
	if level == nil {
		return false
	}

	for _, order := range level.Queue() {
		if order.ID == orderID {
			return true
		}
	}

	return false
}
