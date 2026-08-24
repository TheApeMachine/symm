package tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/backtest"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	tes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

/*
WithStagedReplay defers each analytical stage until replay closes one
engine-clock interval. PumpDump advances at each trade and committed Level 3
observation because it spans all three raw queues and reads the resident book;
the ticker boundary releases every other retained source and the analyzer
groups in dependency order before graph, planner, and desk settle.
*/
func (market *Market) WithStagedReplay(
	bus *runtime.Workspace,
	failure func() error,
) *Market {
	if bus == nil {
		panic("market: staged replay requires the system bus")
	}

	market.bus = bus
	market.replayStart = func() {}
	market.replayObservation = func() error {
		if failure != nil {
			if err := failure(); err != nil {
				return err
			}
		}

		return bus.WaitForQuiescence(market.ctx)
	}
	market.replaySettlement = func() error {
		if failure != nil {
			if err := failure(); err != nil {
				return err
			}
		}

		return bus.WaitForQuiescence(market.ctx)
	}

	return market
}

/*
SettleReplay fences every captured ingress connection before committing the
current derived interval. A settlement failure stops replay before another
captured frame can cross the boundary.
*/
func (market *Market) SettleReplay() error {
	market.beginReplay()

	if market.replaySettlement == nil {
		return nil
	}

	if !market.Public.Fence() {
		return fmt.Errorf("market: public replay ingress fence failed")
	}

	if !market.Private.Fence() {
		return fmt.Errorf("market: private replay ingress fence failed")
	}

	if !market.Level3.Fence() {
		return fmt.Errorf("market: level3 replay ingress fence failed")
	}

	if err := market.replaySettlement(); err != nil {
		return fmt.Errorf("market: settle replay stages: %w", err)
	}

	return nil
}

/*
settleReplayObservation fences one raw capture connection before advancing the
PumpDump estimator over that exact trade or resident-book observation. Derived
reasoning remains held until the next engine-clock settlement.
*/
func (market *Market) settleReplayObservation(connection *Conn) error {
	if market.replayObservation == nil {
		return nil
	}

	if connection == nil || !connection.Fence() {
		return fmt.Errorf("market: replay observation ingress fence failed")
	}

	if err := market.replayObservation(); err != nil {
		return fmt.Errorf("market: settle replay observation: %w", err)
	}

	return nil
}

func (market *Market) beginReplay() {
	if market.replayStarted {
		return
	}

	market.replayStarted = true

	if market.replayStart != nil {
		market.replayStart()
	}
}

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

			return market.SettleReplay()
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

		if err = market.ReplayFrame(backtest.Frame{
			Endpoint:   endpoint,
			Payload:    frame["payload"],
			ReceivedAt: receivedAt,
		}); err != nil {
			return fmt.Errorf("market: replay record %d: %w", record, err)
		}

		previousAt = receivedAt
		records++
	}
}

/*
ReplayFrame applies one stored capture frame through the fixture transport.
Ticker and trade frames advance Market's observed sample once before their one
transport publication. PumpDump consumes each trade and committed Level 3
state in capture order. Level 3 execution advances only after that exact frame
has been published, applied to the production resident book, and checksummed.
*/
func (market *Market) ReplayFrame(frame backtest.Frame) error {
	if frame.ReceivedAt.IsZero() {
		return fmt.Errorf("market: replay frame has no arrival time")
	}
	market.beginReplay()

	if strings.HasPrefix(frame.Endpoint, "/") ||
		frame.Endpoint == "symm_metadata" {
		return nil
	}

	if frame.Endpoint == "futures" {
		if market.thesis != nil {
			var futuresHeader struct {
				Feed      string `json:"feed"`
				ProductID string `json:"product_id"`
			}

			if json.Unmarshal(frame.Payload, &futuresHeader) == nil {
				switch futuresHeader.Feed {
				case "ticker":
					ticker := kraken.NewFuturesTicker(frame.Payload)
					if ticker != nil && ticker.Data.ProductID != "" {
						if spotSymbol := kraken.FuturesProductIDToSpot(ticker.Data.ProductID); spotSymbol != "" {
							ticker.Data.Symbol = spotSymbol
							runtime.ChannelOf[kraken.FuturesTickerData](
								market.bus, types.ChannelFuturesTickers,
								func(t kraken.FuturesTickerData) string { return t.Symbol },
							).Publish(ticker.Data)
						}
					}
				case "trade", "trade_snapshot":
					trades := kraken.NewFuturesTrade(frame.Payload)
					if trades != nil && len(trades.Data) > 0 {
						for _, singleTrade := range trades.Data {
							if spotSymbol := kraken.FuturesProductIDToSpot(singleTrade.ProductID); spotSymbol != "" {
								singleTrade.Symbol = spotSymbol
								runtime.ChannelOf[kraken.FuturesTradeData](
									market.bus, types.ChannelFuturesTrades,
									func(t kraken.FuturesTradeData) string { return t.Symbol },
								).Publish(singleTrade)
							}
						}
					}
				case "book", "book_snapshot":
					bookDelta := kraken.NewFuturesBook(frame.Payload)
					if bookDelta != nil && bookDelta.Data.ProductID != "" {
						if spotSymbol := kraken.FuturesProductIDToSpot(bookDelta.Data.ProductID); spotSymbol != "" {
							bookDelta.Data.Symbol = spotSymbol
							runtime.ChannelOf[kraken.FuturesBookData](
								market.bus, types.ChannelFuturesBooks,
								func(b kraken.FuturesBookData) string { return b.Symbol },
							).Publish(bookDelta.Data)
						}
					}
				}
			}
		}

		if market.Futures != nil {
			_ = market.Futures.Publish("futures", frame.Payload)
		}

		return nil
	}

	var header struct {
		Channel string `json:"channel"`
		Type    string `json:"type"`
	}

	if err := json.Unmarshal(frame.Payload, &header); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}

	isData := header.Type == "snapshot" || header.Type == "update"

	switch {
	case frame.Endpoint == "public" && header.Channel == "ticker" && isData:
		ticker := &kraken.Ticker{}

		if err := json.Unmarshal(frame.Payload, ticker); err != nil {
			return fmt.Errorf("decode ticker: %w", err)
		}

		market.replayTicker(ticker)
		market.tick++
	case frame.Endpoint == "public" && header.Channel == "trade" && isData:
		trade := &kraken.Trade{}

		if err := json.Unmarshal(frame.Payload, trade); err != nil {
			return fmt.Errorf("decode trade: %w", err)
		}

		market.replayTrade(trade)
	case frame.Endpoint == "level3" && header.Channel == "level3" && isData:
		level3 := &kraken.Level3{}

		if err := json.Unmarshal(frame.Payload, level3); err != nil {
			return fmt.Errorf("decode level3: %w", err)
		}
		symbols := make([]string, 0, len(level3.Data))

		for _, data := range level3.Data {
			symbols = append(symbols, data.Symbol)
		}

		if err := market.Level3.WaitReady(market.ctx); err != nil {
			return fmt.Errorf("level3 connection not ready: %w", err)
		}

		if !market.Level3.Publish(header.Channel, frame.Payload) {
			return fmt.Errorf("level3 frame was not delivered")
		}

		if err := market.waitForLevel3(level3); err != nil {
			return err
		}

		if err := market.settleReplayObservation(market.Level3); err != nil {
			return err
		}

		return market.processReplayOrders(symbols, frame.ReceivedAt)
	}

	var connection *Conn
	settle := frame.Endpoint == "public" && header.Channel == "ticker" && isData
	observe := frame.Endpoint == "public" && header.Channel == "trade" && isData

	switch frame.Endpoint {
	case "level3":
		connection = market.Level3
	case "public":
		connection = market.Public
	case "private":
		connection = market.Private
	default:
		return fmt.Errorf("unsupported replay endpoint %s", frame.Endpoint)
	}

	if err := connection.WaitReady(market.ctx); err != nil {
		return fmt.Errorf("%s connection not ready: %w", frame.Endpoint, err)
	}

	if !connection.Publish(header.Channel, frame.Payload) {
		return fmt.Errorf(
			"%s/%s frame was not delivered",
			frame.Endpoint,
			header.Channel,
		)
	}

	if settle {
		return market.SettleReplay()
	}

	if observe {
		return market.settleReplayObservation(market.Public)
	}

	return nil
}

func (market *Market) replayTicker(ticker *kraken.Ticker) {
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

		market.latest[data.Symbol] = tes.Sample{
			Symbol: data.Symbol, Bid: data.Bid.Float64(), BidQty: data.BidQty,
			Ask: data.Ask.Float64(), AskQty: data.AskQty, Last: data.Last.Float64(),
			Volume: data.Volume, VWAP: data.Vwap, Low: data.Low.Float64(),
			High: data.High.Float64(), Change: data.Change.Float64(),
			ChangePct: data.ChangePct, Timestamp: data.Timestamp,
		}
		market.Private.transport.setPrice(data.Symbol, data.Bid.Float64())
	}
}

func (market *Market) replayTrade(trade *kraken.Trade) {
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
	}
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
				sample.Bids = append(sample.Bids, tes.DepthLevel{
					Price: level.Price.Float64(), Quantity: level.Quantity.Float64(),
				})
			}

			for level := managed.BestAsk(); level != nil &&
				len(sample.Asks) < market.Config.Execution.DepthLevels; level = level.Higher {
				sample.Asks = append(sample.Asks, tes.DepthLevel{
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
