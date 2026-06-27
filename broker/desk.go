package broker

import (
	"context"
	"slices"
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

/*
Desk is the link between the trader and the Kraken exchange. It opens and closes
positions on the trader's command and protects them with trailing stops. It makes
no entry decisions of its own; the only call it makes alone is bailing out of a
position whose stop has been breached. All state lives in the shared tree.
*/
type Desk struct {
	ctx        context.Context
	cancel     context.CancelFunc
	pool       *qpool.Q[any]
	tree       *dmt.Tree
	privateBus *qpool.BroadcastGroup
	quote      string
}

func NewDesk(
	ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree,
) *Desk {
	ctx, cancel := context.WithCancel(ctx)

	return &Desk{
		ctx:        ctx,
		cancel:     cancel,
		pool:       pool,
		tree:       tree,
		privateBus: pool.CreateBroadcastGroup("kraken:private"),
		quote:      strings.ToUpper(viper.GetString("market.quote_currency")),
	}
}

/*
Update ratchets every open stop against the latest marks, then dispatches the
actions the trader chose this tick. Ratcheting runs every tick, with or without
new actions, so protection never lapses.
*/
func (desk *Desk) Update(actions []*datura.Artifact) error {
	desk.ratchet()

	for _, action := range actions {
		errnie.Error(desk.execute(action))
	}

	return nil
}

/*
execute turns one trader action into an exchange order. A buy opens a position
and arms its trailing stop; a sell closes the position and retires the stop.
*/
func (desk *Desk) execute(action *datura.Artifact) error {
	if action == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "desk: nil action", nil))
	}

	symbol, _ := action.Scope()
	side, _ := action.Role()

	if symbol == "" || !slices.Contains([]string{"buy", "sell"}, side) {
		return errnie.Error(errnie.Err(
			errnie.Validation, "desk: action missing symbol or side: "+symbol+"/"+side, nil,
		))
	}

	orderType := datura.Peek[string](action, "type")
	qty := datura.Peek[float64](action, "quantity")
	clOrdID := datura.Peek[string](action, "cl_ord_id")

	// The trader sizes entries by risk fraction; the desk turns that into a
	// quantity here, where the live mark and free quote balance are known. An
	// explicit quantity (e.g. on exits) is used as-is.
	if side == "buy" {
		if fraction := datura.Peek[float64](action, "fraction"); fraction > 0 {
			qty = desk.sizeBuy(symbol, fraction)
		}
	} else if qty > 0 {
		// Exits carry an explicit quantity; round it to the exchange increment
		// so the sell is not rejected for sub-increment precision. No minimum
		// guard — an exit must always be able to flatten the position.
		qty = desk.roundQuantity(symbol, qty)
	}

	if qty <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation, "desk: non-positive quantity for "+symbol, nil,
		))
	}

	if err := desk.send(symbol, side, orderType, qty, clOrdID); err != nil {
		return err
	}

	if side == "sell" {
		desk.retireStop(symbol)

		return nil
	}

	return desk.armStop(action, symbol, qty)
}

/*
armStop records a trailing stop for a freshly opened long, priced from the live
mark in the tree and the offset the trader put on the action.
*/
func (desk *Desk) armStop(action *datura.Artifact, symbol string, qty float64) error {
	offset := datura.Peek[float64](action, "offset")

	if offset <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation, "desk: entry for "+symbol+" has no stop offset", nil,
		))
	}

	mark := desk.markFor(symbol)

	if mark <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation, "desk: no mark to price stop for "+symbol, nil,
		))
	}

	desk.store(NewStoploss(symbol, qty, mark, offset))

	return nil
}

/*
ratchet walks every stored stop, trails it against the latest mark, and exits the
position the moment a stop is breached.
*/
func (desk *Desk) ratchet() {
	for artifact := range desk.tree.Seek([]byte("stoploss/")) {
		stoploss := StoplossFromArtifact(artifact)
		artifact.Release()

		if stoploss.Qty <= 0 {
			continue
		}

		mark := desk.markFor(stoploss.Symbol)

		if mark <= 0 {
			errnie.Error(errnie.Err(
				errnie.Validation, "desk: no mark to ratchet stop for "+stoploss.Symbol, nil,
			))

			continue
		}

		if stoploss.Ratchet(mark) {
			errnie.Error(desk.send(stoploss.Symbol, stoploss.Side, "market", stoploss.Qty, ""))
			stoploss.Qty = 0
		}

		desk.store(stoploss)
	}
}

/*
retireStop closes out a stop after the trader exits the position by hand.
*/
func (desk *Desk) retireStop(symbol string) {
	for artifact := range desk.tree.Seek([]byte("stoploss/" + symbol)) {
		stoploss := StoplossFromArtifact(artifact)
		artifact.Release()
		stoploss.Qty = 0
		desk.store(stoploss)
	}
}

/*
store overwrites the stop at a stable per-symbol key; a zero quantity tombstones
it. The key is deterministic (no uuid/timestamp) so updates replace in place.
*/
func (desk *Desk) store(stoploss *Stoploss) {
	desk.tree.InsertArtifact(stoplossKey(stoploss.Symbol), stoploss.Artifact())
}

func stoplossKey(symbol string) []byte {
	return []byte("stoploss/" + symbol)
}

/*
send emits an add_order to the private bus, retrying a few times before giving up
and logging the failure.
*/
func (desk *Desk) send(
	symbol, side, orderType string, qty float64, clOrdID string,
) error {
	order := datura.Acquire(
		"broker", datura.APPJSON,
	).WithDestination(
		"kraken:private",
	).WithRole(
		"orders",
	).WithPayload(datura.Map[any]{
		"method": "add_order",
		"params": datura.Map[any]{
			"symbol":     symbol,
			"side":       side,
			"order_type": orderType,
			"order_qty":  qty,
			"cl_ord_id":  clOrdID,
		},
	}.Marshal())

	var sendErr error

	for range 3 {
		if err := desk.privateBus.Send(order); err == nil {
			return nil
		} else {
			sendErr = err
		}
	}

	return errnie.Error(errnie.Err(
		errnie.Validation, "desk: failed to send order for "+symbol, sendErr,
	))
}

/*
markFor reads the freshest ticker mid for the symbol straight from the shared
tree, the same source the paper fill simulator prices against.
*/
func (desk *Desk) markFor(symbol string) float64 {
	var (
		latestMark  float64
		latestStamp int64
	)

	for candidate := range desk.tree.Seek([]byte("ticker/")) {
		if role, err := candidate.Role(); err != nil || role != "ticker" {
			candidate.Release()
			break
		}

		mark := tickerMark(candidate, symbol)
		if mark > 0 && candidate.Timestamp() >= latestStamp {
			latestMark = mark
			latestStamp = candidate.Timestamp()
		}

		candidate.Release()
	}

	return latestMark
}

func tickerMark(ticker *datura.Artifact, symbol string) float64 {
	for rowIndex := 0; ; rowIndex++ {
		rowSymbol := datura.Peek[string](ticker, "data", rowIndex, "symbol")

		if rowSymbol == "" {
			break
		}

		if rowSymbol != symbol {
			continue
		}

		if last := datura.Peek[float64](ticker, "data", rowIndex, "last"); last > 0 {
			return last
		}

		bid := datura.Peek[float64](ticker, "data", rowIndex, "bid")
		ask := datura.Peek[float64](ticker, "data", rowIndex, "ask")

		if bid > 0 && ask > 0 {
			return (bid + ask) / 2
		}

		return 0
	}

	scope := errnie.Does(func() (string, error) {
		return ticker.Scope()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation, "desk: failed to get ticker scope", err,
		))
	}).Value()

	if scope != symbol {
		return 0
	}

	if last := datura.Peek[float64](ticker, "data", 0, "last"); last > 0 {
		return last
	}

	bid := datura.Peek[float64](ticker, "data", 0, "bid")
	ask := datura.Peek[float64](ticker, "data", 0, "ask")

	if bid > 0 && ask > 0 {
		return (bid + ask) / 2
	}

	return 0
}

func (desk *Desk) Close() error {
	desk.cancel()
	return nil
}
