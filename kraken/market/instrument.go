package market

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
InstrumentParams is the Kraken WebSocket v2 subscribe payload for the instrument channel.
*/
type InstrumentParams struct {
	Channel  string `json:"channel"`
	Snapshot bool   `json:"snapshot"`
}

/*
InstrumentAsset is one tradable asset's precision and margin metadata from the instrument feed.
*/
type InstrumentAsset struct {
	ID               string  `json:"id"`
	Status           string  `json:"status"`
	Precision        int     `json:"precision"`
	PrecisionDisplay int     `json:"precision_display"`
	Borrowable       bool    `json:"borrowable"`
	CollateralValue  float64 `json:"collateral_value"`
	MarginRate       float64 `json:"margin_rate"`
}

/*
InstrumentPair is one market's sizing, status, and increment rules from the instrument feed.
*/
type InstrumentPair struct {
	Symbol             string  `json:"symbol"`
	Base               string  `json:"base"`
	Quote              string  `json:"quote"`
	Status             string  `json:"status"`
	QtyPrecision       int     `json:"qty_precision"`
	QtyIncrement       float64 `json:"qty_increment"`
	PricePrecision     int     `json:"price_precision"`
	CostPrecision      int     `json:"cost_precision"`
	Marginable         bool    `json:"marginable"`
	HasIndex           bool    `json:"has_index"`
	CostMin            float64 `json:"cost_min"`
	MarginInitial      float64 `json:"margin_initial,omitempty"`
	PositionLimitLong  int     `json:"position_limit_long,omitempty"`
	PositionLimitShort int     `json:"position_limit_short,omitempty"`
	PriceIncrement     float64 `json:"price_increment"`
	QtyMin             float64 `json:"qty_min"`
}

/*
InstrumentUpdate is the instrument channel snapshot: the tradable asset and pair catalog.

The exchange's complete tradable catalog pushed live: every asset's precision and
margin terms, and every pair's status, sizing increments, minimums, and limits.
It is the authoritative, self-updating definition of what is currently tradable
and the exact rules for sizing and rounding an order, reflecting halts and
precision changes the moment they happen.
*/
type InstrumentUpdate struct {
	Assets []InstrumentAsset `json:"assets"`
	Pairs  []InstrumentPair  `json:"pairs"`
}

/*
NewInstrumentSubscription opens the instrument channel and forwards snapshots to recv.
It blocks until ctx is canceled or the socket closes.
*/
func NewInstrumentSubscription(
	ctx context.Context,
	recv chan *InstrumentUpdate,
) {
	ws := errnie.Does(func() (*public.WebSocket, error) {
		return public.NewWebSocket(ctx)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	messages := errnie.Does(func() (chan *public.SocketMessage, error) {
		return ws.Generate(public.InstrumentsChannel)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	for message := range messages {
		var update InstrumentUpdate

		if err := sonic.Unmarshal(message.Data, &update); err != nil {
			continue
		}

		copied := update
		recv <- &copied
	}
}
