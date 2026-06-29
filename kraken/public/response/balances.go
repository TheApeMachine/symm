package response

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

func (balances *Balances) Send(artifact *datura.Artifact) *datura.Artifact {
	method := datura.Peek[string](artifact, "method")
	var out *datura.Artifact
	publish := false

	switch method {
	case "subscribe":
		errnie.Info("subscribing to balances")
		balances.isActive.Store(true)
		publish = true
		out = datura.Acquire(
			"kraken:private", datura.APPJSON,
		).WithRole(
			"balances",
		).WithScope(
			"snapshot",
		).WithPayload(
			balances.model.DecryptPayload(),
		)
	case "unsubscribe":
		errnie.Info("unsubscribing from balances")
		balances.isActive.Store(false)
		out = datura.Acquire(
			"kraken:private", datura.APPJSON,
		).WithRole(
			"balances",
		).WithScope(
			"unsubscribe",
		).WithPayload(datura.Map[any]{
			"method":   "unsubscribe",
			"success":  true,
			"time_in":  time.Now(),
			"time_out": time.Now(),
		}.Marshal())
	case "add_order":
		errnie.Info("adding order to balances")

		balance := datura.Peek[float64](balances.model, "data", 0, "balance")
		price := datura.Peek[float64](artifact, "params", "limit_price")

		if balance-price < 0 {
			out = datura.Acquire(
				"kraken:private", datura.APPJSON,
			).WithRole(
				"balances",
			).WithScope(
				"error",
			).WithPayload(datura.Map[any]{
				"error":    "EOrder:Insufficient funds",
				"method":   "add_order",
				"req_id":   123456789,
				"success":  false,
				"time_in":  time.Now(),
				"time_out": time.Now(),
			}.Marshal())

			break
		}

		balances.model.PokePayload(
			balance-price, "data", 0, "balance",
		)

		publish = true
		out = datura.Acquire(
			"kraken:private", datura.APPJSON,
		).WithRole(
			"balances",
		).WithScope(
			"update",
		).WithPayload(
			balances.model.DecryptPayload(),
		)
	default:
		return nil
	}

	if publish {
		balances.observers.Range(func(_ any, value any) bool {
			value.(types.Socket).Send(out)
			return true
		})
	}

	return out
}

func (balances *Balances) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		balances.observers.Store(uuid.NewString(), socket)
	}
}
