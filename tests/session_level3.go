package tests

import (
	"context"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
)

/*
SessionLevel3 wires harness Level3 books through the websocket API so toxicity
and PeekBook signals can run without a live L3 socket.
*/
type SessionLevel3 struct {
	live *websocket.Live
}

/*
NewSessionLevel3 attaches an SDK-managed book transport when Level3 is enabled.
*/
func NewSessionLevel3(
	ctx context.Context,
	api *websocket.API,
) *SessionLevel3 {
	level3 := websocket.New(ctx, nil, true, websocket.Level3WebSocketURL)
	api.AttachLevel3(level3)

	return &SessionLevel3{live: level3}
}

/*
Enabled reports whether PeekBook-backed books are attached.
*/
func (level3 *SessionLevel3) Enabled() bool {
	return level3 != nil && level3.live != nil
}

/*
Apply feeds one raw Level3 websocket payload through the write lease.
*/
func (level3 *SessionLevel3) Apply(payload []byte) error {
	if level3 == nil || !level3.Enabled() {
		return errnie.Err(
			errnie.Validation,
			"tests: level3 books are unavailable",
			nil,
		)
	}

	return level3.live.ApplyLevel3(payload)
}

/*
SeedTouchDecimals installs a two-sided L3 quote using exact decimal prices.
*/
func (level3 *SessionLevel3) SeedTouchDecimals(
	symbol string,
	bid *decimal.Decimal,
	ask *decimal.Decimal,
	quantity float64,
	at time.Time,
) error {
	if level3 == nil || !level3.Enabled() {
		return errnie.Err(
			errnie.Validation,
			"tests: level3 books are unavailable",
			nil,
		)
	}

	level3.live.SeedTouchDecimals(symbol, bid, ask, quantity, at)

	return nil
}

/*
SeedTouch installs a two-sided L3 quote for toxicity Session tests.
*/
func (level3 *SessionLevel3) SeedTouch(
	symbol string,
	bid float64,
	ask float64,
	quantity float64,
	at time.Time,
) error {
	if level3 == nil || !level3.Enabled() {
		return errnie.Err(
			errnie.Validation,
			"tests: level3 books are unavailable",
			nil,
		)
	}

	level3.live.SeedTouch(symbol, bid, ask, quantity, at)

	return nil
}
