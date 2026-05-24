package eval

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/symm/causal"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/fluid"
	"github.com/theapemachine/symm/hawkes"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/book"
	"github.com/theapemachine/symm/kraken/client"
	kmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/ticker"
	"github.com/theapemachine/symm/kraken/trades"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/pumpdump"
	"github.com/theapemachine/symm/replay"
	"github.com/theapemachine/symm/trader"
)

const defaultMaxRescoreTicks = 512

/*
Options configures one offline replay evaluation run.
*/
type Options struct {
	ReplayFile string
	StartTime  time.Time
	MaxTicks   int
}

/*
Run replays captured websocket frames and returns a calibration report.
*/
func Run(ctx context.Context, opts Options) (Report, error) {
	if opts.ReplayFile == "" {
		return Report{}, fmt.Errorf("replay file is required")
	}

	frames, err := replay.LoadFrames(opts.ReplayFile)
	if err != nil {
		return Report{}, err
	}

	publicClient := client.NewPublicClient(
		ctx,
		client.WithReplay([][]byte{[]byte(`{}`)}, 0),
	)

	pairObserver, err := market.NewPairs(ctx, config.System.QuoteCurrency, publicClient)
	if err != nil {
		return Report{}, fmt.Errorf("pairs observer: %w", err)
	}

	instrumentFrames, marketFrames, err := splitReplayFrames(frames)
	if err != nil {
		return Report{}, err
	}

	for _, payload := range instrumentFrames {
		if err := publicClient.InjectFrame(ctx, payload); err != nil {
			return Report{}, fmt.Errorf("inject instrument frame: %w", err)
		}
	}

	symbols, err := pairObserver.Names(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("pair names: %w", err)
	}

	assetPairs, err := pairObserver.GetAll(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("pair index: %w", err)
	}

	pairIndex := buildPairIndex(assetPairs)

	cryptoTrader, _, err := wireTrader(
		ctx,
		publicClient,
		symbols,
		pairIndex,
	)
	if err != nil {
		return Report{}, err
	}

	collector := NewCollector()
	cryptoTrader.BindFeedbackSink(collector.Sink())

	for _, payload := range marketFrames {
		if err := publicClient.InjectFrame(ctx, payload); err != nil {
			return Report{}, fmt.Errorf("inject market frame: %w", err)
		}
	}

	if err := rescoreUntilSettled(cryptoTrader, opts); err != nil {
		return Report{}, err
	}

	return BuildReport(opts.ReplayFile, collector.Records()), nil
}

func splitReplayFrames(frames [][]byte) (instrument, market [][]byte, err error) {
	for _, payload := range frames {
		channel, channelErr := kmarket.ChannelName(payload)

		if channelErr != nil {
			return nil, nil, fmt.Errorf("parse replay channel: %w", channelErr)
		}

		if channel == "instrument" {
			instrument = append(instrument, payload)
			continue
		}

		market = append(market, payload)
	}

	if len(instrument) == 0 {
		return nil, nil, fmt.Errorf("replay file has no instrument frames")
	}

	return instrument, market, nil
}

func buildPairIndex(assetPairs []asset.Pair) map[string]asset.Pair {
	index := make(map[string]asset.Pair, len(assetPairs))

	for _, pair := range assetPairs {
		name := pair.Wsname

		if name == "" {
			name = pair.Altname
		}

		index[name] = pair
	}

	return index
}

func wireTrader(
	ctx context.Context,
	publicClient *client.PublicClient,
	symbols []string,
	pairIndex map[string]asset.Pair,
) (*trader.Crypto, *ticker.Ticker, error) {
	bookObserver, err := book.New(ctx, publicClient, symbols)
	if err != nil {
		return nil, nil, fmt.Errorf("book observer: %w", err)
	}

	tradesObserver, err := trades.New(ctx, publicClient, symbols)
	if err != nil {
		return nil, nil, fmt.Errorf("trades observer: %w", err)
	}

	tickerObserver, err := ticker.New(ctx, publicClient, symbols)
	if err != nil {
		return nil, nil, fmt.Errorf("ticker observer: %w", err)
	}

	symbolWatch := engine.NewSymbolWatch(symbols)

	tradesObserver.SetActivityListener(func(symbol string, volume float64) {
		symbolWatch.NoteTrade(symbol, volume)
	})

	bookObserver.SetActivityListener(func(symbol string) {
		symbolWatch.NoteBook(symbol)
	})

	tickerObserver.OnQuote(func(
		symbol string,
		_, _, _, changePct float64,
		_ string,
	) {
		symbolWatch.NoteTicker(symbol, changePct)
	})

	pumpSignal, err := pumpdump.NewPumpDump(
		ctx,
		bookObserver,
		tradesObserver,
		tickerObserver,
		pairIndex,
		symbols,
		symbolWatch,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("pumpdump signal: %w", err)
	}

	hawkesSignal, err := hawkes.NewHawkes(
		ctx,
		bookObserver,
		tradesObserver,
		tickerObserver,
		pairIndex,
		symbols,
		symbolWatch,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("hawkes signal: %w", err)
	}

	fluidSignal, err := fluid.NewFluid(
		ctx,
		bookObserver,
		tradesObserver,
		tickerObserver,
		pairIndex,
		symbols,
		symbolWatch,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("fluid signal: %w", err)
	}

	causalSignal, err := causal.NewCausal(
		ctx,
		bookObserver,
		tradesObserver,
		tickerObserver,
		pairIndex,
		symbols,
		symbolWatch,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("causal signal: %w", err)
	}

	wallet := trader.NewWallet(
		trader.PaperWallet,
		config.System.QuoteCurrency,
		config.System.WalletEUR,
		config.System.TakerFeePct,
	)

	cryptoTrader, err := trader.NewCrypto(
		ctx,
		nil,
		wallet,
		tickerObserver,
		pumpSignal,
		hawkesSignal,
		fluidSignal,
		causalSignal,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("crypto trader: %w", err)
	}

	return cryptoTrader, tickerObserver, nil
}

func rescoreUntilSettled(cryptoTrader *trader.Crypto, opts Options) error {
	start := opts.StartTime

	if start.IsZero() {
		start = time.Unix(1_700_000_000, 0)
	}

	maxTicks := opts.MaxTicks

	if maxTicks <= 0 {
		maxTicks = defaultMaxRescoreTicks
	}

	step := config.System.RescoreEvery
	seenPending := false

	for tick := 0; tick < maxTicks; tick++ {
		now := start.Add(time.Duration(tick) * step)

		if err := cryptoTrader.Rescore(now); err != nil {
			return err
		}

		pending := cryptoTrader.PendingPredictionCount()

		if pending > 0 {
			seenPending = true
		}

		if seenPending && pending == 0 {
			return nil
		}
	}

	return nil
}
