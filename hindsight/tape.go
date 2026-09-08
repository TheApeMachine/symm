package hindsight

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/store"
	"gocloud.dev/blob"
)

/*
	Leg is an observed directional run of spot trades, closed only when the

next different trade reverses direction. It describes tape geometry, not an
execution guarantee. Constant prices do not invent a completed opportunity.
*/
type Leg struct {
	Symbol                     string
	From, Through, ConfirmedAt time.Time
	Start, End                 *decimal.Decimal
}

type tapePoint struct {
	at        time.Time
	price     *decimal.Decimal
	direction int
	start     time.Time
	initial   *decimal.Decimal
}

/*
	Tape consumes each durable capture object once, in recording order. It owns

only the current leg per symbol; the raw historical record remains in S3.
*/
type Tape struct {
	LastKey string
	points  map[string]*tapePoint
}

/*
	Read delivers completed legs from newly persisted spot trade captures.

A failed read or visitor stops advancement and is returned to the owner.
*/
func (tape *Tape) Read(ctx context.Context, archive *blob.Bucket, run RunID, visit func(Leg) error) error {
	objects := archive.List(&blob.ListOptions{Prefix: run.Prefix("captures")})

	for {
		object, err := objects.Next(ctx)

		if err == io.EOF {
			return nil
		}

		if err != nil {
			return errnie.Error(errnie.Err(errnie.IO, "tape: list captures", err))
		}

		if object.Key <= tape.LastKey {
			continue
		}
		payload, err := store.ReadAll(ctx, archive, object.Key)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.IO, "tape: read captures", err))
		}
		decoder := json.NewDecoder(bytes.NewReader(payload))

		for {
			var frame RawFrame
			err := decoder.Decode(&frame)

			if err == io.EOF {
				break
			}

			if err != nil {
				return errnie.Error(errnie.Err(errnie.Validation, "tape: decode capture", err))
			}

			if err := tape.Step(frame, visit); err != nil {
				return err
			}
		}
		tape.LastKey = object.Key
	}
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

func (tape *Tape) observe(symbol string, at time.Time, price *decimal.Decimal, visit func(Leg) error) error {
	point := tape.points[symbol]

	if point == nil {
		tape.points[symbol] = &tapePoint{at: at, start: at, price: price, initial: price}
		return nil
	}
	direction := price.Cmp(point.price)

	if direction == 0 {
		return nil
	}

	if point.direction != 0 && direction != point.direction {
		leg := Leg{Symbol: symbol, From: point.start, Through: point.at, ConfirmedAt: at, Start: point.initial, End: point.price}

		if err := visit(leg); err != nil {
			return errnie.Error(err)
		}
		point.start, point.initial = point.at, point.price
	}
	point.at, point.price, point.direction = at, price, direction
	return nil
}
