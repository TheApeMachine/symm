package trader

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/futures"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/ui"
)

type Crypto struct {
	ctx    context.Context
	cancel context.CancelFunc
	pool   *qpool.Q[any]
	ui     *qpool.BroadcastGroup
	bus    *internal.Bus
	pairs  *sync.Map
	chart  sync.Once
}

func NewCrypto(ctx context.Context, pool *qpool.Q[any]) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	return &Crypto{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		ui:     pool.CreateBroadcastGroup("ui", 10*time.Millisecond),
		bus: internal.NewBus(
			ctx,
			pool,
			[]string{"kraken:public", "kraken:private", "kraken:futures", "ui", "raw"},
			[]string{"raw"},
		),
		pairs: &sync.Map{},
	}
}

/*
Tick consumes the raw bus. Story emits actions; crypto forwards them as order
messages on raw for the desk, and handles instruments and subscriptions here.
*/
func (crypto *Crypto) Tick() (err error) {
	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		default:
			crypto.ensureAnchorChart()

			message, err := crypto.bus.Receive("raw")

			if errnie.Error(err) != nil || message == nil {
				continue
			}

			var (
				ok bool
			)

			switch message.Type {
			case "balances":
				balances, ok := message.Value.(user.Balances)

				if !ok {
					errnie.Error(errors.New("crypto: invalid balances"))
					continue
				}

				frame, frameErr := ui.WalletFrame(balances)

				if frameErr != nil {
					errnie.Error(frameErr)
					continue
				}

				errnie.Error(crypto.bus.Send("ui", "wallet", frame))
			case "instrument":
				var (
					instrument *market.InstrumentUpdate
				)

				if instrument, ok = message.Value.(*market.InstrumentUpdate); !ok {
					errnie.Error(errors.New("crypto: invalid instrument"), "crypto: invalid instrument")
					continue
				}

				quoteCurrency := viper.GetString("market.quote_currency")
				bookDepth := viper.GetInt("market.book_depth_levels")
				pairs := make([]string, 0)

				for _, pair := range instrument.Pairs {
					if pair.Status != "online" || pair.Quote != quoteCurrency {
						continue
					}

					if _, subscribed := crypto.pairs.Load(pair.Symbol); subscribed {
						continue
					}

					pairs = append(pairs, pair.Symbol)
				}

				if len(pairs) == 0 {
					continue
				}

				errnie.Info(fmt.Sprintf("subscribing to %d pairs", len(pairs)))

				errnie.Error(crypto.bus.Send("raw", "symbols", pairs))

				errnie.Error(crypto.bus.Send("kraken:public", "ticker", types.KrakenMessage{
					Method: "subscribe",
					Params: market.NewTickerParams(pairs),
					ReqID:  time.Now().UnixNano(),
				}))

				errnie.Error(crypto.bus.Send("kraken:public", "book", types.KrakenMessage{
					Method: "subscribe",
					Params: market.NewBookParams(pairs, bookDepth),
					ReqID:  time.Now().UnixNano(),
				}))

				errnie.Error(crypto.bus.Send("kraken:public", "trade", types.KrakenMessage{
					Method: "subscribe",
					Params: market.NewTradeParams(pairs),
					ReqID:  time.Now().UnixNano(),
				}))

				if viper.GetBool("market.l3_enabled") {
					token, err := types.NewToken(crypto.ctx)

					if errnie.Error(err) != nil {
						continue
					}

					level3Params := market.NewLevel3Params(pairs)
					level3Params.Token = token

					errnie.Error(crypto.bus.Send("kraken:private", "level3", types.KrakenMessage{
						Method: "subscribe",
						Params: level3Params,
						ReqID:  time.Now().UnixNano(),
					}))
				}

				for _, symbol := range pairs {
					crypto.pairs.Store(symbol, true)
				}

				if !viper.GetBool("market.futures_enabled") {
					continue
				}

				catalog := futures.SharedCatalog()

				if loadErr := catalog.EnsureLoaded(crypto.ctx); loadErr != nil {
					errnie.Error(loadErr)
					continue
				}

				productSet := make(map[string]struct{})

				for _, symbol := range pairs {
					products, productErr := catalog.ProductsForSpotPair(symbol)

					if productErr != nil {
						errnie.Error(productErr)
						continue
					}

					for _, productID := range products {
						productSet[productID] = struct{}{}
					}
				}

				if len(productSet) == 0 {
					continue
				}

				futuresProducts := make([]string, 0, len(productSet))

				for productID := range productSet {
					futuresProducts = append(futuresProducts, productID)
				}

				errnie.Error(crypto.bus.Send("kraken:futures", "book", futures.SubscribeMessage{
					Event:      "subscribe",
					Feed:       futures.BookFeed,
					ProductIDs: futuresProducts,
				}))
			case "actions":
				var (
					action *logic.Action
				)

				if action, ok = message.Value.(*logic.Action); !ok {
					errnie.Error(errors.New("crypto: invalid action"), "crypto: invalid action")
					continue
				}

				errnie.Error(crypto.bus.Send("raw", "order", action))
			}
		}
	}
}

func (crypto *Crypto) Close() error {
	if crypto.cancel != nil {
		crypto.cancel()
	}

	return nil
}

func (crypto *Crypto) ensureAnchorChart() {
	crypto.chart.Do(func() {
		anchor := viper.GetString("market.anchor_symbol")

		if anchor == "" {
			return
		}

		candleParams, paramsErr := market.NewCandleParams([]string{anchor}, 1)

		if errnie.Error(paramsErr) != nil {
			return
		}

		errnie.Error(crypto.bus.Send("kraken:public", "ohlc", types.KrakenMessage{
			Method: "subscribe",
			Params: candleParams,
			ReqID:  time.Now().UnixNano(),
		}))
	})
}
