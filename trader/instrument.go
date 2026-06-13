package trader

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/futures"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/rawbus"
)

type Instrument struct {
	ctx              context.Context
	cancel           context.CancelFunc
	bus              *internal.Bus
	pairs            *sync.Map
	candles          *sync.Map
	anchorSubscribed atomic.Bool
	marketConfig     config.MarketConfig
}

func (instrument *Instrument) reset() {
	instrument.pairs.Clear()
	instrument.candles.Clear()
	instrument.anchorSubscribed.Store(false)
}

func (instrument *Instrument) subscribeAnchor() error {
	if instrument.anchorSubscribed.Load() {
		return nil
	}

	anchor := instrument.marketConfig.AnchorSymbol

	if anchor == "" {
		return nil
	}

	if err := instrument.subscribeCandles([]string{anchor}); err != nil {
		return err
	}

	instrument.anchorSubscribed.Store(true)

	return nil
}

func NewInstrument(
	ctx context.Context, bus *internal.Bus,
) *Instrument {
	ctx, cancel := context.WithCancel(ctx)
	marketConfig, _ := config.LoadMarketConfig()

	return &Instrument{
		ctx:          ctx,
		cancel:       cancel,
		bus:          bus,
		pairs:        &sync.Map{},
		candles:      &sync.Map{},
		marketConfig: marketConfig,
	}
}

func (instrument *Instrument) SubscribePositionCandles(
	balances user.Balances,
) error {
	return instrument.subscribeCandles(
		instrument.positionSymbols(balances),
	)
}

func (instrument *Instrument) subscribeTickers(pairs []string) error {
	for _, trigger := range market.TickerTriggers() {
		if err := errnie.Error(instrument.bus.Send(
			internal.ChannelKrakenPublic,
			"ticker",
			types.KrakenMessage{
				Method: "subscribe",
				Params: market.NewTickerParams(pairs, trigger),
				ReqID:  time.Now().UnixNano(),
			},
		)); err != nil {
			return errnie.Err(
				errnie.IO,
				"crypto: failed to send ticker",
				err,
			)
		}
	}

	return nil
}

func (instrument *Instrument) subscribeCandles(symbols []string) error {
	pending := make([]string, 0, len(symbols))

	for _, symbol := range symbols {
		normalized := strings.ToUpper(strings.TrimSpace(symbol))

		if normalized == "" {
			continue
		}

		if _, subscribed := instrument.candles.Load(normalized); subscribed {
			continue
		}

		pending = append(pending, normalized)
	}

	if len(pending) == 0 {
		return nil
	}

	if err := errnie.Error(instrument.bus.Send(
		internal.ChannelKrakenPublic,
		"ohlc",
		types.KrakenMessage{
			Method: "subscribe",
			Params: market.CandleParams{
				Channel:  "ohlc",
				Symbol:   pending,
				Interval: 1,
				Snapshot: true,
			},
			ReqID: time.Now().UnixNano(),
		},
	)); err != nil {
		return err
	}

	for _, symbol := range pending {
		instrument.candles.Store(symbol, true)
	}

	return nil
}

func (instrument *Instrument) positionSymbols(
	balances user.Balances,
) []string {
	quoteCurrency := instrument.positionQuoteCurrency(balances)
	symbols := make([]string, 0, len(balances.Inventory)+len(balances.Asset))

	for base, quantity := range balances.Inventory {
		normalizedBase := strings.ToUpper(strings.TrimSpace(base))

		if quantity <= 0 || normalizedBase == "" {
			continue
		}

		symbols = append(symbols, normalizedBase+"/"+quoteCurrency)
	}

	if len(symbols) > 0 {
		return symbols
	}

	for _, asset := range balances.Asset {
		assetName := strings.ToUpper(strings.TrimSpace(asset.Asset))

		if asset.Balance <= 0 || assetName == "" {
			continue
		}

		if assetName == quoteCurrency || assetName == "Z"+quoteCurrency {
			continue
		}

		symbols = append(symbols, assetName+"/"+quoteCurrency)
	}

	return symbols
}

func (instrument *Instrument) positionQuoteCurrency(
	balances user.Balances,
) string {
	quoteCurrency := strings.ToUpper(strings.TrimSpace(balances.Currency))

	if quoteCurrency != "" {
		return quoteCurrency
	}

	quoteCurrency = strings.ToUpper(strings.TrimSpace(
		instrument.marketConfig.QuoteCurrency,
	))

	if quoteCurrency != "" {
		return quoteCurrency
	}

	return "USD"
}

func (instrument *Instrument) Tick(message *qpool.QValue[any]) error {
	if err := instrument.subscribeAnchor(); err != nil {
		return errnie.Err(
			errnie.IO,
			"crypto: failed to subscribe anchor ohlc",
			err,
		)
	}

	update, ok := message.Value.(*market.InstrumentUpdate)

	if !ok {
		return errnie.Err(
			errnie.Validation,
			"crypto: invalid instrument",
			errors.New(message.Type),
		)
	}

	quoteCurrency := instrument.marketConfig.QuoteCurrency
	bookDepth := instrument.marketConfig.BookDepthLevels
	pairs := make([]string, 0)

	for _, pair := range update.Pairs {
		if pair.Status != "online" || pair.Quote != quoteCurrency {
			continue
		}

		if _, subscribed := instrument.pairs.Load(pair.Symbol); subscribed {
			continue
		}

		pairs = append(pairs, pair.Symbol)
	}

	if len(pairs) == 0 {
		return nil
	}

	errnie.Info(fmt.Sprintf("subscribing to %d pairs", len(pairs)))

	if err := errnie.Error(rawbus.Send(
		instrument.bus, rawbus.TypeSymbols, pairs,
	)); errnie.Error(err) != nil {
		return errnie.Err(
			errnie.IO,
			"crypto: failed to send symbols",
			err,
		)
	}

	if err := instrument.subscribeTickers(pairs); err != nil {
		return err
	}

	if err := errnie.Error(instrument.bus.Send(
		internal.ChannelKrakenPublic,
		"book",
		types.KrakenMessage{
			Method: "subscribe",
			Params: market.NewBookParams(pairs, bookDepth),
			ReqID:  time.Now().UnixNano(),
		},
	)); errnie.Error(err) != nil {
		return errnie.Err(
			errnie.IO,
			"crypto: failed to send book",
			err,
		)
	}

	if err := errnie.Error(instrument.bus.Send(
		internal.ChannelKrakenPublic,
		"trade",
		types.KrakenMessage{
			Method: "subscribe",
			Params: market.NewTradeParams(pairs),
			ReqID:  time.Now().UnixNano(),
		},
	)); errnie.Error(err) != nil {
		return errnie.Err(
			errnie.IO,
			"crypto: failed to send trade",
			err,
		)
	}

	if instrument.marketConfig.L3Enabled {
		token, err := types.NewToken(instrument.ctx)

		if errnie.Error(err) != nil {
			return errnie.Err(
				errnie.Validation,
				"crypto: failed to create token",
				err,
			)
		}

		level3Params := market.NewLevel3Params(pairs)
		level3Params.Token = token

		if err := errnie.Error(instrument.bus.Send(
			internal.ChannelKrakenPrivate,
			"level3",
			types.KrakenMessage{
				Method: "subscribe",
				Params: level3Params,
				ReqID:  time.Now().UnixNano(),
			},
		)); errnie.Error(err) != nil {
			return errnie.Err(
				errnie.IO,
				"crypto: failed to send level3",
				err,
			)
		}
	}

	for _, symbol := range pairs {
		instrument.pairs.Store(symbol, true)
	}

	if !instrument.marketConfig.FuturesEnabled {
		return nil
	}

	catalog := futures.SharedCatalog()

	if err := catalog.EnsureLoaded(instrument.ctx); err != nil {
		if internal.IsShutdown(err) {
			return err
		}

		return errnie.Err(
			errnie.IO,
			"crypto: failed to load futures catalog",
			err,
		)
	}

	productSet := make(map[string]struct{})

	for _, symbol := range pairs {
		products, err := catalog.ProductsForSpotPair(symbol)

		if errnie.Error(err) != nil {
			return errnie.Err(
				errnie.IO,
				"crypto: failed to get futures products",
				err,
			)
		}

		for _, productID := range products {
			productSet[productID] = struct{}{}
		}
	}

	if len(productSet) == 0 {
		return nil
	}

	futuresProducts := make([]string, 0, len(productSet))

	for productID := range productSet {
		futuresProducts = append(futuresProducts, productID)
	}

	if err := errnie.Error(instrument.bus.Send(
		internal.ChannelKrakenFutures,
		"book",
		futures.SubscribeMessage{
			Event:      "subscribe",
			Feed:       futures.BookFeed,
			ProductIDs: futuresProducts,
		},
	)); errnie.Error(err) != nil {
		return errnie.Err(
			errnie.IO,
			"crypto: failed to send futures book",
			err,
		)
	}

	return nil
}
