package broker

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

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
position whose stop has been breached. Stop logic lives on Stoploss; Desk only
owns the live stop map and forwards resulting orders to Kraken.
*/
type Desk struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q[any]
	tree        *dmt.Tree
	broadcasts  *sync.Map
	orders      *sync.Map
	stoplosses  *sync.Map
	subscribers []*qpool.BroadcastConsumer
	balances    *datura.Artifact
	quote       string
	closed      atomic.Bool
}

func NewDesk(
	ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree,
) *Desk {
	ctx, cancel := context.WithCancel(ctx)

	desk := &Desk{
		ctx:        ctx,
		cancel:     cancel,
		pool:       pool,
		tree:       tree,
		broadcasts: &sync.Map{},
		orders:     &sync.Map{},
		stoplosses: &sync.Map{},
		quote: strings.ToUpper(
			viper.GetString("market.quote_currency"),
		),
	}

	for _, channel := range []string{"kraken:private"} {
		desk.broadcasts.Store(channel, pool.CreateBroadcastGroup(channel))
	}

	for _, channel := range []string{"ticker", "executions", "balances"} {
		desk.subscribers = append(
			desk.subscribers, pool.Subscribe(channel, desk.onMessage),
		)
	}

	return desk
}

/*
Update converts each chosen action into a Kraken order request and sends it
to the kraken:private channel.
*/
func (desk *Desk) Update(
	chosen []*datura.Artifact,
) error {
	for _, action := range chosen {
		symbol, err := action.Scope()

		if err != nil || symbol == "" {
			continue
		}

		actionType := datura.Peek[string](action, "type")
		side := datura.Peek[string](action, "side")
		qty := datura.Peek[float64](action, "quantity")
		// price := datura.Peek[float64](action, "price")

		// Translate action type to Kraken order type
		orderType := "market"

		switch actionType {
		case "limit":
			orderType = "limit"
		case "settle_position":
			orderType = "market"
		default:
			errnie.Error(errnie.Err(
				errnie.Validation,
				"unknown order type from action",
				nil,
			))

			continue
		}

		order := datura.Acquire(
			"broker", datura.APPJSON,
		).WithDestination(
			"kraken:private",
		).WithRole(
			"orders",
		).WithScope(
			symbol,
		)

		uuid, err := order.Uuid()

		if err != nil {
			errnie.Error(err)
			continue
		}

		order.WithPayload(datura.Map[any]{
			"method": "add_order",
			"params": datura.Map[any]{
				"symbol":     symbol,
				"side":       side,
				"order_type": orderType,
				"order_qty":  qty,
				"cl_ord_id":  uuid,
			},
		}.Marshal())

		desk.orders.Store(uuid, order)
		desk.stoplosses.Store(symbol, NewStoploss(order, symbol))

		bg, _ := desk.broadcasts.Load("kraken:private")
		bg.(*qpool.BroadcastGroup).Send(order)
	}

	return nil
}

/*
onMessage is called by the qpool.BroadcastGroup for every consumer
that has subscribed with a callback function.
*/
func (desk *Desk) onMessage(
	artifact *datura.Artifact,
) error {
	role := datura.Peek[string](artifact, "role")
	symbol := datura.Peek[string](artifact, "scope")

	switch role {
	case "ticker":
		data := datura.Peek[[]map[string]any](artifact, "data")

		for _, update := range data {
			symbol := update["symbol"].(string)
			last := update["last"].(float64)

			stoploss, ok := desk.stoplosses.Load(symbol)

			if ok {
				stoploss = stoploss.(*Stoploss).Ratchet(last)

				if stoploss.(*Stoploss).State == EXITING {

				}
			}
		}
	case "balances":
		desk.balances = artifact
	case "executions":
		status := datura.Peek[string](artifact, "order_status")
		if status == "filled" {
			price := datura.Peek[float64](artifact, "last_price")
			side := datura.Peek[string](artifact, "side")

			switch side {
			case "buy":
				offset := viper.GetFloat64("trading.stop.trailing_offset_bps") / 10000.0

				if offset <= 0 {
					offset = 0.01
				}

				stoploss, ok := desk.stoplosses.Load(symbol)

				if ok {
					stoploss.(*Stoploss).Ratchet(price)
				}
			case "sell":
				stoploss, ok := desk.stoplosses.Load(symbol)

				if ok {
					stoploss.(*Stoploss).Close()
					desk.stoplosses.Delete(symbol)
				}
			}
		}
	}

	return nil
}

func (desk *Desk) Close() error {
	desk.closed.Store(true)
	desk.cancel()
	return nil
}
