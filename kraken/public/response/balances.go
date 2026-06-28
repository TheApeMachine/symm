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
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/types"
)

const (
	balanceSnapshotScope = "snapshot"
	balanceUpdateScope   = "update"
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

	return &Balances{
		ctx:           ctx,
		cancel:        cancel,
		observers:     &sync.Map{},
		quoteCurrency: quote,
		model: datura.Acquire(
			"kraken:private", datura.APPJSON,
		).WithPayload(datura.Map[any]{
			"asset": []map[string]any{{
				"asset":       viper.GetString("market.quote_currency"),
				"asset_class": "currency",
				"balance": viper.GetFloat64(
					"trading.paper.wallet." + strings.ToLower(quote),
				),
				"wallets": []map[string]any{{
					"balance": viper.GetFloat64(
						"trading.paper.wallet." + strings.ToLower(quote),
					),
					"type": "spot",
					"id":   "main",
				}},
			}},
		}.Marshal()),
	}
}

func (balances *Balances) Send(message []byte) *types.SocketMessage {
	incoming := datura.Map[any]{}

	if err := sonic.Unmarshal(message, &incoming); err != nil {
		return nil
	}

	artifact := datura.Acquire(
		"kraken:private", datura.APPJSON,
	).WithRole(
		"balances",
	).WithScope(
		balances.quoteCurrency,
	)

	switch incoming["method"] {
	case "subscribe":
		balances.isActive.Store(true)
	case "unsubscribe":
		balances.isActive.Store(false)
	case "add_order":
		balance := datura.Peek[float64](balances.model, "asset", "balance")
		price := incoming["data"].(map[string]any)["params"].(map[string]any)["limit_price"].(float64)

		if balance-price < 0 {
			artifact.WithPayload(datura.Map[any]{
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
			balance-price, "asset", "balance",
		)

		artifact = balances.model
	default:
		return nil
	}

	balances.observers.Range(func(_ any, value any) bool {
		value.(types.Socket).Send(artifact.Pack())
		return true
	})

	return &types.SocketMessage{}
}

func (balances *Balances) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		balances.observers.Store(uuid.NewString(), socket)
	}
}
