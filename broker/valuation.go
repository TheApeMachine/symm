package broker

import (
	"math"
	"math/big"
	"strconv"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

/*
Valuation owns exact open-position accounting and adaptive stop state.
*/
type Valuation struct {
	previous   *decimal.Decimal
	sumSquares big.Rat
	count      int64
}

/*
Update applies one executable quote to an open long position.
*/
func (valuation *Valuation) Update(
	data *PositionData,
	stop *StopData,
	ticker kraken.TickerData,
) (*StopData, bool, error) {
	if data == nil || ticker.Symbol != data.Symbol {
		return stop, false, errnie.Err(
			errnie.Validation, "position ticker identity mismatch", nil,
		)
	}

	if ticker.Bid == nil || ticker.Ask == nil || data.FeeRate == nil {
		return stop, false, errnie.Err(
			errnie.Validation, "position valuation inputs required", nil,
		)
	}

	if ticker.Bid.Sign() <= 0 || ticker.Ask.Sign() <= 0 ||
		data.EntryPrice.Sign() <= 0 || data.Qty.Sign() <= 0 {
		return stop, false, errnie.Err(
			errnie.Validation, "position valuation inputs invalid", nil,
		)
	}

	net, entryNotional := valuation.pnl(data, *ticker.Bid)
	pnl := valuation.fromRat(net)

	data.Mark = *ticker.Bid
	data.PnL = *pnl
	data.ReturnPct, _ = new(big.Rat).Mul(
		new(big.Rat).Quo(net, entryNotional),
		big.NewRat(100, 1),
	).Float64()

	return valuation.stop(data, stop, ticker)
}

func (valuation *Valuation) pnl(
	data *PositionData, bid decimal.Decimal,
) (*big.Rat, *big.Rat) {
	entryNotional := new(big.Rat).Mul(
		data.EntryPrice.Rat(), data.Qty.Rat(),
	)
	exitNotional := new(big.Rat).Mul(bid.Rat(), data.Qty.Rat())
	gross := new(big.Rat).Sub(exitNotional, entryNotional)
	feeRate := new(big.Rat).Quo(
		data.FeeRate.Rat(), big.NewRat(100, 1),
	)
	entryFee := new(big.Rat).Mul(entryNotional, feeRate)
	exitFee := new(big.Rat).Mul(exitNotional, feeRate)

	return new(big.Rat).Sub(
		gross, new(big.Rat).Add(entryFee, exitFee),
	), entryNotional
}

func (valuation *Valuation) stop(
	data *PositionData,
	stop *StopData,
	ticker kraken.TickerData,
) (*StopData, bool, error) {
	dispersion, err := valuation.observe(*ticker.Bid)

	if err != nil {
		return stop, false, err
	}

	feeRate := new(big.Rat).Quo(
		data.FeeRate.Rat(), big.NewRat(100, 1),
	)
	spread := new(big.Rat).Quo(
		new(big.Rat).Sub(ticker.Ask.Rat(), ticker.Bid.Rat()),
		ticker.Bid.Rat(),
	)
	offset := new(big.Rat).Add(
		spread, new(big.Rat).Mul(feeRate, big.NewRat(2, 1)),
	)

	if dispersion.Cmp(offset) > 0 {
		offset = dispersion
	}

	if stop == nil {
		stop = &StopData{Symbol: data.Symbol}
	}

	if !stop.Armed || ticker.Bid.Rat().Cmp(stop.PeakPrice.Rat()) > 0 {
		stop.PeakPrice = *ticker.Bid
	}

	candidateRat := new(big.Rat).Mul(
		stop.PeakPrice.Rat(),
		new(big.Rat).Sub(big.NewRat(1, 1), offset),
	)
	candidate := valuation.fromRat(candidateRat)

	if !stop.Armed || candidate.Rat().Cmp(stop.StopPrice.Rat()) > 0 {
		stop.StopPrice = *candidate
	}

	stop.Armed = true
	stop.PeakReturn = valuation.returnOf(stop.PeakPrice, data.EntryPrice)
	stop.StopReturn = valuation.returnOf(stop.StopPrice, data.EntryPrice)

	return stop, ticker.Bid.Rat().Cmp(stop.StopPrice.Rat()) <= 0, nil
}

func (valuation *Valuation) observe(mark decimal.Decimal) (*big.Rat, error) {
	if valuation.previous != nil {
		observedReturn := new(big.Rat).Quo(
			new(big.Rat).Sub(mark.Rat(), valuation.previous.Rat()),
			valuation.previous.Rat(),
		)
		valuation.sumSquares.Add(
			&valuation.sumSquares,
			new(big.Rat).Mul(observedReturn, observedReturn),
		)
		valuation.count++
	}

	previous := mark
	valuation.previous = &previous

	if valuation.count == 0 {
		return new(big.Rat), nil
	}

	meanSquare := new(big.Rat).Quo(
		&valuation.sumSquares, big.NewRat(valuation.count, 1),
	)
	meanSquareFloat, _ := meanSquare.Float64()
	dispersion, err := decimal.NewFromString(strconv.FormatFloat(
		math.Sqrt(meanSquareFloat), 'f', -1, 64,
	))

	if err != nil {
		return nil, errnie.Err(
			errnie.Validation, "position dispersion is not representable", err,
		)
	}

	return dispersion.Rat(), nil
}

func (valuation *Valuation) returnOf(
	price decimal.Decimal, entry decimal.Decimal,
) float64 {
	value, _ := new(big.Rat).Quo(
		new(big.Rat).Sub(price.Rat(), entry.Rat()),
		entry.Rat(),
	).Float64()

	return value
}

/*
fromRat converts an exact rational to a Decimal through its nearest
float64, the same round-trip-safe precision decimal.NewFromFloat64 already
relies on for observed dispersion below. big.Rat has no direct decimal
string form for non-terminating fractions, so this is the only conversion
that neither truncates precision nor invents a scale.
*/
func (valuation *Valuation) fromRat(rat *big.Rat) *decimal.Decimal {
	value, _ := rat.Float64()
	return decimal.NewFromFloat64(value)
}
