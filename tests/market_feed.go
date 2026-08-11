package tests

import (
	"fmt"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/tests/fixtures/book"
	"github.com/theapemachine/symm/tests/fixtures/level3"
	"github.com/theapemachine/symm/tests/fixtures/ticker"
	"github.com/theapemachine/symm/tests/fixtures/trade"
	"github.com/theapemachine/symm/tests/signal"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func (market *Market) publish(
	generator *signal.Generator,
	marketShock float64,
) testtypes.Sample {
	sample := generator.Step(marketShock)
	market.publishSample(generator, sample)

	return sample
}

/*
PublishSample replays an externally supplied coherent market observation
through the same production websocket channels as generated data.
*/
func (market *Market) PublishSample(sample testtypes.Sample) error {
	generator, known := market.generators[sample.Symbol]

	if !known {
		return fmt.Errorf("market: cannot replay unknown symbol %q", sample.Symbol)
	}

	if err := market.validateSample(sample); err != nil {
		return err
	}

	market.publishSample(generator, sample)
	market.tick++

	return nil
}

func (market *Market) validateSample(sample testtypes.Sample) error {
	if sample.Timestamp.IsZero() || sample.Bid <= 0 || sample.Ask <= sample.Bid ||
		sample.Last < sample.Bid || sample.Last > sample.Ask ||
		sample.BidQty <= 0 || sample.AskQty <= 0 || sample.StepVolume <= 0 ||
		sample.Volume < sample.StepVolume {
		return fmt.Errorf("market: replay sample for %s violates price, quantity, or timestamp invariants", sample.Symbol)
	}

	if err := validateDepth(sample.Bids, sample.Bid, sample.BidQty, true); err != nil {
		return fmt.Errorf("market: replay bid depth for %s: %w", sample.Symbol, err)
	}

	if err := validateDepth(sample.Asks, sample.Ask, sample.AskQty, false); err != nil {
		return fmt.Errorf("market: replay ask depth for %s: %w", sample.Symbol, err)
	}

	previous, known := market.LastSample(sample.Symbol)

	if known && !sample.Timestamp.After(previous.Timestamp) {
		return fmt.Errorf("market: replay sample for %s is not newer than the previous sample", sample.Symbol)
	}

	return nil
}

func validateDepth(
	levels []testtypes.DepthLevel,
	topPrice float64,
	topQuantity float64,
	descending bool,
) error {
	if len(levels) == 0 {
		return nil
	}

	if levels[0].Price != topPrice || levels[0].Quantity != topQuantity {
		return fmt.Errorf("first level does not match the declared top of book")
	}

	for index, level := range levels {
		if level.Price <= 0 || level.Quantity <= 0 {
			return fmt.Errorf("level %d is not positive", index)
		}

		if index == 0 {
			continue
		}

		if descending && level.Price >= levels[index-1].Price {
			return fmt.Errorf("level %d is not strictly descending", index)
		}

		if !descending && level.Price <= levels[index-1].Price {
			return fmt.Errorf("level %d is not strictly ascending", index)
		}
	}

	return nil
}

func (market *Market) publishSample(
	generator *signal.Generator,
	sample testtypes.Sample,
) {
	market.pace(sample.Timestamp)
	market.sampleMu.Lock()

	if known, ok := market.latest[sample.Symbol]; ok {
		market.previous[sample.Symbol] = known
	}

	market.latest[sample.Symbol] = sample
	market.sampleMu.Unlock()

	market.Public.Publish(
		"ticker",
		ticker.NewFixture(ticker.UPDATE, 1, generator).Render(sample),
	)
	market.Public.Publish(
		"book",
		book.NewFixture(book.SNAPSHOT, 1, generator).Render(sample),
	)
	level3Type := level3.UPDATE

	if !market.published[sample.Symbol] {
		level3Type = level3.SNAPSHOT
	}

	published := market.Level3.Publish(
		"level3",
		level3.NewFixture(level3Type, 1, generator).Render(sample),
	)

	market.published[sample.Symbol] = published

	market.waitForBook(sample)

	if market.autoFill && market.execution != nil {
		market.execution.Process(sample, market.states[sample.Symbol])
	}

	market.Public.Publish(
		"trade",
		trade.NewFixture(trade.UPDATE, 1, generator).Render(sample),
	)
	market.Public.Publish("ohlc", market.renderCandle(sample))
}

func (market *Market) pace(sampleAt time.Time) {
	if market.stack == nil {
		return
	}

	if !market.clockSet {
		market.clockSet = true
		market.clockAt = time.Now()
		market.sampleAt = sampleAt
		return
	}

	deliveryAt := market.clockAt.Add(sampleAt.Sub(market.sampleAt))
	delay := time.Until(deliveryAt)

	if delay <= 0 {
		return
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-market.ctx.Done():
	case <-timer.C:
	}
}

func (market *Market) waitForBook(sample testtypes.Sample) {
	if market.stack == nil {
		return
	}

	var observedBid float64
	var observedAsk float64
	var bidLevels int
	var askLevels int

	timeout := time.NewTimer(market.Config.BookApplyTimeout)
	defer timeout.Stop()
	poll := time.NewTicker(market.Config.BookPollInterval)
	defer poll.Stop()

	for {
		matched := false
		market.private.Book(sample.Symbol, func(liveBook *spotbook.Book) {
			bidLevels = len(liveBook.Bids.Levels)
			askLevels = len(liveBook.Asks.Levels)
			bid := liveBook.BestBid()
			ask := liveBook.BestAsk()

			if bid != nil {
				observedBid = bid.Price.Float64()
			}

			if ask != nil {
				observedAsk = ask.Price.Float64()
			}

			if bid != nil && ask != nil &&
				bid.Price.Float64() == sample.Bid &&
				ask.Price.Float64() == sample.Ask &&
				!bid.Timestamp.Before(sample.Timestamp) &&
				!ask.Timestamp.Before(sample.Timestamp) {
				matched = true
			}
		})

		if matched {
			return
		}

		select {
		case <-market.ctx.Done():
			return
		case <-poll.C:
		case <-timeout.C:
			panic(fmt.Errorf(
				"market: live book did not reach %s at %s: want %g/%g, got %g/%g across %d/%d levels",
				sample.Symbol, sample.Timestamp, sample.Bid, sample.Ask,
				observedBid, observedAsk, bidLevels, askLevels,
			))
		}
	}
}
