package tests

import (
	"iter"

	"github.com/theapemachine/errnie"
	bookfixture "github.com/theapemachine/symm/tests/fixtures/book"
	level3fixture "github.com/theapemachine/symm/tests/fixtures/level3"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	tradefixture "github.com/theapemachine/symm/tests/fixtures/trade"
)

/*
frameSet holds one coherent observation from every simulated Kraken feed.
*/
type frameSet struct {
	ticker []byte
	trade  []byte
	book   []byte
	level3 []byte
}

/*
current renders one full snapshot of the present authoritative state for a new
or reconnecting subscription without replaying prior updates.
*/
func (market *Market) current() frameSet {
	frames, err := market.read(
		tickerfixture.NewMarket(market.Symbols, market.signal),
		tradefixture.NewMarket(market.Symbols, market.signal),
		bookfixture.NewMarket(market.Symbols, market.signal),
		level3fixture.NewMarket(market.Symbols, market.signal),
	)

	if err != nil {
		panic(err)
	}

	return frames
}

/*
read consumes exactly one ready payload from each coordinated fixture.
*/
func (market *Market) read(
	ticker *tickerfixture.Fixture,
	trade *tradefixture.Fixture,
	book *bookfixture.Fixture,
	level3 *level3fixture.Fixture,
) (frameSet, error) {
	tickerNext, tickerStop := iter.Pull(ticker.Generate())
	tradeNext, tradeStop := iter.Pull(trade.Generate())
	bookNext, bookStop := iter.Pull(book.Generate())
	level3Next, level3Stop := iter.Pull(level3.Generate())
	defer tickerStop()
	defer tradeStop()
	defer bookStop()
	defer level3Stop()

	tickerPayload, tickerMore := tickerNext()
	tradePayload, tradeMore := tradeNext()
	bookPayload, bookMore := bookNext()
	level3Payload, level3More := level3Next()

	if !tickerMore || !tradeMore || !bookMore || !level3More {
		return frameSet{}, errnie.Err(errnie.Validation, "tests: bootstrap frame missing", nil)
	}

	_, tickerExtra := tickerNext()
	_, tradeExtra := tradeNext()
	_, bookExtra := bookNext()
	_, level3Extra := level3Next()

	if tickerExtra || tradeExtra || bookExtra || level3Extra {
		return frameSet{}, errnie.Err(errnie.Validation, "tests: fixture frame count differs", nil)
	}

	return frameSet{
		ticker: tickerPayload,
		trade:  tradePayload,
		book:   bookPayload,
		level3: level3Payload,
	}, nil
}
