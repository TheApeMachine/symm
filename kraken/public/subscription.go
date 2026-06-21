package public

import (
	"context"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

/*
Subscription arms Kraken public channel feeds from the instrument catalog.
*/
type Subscription struct {
	ctx        context.Context
	tree       *dmt.Tree
	outbound   *qpool.BroadcastGroup
	instrument bool
	armed      bool
}

/*
NewSubscription binds catalog replay to outbound Kraken frames.
*/
func NewSubscription(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Subscription {
	return &Subscription{
		ctx:      ctx,
		tree:     tree,
		outbound: pool.CreateBroadcastGroup("kraken:public"),
	}
}

/*
Armed reports whether market channel subscribe frames were published.
*/
func (subscription *Subscription) Armed() bool {
	return subscription.armed
}

/*
Ensure requests the instrument catalog and subscribes ticker, book, and trade.
*/
func (subscription *Subscription) Ensure() error {
	if subscription.armed {
		return nil
	}

	if !subscription.instrument {
		errnie.Error(subscription.Request())
		subscription.instrument = true
	}

	symbols := subscription.Symbols()

	if len(symbols) == 0 {
		return nil
	}

	for _, channel := range []string{"ticker", "book", "trade"} {
		if err := subscription.Publish(channel, symbols); err != nil {
			return errnie.Error(err)
		}
	}

	subscription.armed = true

	return nil
}

/*
Request publishes the instrument channel subscribe frame.
*/
func (subscription *Subscription) Request() error {
	frame := datura.Acquire("symm", datura.APPJSON).
		WithDestination("kraken:public").
		WithRole("instrument").
		WithPayload(datura.Map[any]{
			"method": "subscribe",
			"params": datura.Map[any]{
				"channel":  "instrument",
				"snapshot": true,
			},
		}.Marshal())

	return errnie.Error(subscription.outbound.Send(frame))
}

/*
Publish batches symbol subscribe frames for one market channel.
*/
func (subscription *Subscription) Publish(channel string, symbols []string) error {
	batchSize := viper.GetInt("market.subscribe_batch")
	pace := viper.GetDuration("market.subscribe_pace")

	if batchSize <= 0 {
		batchSize = len(symbols)
	}

	for start := 0; start < len(symbols); start += batchSize {
		end := start + batchSize

		if end > len(symbols) {
			end = len(symbols)
		}

		frame := datura.Acquire("symm", datura.APPJSON).
			WithDestination("kraken:public").
			WithRole(channel).
			WithPayload(datura.Map[any]{
				"method": "subscribe",
				"params": datura.Map[any]{
					"channel": channel,
					"symbol":  symbols[start:end],
				},
			}.Marshal())

		if err := subscription.outbound.Send(frame); err != nil {
			return errnie.Error(err)
		}

		if pace > 0 && end < len(symbols) {
			time.Sleep(pace)
		}
	}

	return nil
}

/*
Symbols returns quote-matched pairs from the tree, else configured defaults.
*/
func (subscription *Subscription) Symbols() []string {
	symbols := subscription.pairs()

	if len(symbols) > 0 {
		return symbols
	}

	return subscription.defaults()
}

func (subscription *Subscription) pairs() []string {
	quoteCurrency := viper.GetString("market.quote_currency")
	maxScan := viper.GetInt("market.max_scan_symbols")
	seen := map[string]struct{}{}
	symbols := make([]string, 0)

	for artifact := range subscription.tree.Seek([]byte("instrument")) {
		for _, pair := range datura.Peek[[]any](artifact, "data", "pairs") {
			row, ok := pair.(map[string]any)

			if !ok {
				continue
			}

			quote, quoteOK := row["quote"].(string)

			if !quoteOK || quote != quoteCurrency {
				continue
			}

			symbol, symbolOK := row["symbol"].(string)

			if !symbolOK || symbol == "" {
				continue
			}

			if _, exists := seen[symbol]; exists {
				continue
			}

			seen[symbol] = struct{}{}
			symbols = append(symbols, symbol)

			if maxScan > 0 && len(symbols) >= maxScan {
				return symbols
			}
		}
	}

	return symbols
}

func (subscription *Subscription) defaults() []string {
	anchorSymbol := viper.GetString("market.anchor_symbol")
	defaultSymbols := viper.GetStringSlice("market.default_symbols")
	seen := map[string]struct{}{}
	symbols := make([]string, 0, len(defaultSymbols)+1)

	if anchorSymbol != "" {
		seen[anchorSymbol] = struct{}{}
		symbols = append(symbols, anchorSymbol)
	}

	for _, symbol := range defaultSymbols {
		if symbol == "" {
			continue
		}

		if _, exists := seen[symbol]; exists {
			continue
		}

		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}

	return symbols
}
