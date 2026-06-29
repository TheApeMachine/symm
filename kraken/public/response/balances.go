package response

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/types"
)

/*
Balances simulates the Kraken balances channel on the shared raw bus.
*/
type Balances struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	isActive      atomic.Bool
	observers     *sync.Map
	quoteCurrency string
	model         *datura.Artifact
}

func NewBalances(
	ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree,
) *Balances {
	ctx, cancel := context.WithCancel(ctx)
	quote := strings.ToUpper(viper.GetString("market.quote_currency"))
	balance := viper.GetFloat64("trading.paper.wallet." + strings.ToLower(quote))

	return &Balances{
		ctx:           ctx,
		cancel:        cancel,
		observers:     &sync.Map{},
		quoteCurrency: quote,
		model: datura.Acquire(
			"kraken:private", datura.APPJSON,
		).WithPayload(datura.Map[any]{
			"channel": "balances",
			"type":    "snapshot",
			"data": []map[string]any{{
				"asset":       quote,
				"asset_class": "currency",
				"balance":     balance,
				"wallets": []map[string]any{{
					"balance": balance,
					"type":    "spot",
					"id":      "main",
				}},
			}},
		}.Marshal()),
	}
}

func (balances *Balances) Send(message []byte) *types.SocketMessage {
	incoming := types.SocketMessage{}

	out := types.SocketMessage{
		Channel: "balances",
		Type:    "update",
		Method:  "",
		TimeIn:  time.Now(),
	}

	if err := sonic.Unmarshal(message, &incoming); err != nil {
		errnie.Error(err)
		return nil
	}

	switch incoming.Method {
	case "subscribe":
		errnie.Info("subscribing to balances")
		balances.isActive.Store(true)
		out.Type = "snapshot"
		out.Data = balances.model.DecryptPayload()
	case "unsubscribe":
		errnie.Info("unsubscribing from balances")
		balances.isActive.Store(false)
	case "add_order":
		errnie.Info("adding order to balances")

		balance := datura.Peek[float64](balances.model, "data", 0, "balance")

		data := map[string]any{}
		sonic.Unmarshal(incoming.Data, &data)

		price := data["params"].(map[string]any)["limit_price"].(float64)

		if balance-price < 0 {
			out.Data = datura.Map[any]{
				"error":    "EOrder:Insufficient funds",
				"method":   "add_order",
				"req_id":   123456789,
				"success":  false,
				"time_in":  time.Now(),
				"time_out": time.Now(),
			}.Marshal()

			break
		}

		balances.model.PokePayload(
			balance-price, "data", 0, "balance",
		)

		out.Data = balances.model.DecryptPayload()
	default:
		out.TimeOut = time.Now()
		return nil
	}

	balances.observers.Range(func(_ any, value any) bool {
		return true
	})

	out.TimeOut = time.Now()
	return &out
}

func (balances *Balances) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		balances.observers.Store(uuid.NewString(), socket)
	}
}
