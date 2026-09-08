package hindsight

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
)

/*
	Leg is an observed directional run of spot trades, running from its origin

to the extremum it reached, and confirmed only once price retraces away from
that extremum. It describes tape geometry, not an execution guarantee. Constant
prices do not invent a completed opportunity.

Start and End are the origin and the extremum, so a leg reports the whole
excursion that was available. Through is when the extremum printed and
ConfirmedAt is when the retracement made it observable; the gap between them is
the geometry's cost, and it is stated rather than hidden.
*/
type Leg struct {
	Symbol                     string
	From, Through, ConfirmedAt time.Time
	Start, End                 *decimal.Decimal
}

/*
DefaultRetraceFraction is the share of a leg's own excursion that price must
give back before the leg counts as finished. It is proportional, not a fixed
percentage of price, for the same reason excursionEpisodes in episode.go is: a
fixed floor is meaningless across instruments of different volatility, and any
floor above an instrument's typical move confirms nothing at all. Half the
excursion is a reversal; anything less is a pullback inside a move that is still
developing.
*/
const DefaultRetraceFraction = 0.5

/*
tapePoint is one symbol's leg in progress: where it began, the extremum it has
reached so far, and the last price seen. direction is the sign of the leg, and
is zero until the first price that differs from the origin establishes it.
*/
type tapePoint struct {
	at         time.Time
	price      *decimal.Decimal
	direction  int
	start      time.Time
	initial    *decimal.Decimal
	extremumAt time.Time
	extremum   *decimal.Decimal
}

/*
	Tape consumes each durable capture object once, in recording order. It owns

only the current leg per symbol; the raw historical record remains in S3.
*/
type Tape struct {
	// LastSequence is the highest capture sequence this tape has consumed.
	// Resumption is a predicate on the sequence column rather than a key
	// comparison, so it survives compaction rewriting the underlying files.
	LastSequence int64
	// Retrace overrides DefaultRetraceFraction when set in (0, 1).
	Retrace float64
	points  map[string]*tapePoint
}

/* retracement reports the configured share of a leg's excursion, or the default. */
func (tape *Tape) retracement() float64 {
	if tape.Retrace > 0 && tape.Retrace < 1 {
		return tape.Retrace
	}

	return DefaultRetraceFraction
}

/*
	Read delivers completed legs from newly persisted spot trade captures.

A failed read or visitor stops advancement and is returned to the owner.
*/
func (tape *Tape) Read(ctx context.Context, catalog *tables.Catalog, run RunID, visit func(Leg) error) error {
	rows, err := catalog.Captures(ctx, string(run), tape.LastSequence)

	if err != nil {
		return err
	}

	for _, row := range rows {
		if err := tape.Step(FrameFromRow(row), visit); err != nil {
			return err
		}

		tape.LastSequence = row.Sequence
	}

	return nil
}

/* Step consumes the original captured decimal trade prices. */
func (tape *Tape) Step(frame RawFrame, visit func(Leg) error) error {
	if frame.Kind != "trade" || strings.Contains(frame.Endpoint, "futures") {
		return nil
	}
	var trades kraken.Trade

	if err := json.Unmarshal(frame.Payload, &trades); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "tape: decode trades", err))
	}

	if tape.points == nil {
		tape.points = make(map[string]*tapePoint)
	}

	for _, trade := range trades.Data {
		if trade.Price.Sign() <= 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "tape: positive trade price required", nil))
		}

		if err := tape.observe(trade.Symbol, frame.ReceivedAt, &trade.Price, visit); err != nil {
			return err
		}
	}
	return nil
}

/*
observe advances one symbol's leg. A leg extends through every price that moves
it further from its origin, and survives every pullback that does not give back
the configured share of the excursion it has accumulated. It closes only on a
retracement of that size, which is what separates a reversal from noise inside a
move still developing — a single opposite print used to end a leg, so a
breakout was cut at its first pullback and graded as the few ticks before it
rather than the whole excursion.

The completed leg reports origin to extremum, and the price that confirmed it
becomes the origin of the next one.
*/
func (tape *Tape) observe(symbol string, at time.Time, price *decimal.Decimal, visit func(Leg) error) error {
	point := tape.points[symbol]

	if point == nil {
		tape.points[symbol] = &tapePoint{
			at: at, start: at, price: price,
			initial: price, extremum: price, extremumAt: at,
		}
		return nil
	}
	point.at, point.price = at, price
	direction := price.Cmp(point.extremum)

	if point.direction == 0 {
		// The leg has no sign yet; the first price away from the origin gives
		// it one, and that price is by construction the extremum so far.
		if direction != 0 {
			point.direction, point.extremum, point.extremumAt = direction, price, at
		}

		return nil
	}

	if direction == point.direction {
		point.extremum, point.extremumAt = price, at
		return nil
	}

	if direction == 0 || !tape.retraced(point, price) {
		return nil
	}
	leg := Leg{
		Symbol: symbol, From: point.start, Through: point.extremumAt, ConfirmedAt: at,
		Start: point.initial, End: point.extremum,
	}

	if err := visit(leg); err != nil {
		return errnie.Error(err)
	}
	// The confirmed extremum is the pivot: the next leg runs from there, in the
	// direction the retracement just established.
	point.start, point.initial = point.extremumAt, point.extremum
	point.direction, point.extremum, point.extremumAt = direction, price, at
	return nil
}

/*
retraced reports whether price has given back the required share of the move
the leg has accumulated. Both sides are measured against the same excursion, so
the fraction reads directly as "half the move handed back". A leg that has not
moved has nothing to retrace and can never confirm on its own.
*/
func (tape *Tape) retraced(point *tapePoint, price *decimal.Decimal) bool {
	origin, extremum := point.initial.Float64(), point.extremum.Float64()
	excursion := math.Abs(extremum - origin)

	if excursion <= 0 {
		return false
	}

	return math.Abs(price.Float64()-extremum)/excursion >= tape.retracement()
}
