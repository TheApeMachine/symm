package market

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
)

/*
Input is one typed market-data update from Kraken.
It is the signal input contract; it is not a transport envelope.
*/
type Input struct {
	Role   string
	At     time.Time
	Ticker kraken.TickerDataSlice
	Trade  kraken.TradeDataSlice
	OHLC   kraken.OHLCDataSlice
	Book   kraken.BookDataSlice
	Level3 kraken.Level3DataSlice
}

/*
Signal is a mechanism to structure raw market data into
measurements, which are labeled as semantic categories.
*/
type Signal interface {
	Measure(Input, *CrossSection) ([]*logic.Measurement, error)
	IngestRoles() []string
	Close() error
}

/*
NewInput decodes a websocket payload into the typed market rows for its role.
*/
func NewInput(role string, payload []byte, at time.Time) (Input, error) {
	input := Input{
		Role: role,
		At:   at,
	}

	if role == "" {
		return Input{}, errnie.Err(errnie.Validation, "market: input role required", nil)
	}

	if err := input.Decode(payload); err != nil {
		return Input{}, err
	}

	if input.At.IsZero() {
		input.At = input.Latest()
	}

	return input, nil
}

/*
Decode writes the payload into the role-specific typed rows.
*/
func (input *Input) Decode(payload []byte) error {
	switch input.Role {
	case "ticker":
		if err := sonic.Unmarshal(payload, &input.Ticker); err != nil {
			return errnie.Err(errnie.Validation, "market: decode ticker input", err)
		}

		return nil
	case "trade":
		if err := sonic.Unmarshal(payload, &input.Trade); err != nil {
			return errnie.Err(errnie.Validation, "market: decode trade input", err)
		}

		return nil
	case "ohlc":
		if err := sonic.Unmarshal(payload, &input.OHLC); err != nil {
			return errnie.Err(errnie.Validation, "market: decode ohlc input", err)
		}

		return nil
	case "book":
		if err := input.Book.Decode(payload); err != nil {
			return errnie.Err(errnie.Validation, "market: decode book input", err)
		}

		return nil
	case "level3":
		if err := sonic.Unmarshal(payload, &input.Level3); err != nil {
			return errnie.Err(errnie.Validation, "market: decode level3 input", err)
		}

		return nil
	default:
		return errnie.Err(errnie.Validation, "market: unsupported input role "+input.Role, nil)
	}
}

/*
Latest returns the most recent timestamp in the typed input.
*/
func (input Input) Latest() time.Time {
	latest := time.Time{}

	for _, row := range input.Ticker {
		if row.Timestamp.After(latest) {
			latest = row.Timestamp
		}
	}

	for _, row := range input.Trade {
		if row.Timestamp.After(latest) {
			latest = row.Timestamp
		}
	}

	for _, row := range input.OHLC {
		if row.Timestamp.After(latest) {
			latest = row.Timestamp
		}
	}

	for _, row := range input.Book {
		if row.Timestamp.After(latest) {
			latest = row.Timestamp
		}
	}

	for _, row := range input.Level3 {
		if row.Timestamp.After(latest) {
			latest = row.Timestamp
		}
	}

	return latest
}
