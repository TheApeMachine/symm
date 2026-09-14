package tables

import (
	"context"
	"slices"
	"sort"
	"time"

	"github.com/apache/iceberg-go"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

/*
TapePublisher abstracts the handoff between the Hindsight archive walk and the learner cohort.
strategy.Tape satisfies this interface directly without introducing circular dependencies.
*/
type TapePublisher interface {
	Publish(fragment types.ReplayFragment)
	Close()
	SetBudget(budget uint64)
	AddObservations(count uint64)
	AddRuns(count uint64)
}

type observationTick struct {
	tick       int64
	venueAt    time.Time
	symbol     string
	price      float64
	bid        float64
	ask        float64
	spread     float64
	qty        float64
	sourceKind string
}

type l3Order struct {
	price float64
	qty   float64
	side  string
}

type bookState struct {
	orders map[string]l3Order
	bids   map[float64]float64
	asks   map[float64]float64
}

func newBookState() *bookState {
	return &bookState{
		orders: make(map[string]l3Order),
		bids:   make(map[float64]float64),
		asks:   make(map[float64]float64),
	}
}

func (book *bookState) apply(side string, event string, orderID string, price float64, qty float64) {
	if event == "add" {
		book.orders[orderID] = l3Order{price: price, qty: qty, side: side}

		if side == "buy" || side == "b" {
			book.bids[price] += qty
		}

		if side == "sell" || side == "s" || side == "ask" || side == "a" {
			book.asks[price] += qty
		}

		return
	}

	if event == "modify" {
		oldOrder, exists := book.orders[orderID]

		if exists {
			if oldOrder.side == "buy" || oldOrder.side == "b" {
				book.bids[oldOrder.price] -= oldOrder.qty

				if book.bids[oldOrder.price] <= 0 {
					delete(book.bids, oldOrder.price)
				}

				book.bids[price] += qty
			}

			if oldOrder.side == "sell" || oldOrder.side == "s" || oldOrder.side == "ask" || oldOrder.side == "a" {
				book.asks[oldOrder.price] -= oldOrder.qty

				if book.asks[oldOrder.price] <= 0 {
					delete(book.asks, oldOrder.price)
				}

				book.asks[price] += qty
			}
		}

		if !exists {
			if side == "buy" || side == "b" {
				book.bids[price] += qty
			}

			if side == "sell" || side == "s" || side == "ask" || side == "a" {
				book.asks[price] += qty
			}
		}

		book.orders[orderID] = l3Order{price: price, qty: qty, side: side}
		return
	}

	if event == "delete" {
		oldOrder, exists := book.orders[orderID]

		if !exists {
			return
		}

		if oldOrder.side == "buy" || oldOrder.side == "b" {
			book.bids[oldOrder.price] -= oldOrder.qty

			if book.bids[oldOrder.price] <= 0 {
				delete(book.bids, oldOrder.price)
			}
		}

		if oldOrder.side == "sell" || oldOrder.side == "s" || oldOrder.side == "ask" || oldOrder.side == "a" {
			book.asks[oldOrder.price] -= oldOrder.qty

			if book.asks[oldOrder.price] <= 0 {
				delete(book.asks, oldOrder.price)
			}
		}

		delete(book.orders, orderID)
	}
}

func (book *bookState) bestBidAsk() (float64, float64, float64, float64) {
	bestBid := 0.0
	bidQty := 0.0
	bestAsk := 0.0
	askQty := 0.0

	for price, qty := range book.bids {
		if qty > 0 && (bestBid == 0 || price > bestBid) {
			bestBid = price
			bidQty = qty
		}
	}

	for price, qty := range book.asks {
		if qty > 0 && (bestAsk == 0 || price < bestAsk) {
			bestAsk = price
			askQty = qty
		}
	}

	return bestBid, bestAsk, bidQty, askQty
}

/*
LoadRehearsalTape walks historical runs in the Iceberg catalog, discovers market
excursion fragments from the reconstructed order book and tape, and publishes them into the training tape.
*/
func (c *Catalog) LoadRehearsalTape(ctx context.Context, tape TapePublisher) error {
	if c == nil || tape == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[iceberg] LoadRehearsalTape: catalog or tape is nil",
			nil,
		))
	}

	ctx = c.Context(ctx)
	defer tape.Close()

	budget := uint64(viper.GetInt("hindsight.rehearsal.observation_budget"))

	if budget == 0 {
		budget = 500000
	}

	tape.SetBudget(budget)

	epochs, err := c.Epochs(ctx)

	if err != nil {
		return err
	}

	if len(epochs) == 0 {
		return nil
	}

	walkEpochs := make([]int64, len(epochs))
	copy(walkEpochs, epochs)
	slices.Reverse(walkEpochs)

	totalObservations := uint64(0)

	for _, epoch := range walkEpochs {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if totalObservations >= budget {
			break
		}

		l3Rows, _ := c.SpotLevel3Scan(ctx, epoch, iceberg.AlwaysTrue{}, 0)
		tickers, _ := c.SpotTickerScan(ctx, epoch, iceberg.AlwaysTrue{}, 0)
		trades, _ := c.SpotTradeScan(ctx, epoch, iceberg.AlwaysTrue{}, 0)

		if len(l3Rows) == 0 && len(tickers) == 0 && len(trades) == 0 {
			continue
		}

		symbolObservations := make(map[string][]observationTick)

		// 1. Reconstruct order books from Level 3 events
		if len(l3Rows) > 0 {
			books := make(map[string]*bookState)

			for _, l3 := range l3Rows {
				if l3.Symbol == "" || l3.LimitPrice <= 0 {
					continue
				}

				book, exists := books[l3.Symbol]

				if !exists {
					book = newBookState()
					books[l3.Symbol] = book
				}

				book.apply(l3.Side, l3.Event, l3.OrderID, l3.LimitPrice, l3.OrderQty)

				bid, ask, bidQty, askQty := book.bestBidAsk()
				price := l3.LimitPrice
				spread := 0.0

				if bid > 0 && ask > 0 && ask > bid {
					price = (bid + ask) / 2
					spread = ask - bid
				}

				if price <= 0 {
					continue
				}

				symbolObservations[l3.Symbol] = append(symbolObservations[l3.Symbol], observationTick{
					tick:       l3.Tick,
					venueAt:    l3.VenueAt,
					symbol:     l3.Symbol,
					price:      price,
					bid:        bid,
					ask:        ask,
					spread:     spread,
					qty:        bidQty + askQty,
					sourceKind: "l3",
				})
			}
		}

		// 2. Incorporate spot tickers using true midpoint
		for _, ticker := range tickers {
			bid := ticker.Bid
			ask := ticker.Ask
			price := ticker.Last
			spread := 0.0

			if bid > 0 && ask > 0 && ask >= bid {
				price = (bid + ask) / 2
				spread = ask - bid
			}

			if price <= 0 && bid > 0 {
				price = bid
			}

			if price <= 0 && ask > 0 {
				price = ask
			}

			if price <= 0 {
				continue
			}

			symbolObservations[ticker.Symbol] = append(symbolObservations[ticker.Symbol], observationTick{
				tick:       ticker.Tick,
				venueAt:    ticker.VenueAt,
				symbol:     ticker.Symbol,
				price:      price,
				bid:        bid,
				ask:        ask,
				spread:     spread,
				qty:        ticker.BidQty + ticker.AskQty,
				sourceKind: "ticker",
			})
		}

		// 3. Incorporate spot trades
		for _, trade := range trades {
			if trade.Price <= 0 {
				continue
			}

			symbolObservations[trade.Symbol] = append(symbolObservations[trade.Symbol], observationTick{
				tick:       trade.Tick,
				venueAt:    trade.VenueAt,
				symbol:     trade.Symbol,
				price:      trade.Price,
				qty:        trade.Qty,
				sourceKind: "trade",
			})
		}

		symbols := make([]string, 0, len(symbolObservations))

		for symbol, ticks := range symbolObservations {
			if len(ticks) < 4 {
				continue
			}

			symbols = append(symbols, symbol)
		}

		sort.Strings(symbols)

		symbolFragments := make(map[string][]types.ReplayFragment)
		maxFragments := 0

		for _, symbol := range symbols {
			ticks := symbolObservations[symbol]

			sort.Slice(ticks, func(firstIndex, secondIndex int) bool {
				return ticks[firstIndex].tick < ticks[secondIndex].tick
			})

			// Discover start and end of market events across the reconstructed tape
			fragments := discoverMarketEvents(symbol, ticks)

			if len(fragments) > 0 {
				symbolFragments[symbol] = fragments

				if len(fragments) > maxFragments {
					maxFragments = len(fragments)
				}
			}
		}

		fragmentsPublished := 0

		for round := 0; round < maxFragments; round++ {
			if totalObservations >= budget {
				break
			}

			for _, symbol := range symbols {
				frags := symbolFragments[symbol]

				if round >= len(frags) {
					continue
				}

				if totalObservations >= budget {
					break
				}

				frag := frags[round]
				tape.Publish(frag)
				obsCount := uint64(len(frag.Frames))
				totalObservations += obsCount
				tape.AddObservations(obsCount)
				fragmentsPublished++
			}
		}

		if fragmentsPublished > 0 {
			tape.AddRuns(1)
		}
	}

	return nil
}

type marketEvent struct {
	anchorIndex    int
	extremumIndex  int
	clearsFriction bool
}

/*
discoverMarketEvents scans the chronological tape for a symbol, identifying genuine
upward and downward price legs (start anchor B to peak/trough C) with precursor context A -> B.
It yields both movements that clear friction and movements that do not, creating varied learning data.
*/
func discoverMarketEvents(symbol string, ticks []observationTick) []types.ReplayFragment {
	totalTicks := len(ticks)

	if totalTicks < 8 {
		return nil
	}

	// Compute average spread / friction from quotes
	totalSpread := 0.0
	spreadCount := 0

	for _, item := range ticks {
		if item.spread > 0 {
			totalSpread += item.spread / item.price
			spreadCount++
		}
	}

	frictionRate := 0.002

	if spreadCount > 0 && totalSpread > 0 {
		frictionRate = totalSpread / float64(spreadCount)
	}

	var events []marketEvent

	// Scan with adaptive step sizes to find local inflections
	stepHorizon := 16

	if totalTicks < 32 {
		stepHorizon = totalTicks / 2
	}

	for anchorPos := 0; anchorPos+4 < totalTicks; anchorPos += max(1, stepHorizon/2) {
		anchorPrice := ticks[anchorPos].price
		windowEnd := min(anchorPos+stepHorizon*2, totalTicks)

		maxPrice := anchorPrice
		maxPos := anchorPos
		minPrice := anchorPrice
		minPos := anchorPos

		for candidatePos := anchorPos + 1; candidatePos < windowEnd; candidatePos++ {
			candPrice := ticks[candidatePos].price

			if candPrice > maxPrice {
				maxPrice = candPrice
				maxPos = candidatePos
			}

			if candPrice < minPrice {
				minPrice = candPrice
				minPos = candidatePos
			}
		}

		upwardMove := (maxPrice - anchorPrice) / anchorPrice
		downwardMove := (anchorPrice - minPrice) / anchorPrice

		if upwardMove > 0 && maxPos > anchorPos {
			clears := upwardMove >= frictionRate
			events = append(events, marketEvent{
				anchorIndex:    anchorPos,
				extremumIndex:  maxPos,
				clearsFriction: clears,
			})
		}

		if downwardMove > 0 && minPos > anchorPos {
			clears := downwardMove >= frictionRate
			events = append(events, marketEvent{
				anchorIndex:    anchorPos,
				extremumIndex:  minPos,
				clearsFriction: clears,
			})
		}
	}

	if len(events) == 0 {
		return nil
	}

	var fragments []types.ReplayFragment

	for _, evt := range events {
		// Precursor window: up to 16 observations before B
		precursorLen := 12

		if evt.anchorIndex < precursorLen {
			precursorLen = evt.anchorIndex
		}

		startPos := evt.anchorIndex - precursorLen

		// Post-extremum window: up to 8 observations after C
		postLen := 8

		if evt.extremumIndex+postLen > totalTicks {
			postLen = totalTicks - evt.extremumIndex
		}

		endPos := evt.extremumIndex + postLen

		if endPos <= startPos {
			continue
		}

		slice := ticks[startPos:endPos]
		relAnchor := evt.anchorIndex - startPos
		relExtremum := evt.extremumIndex - startPos

		if relAnchor >= len(slice) || relExtremum >= len(slice) {
			continue
		}

		fragment := buildFragmentWithIndices(symbol, slice, relAnchor, relExtremum)
		fragments = append(fragments, fragment)
	}

	return fragments
}

func buildFragmentWithIndices(
	symbol string,
	slice []observationTick,
	anchorIndex int,
	extremumIndex int,
) types.ReplayFragment {
	frames := make([][]*data.Measurement[float64], len(slice))

	for index, item := range slice {
		metrics := map[string]data.Metric[float64]{
			"price": data.NewMetric[float64]("price", data.UnitDimensionless, data.TimescaleInstantaneous, 0, item.price),
			"qty":   data.NewMetric[float64]("qty", data.UnitCount, data.TimescaleInstantaneous, 0, item.qty),
		}

		if item.bid > 0 {
			metrics["bid"] = data.NewMetric[float64]("bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, item.bid)
		}

		if item.ask > 0 {
			metrics["ask"] = data.NewMetric[float64]("ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, item.ask)
		}

		if item.spread > 0 {
			metrics["spread"] = data.NewMetric[float64]("spread", data.UnitDimensionless, data.TimescaleInstantaneous, 0, item.spread)
		}

		meas := data.NewMeasurement[float64]("market", metrics)
		meas.Label = symbol
		meas.At = item.venueAt
		meas.SeqIdx = item.tick

		frames[index] = []*data.Measurement[float64]{meas}
	}

	return types.ReplayFragment{
		Frames:        frames,
		Symbol:        symbol,
		AnchorIndex:   anchorIndex,
		ExtremumIndex: extremumIndex,
	}
}
