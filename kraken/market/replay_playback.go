package market

import (
	"context"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/replay"
)

func replayPath() string {
	return strings.TrimSpace(config.System.ReplayFile)
}

func openReplayHub() (*replay.Hub, bool) {
	path := replayPath()

	if path == "" {
		return nil, false
	}

	hub, err := replay.Open(path)

	if err != nil {
		errnie.Error(err)

		return nil, false
	}

	return hub, true
}

/*
ReplayDone returns the replay completion channel when a fixture is active.
*/
func ReplayDone() <-chan struct{} {
	path := replayPath()

	if path == "" {
		return nil
	}

	hub, err := replay.Open(path)

	if err != nil {
		return nil
	}

	return hub.Done()
}

func replayTrades(ctx context.Context, symbols []string) <-chan *TradeUpdate {
	hub, ok := openReplayHub()

	if !ok {
		return closed[TradeUpdate]()
	}

	return replay.StreamRows[TradeUpdate](ctx, hub, public.TradesChannel)
}

func replayBook(ctx context.Context, symbols []string) <-chan *BookUpdate {
	hub, ok := openReplayHub()

	if !ok {
		return closed[BookUpdate]()
	}

	return replay.StreamRows[BookUpdate](ctx, hub, public.BookChannel)
}

func replayTicker(ctx context.Context, symbols []string) <-chan *TickerUpdate {
	hub, ok := openReplayHub()

	if !ok {
		return closed[TickerUpdate]()
	}

	return replay.StreamRows[TickerUpdate](ctx, hub, public.TickerChannel)
}

func tradeUpstream(ctx context.Context, symbols []string) <-chan *TradeUpdate {
	if replayPath() != "" {
		return replayTrades(ctx, symbols)
	}

	return dialTrades(ctx, symbols)
}

func bookUpstream(ctx context.Context, depth int, symbols []string) <-chan *BookUpdate {
	if replayPath() != "" {
		return replayBook(ctx, symbols)
	}

	return dialBook(ctx, depth, symbols)
}

func tickerUpstream(ctx context.Context, symbols []string) <-chan *TickerUpdate {
	if replayPath() != "" {
		return replayTicker(ctx, symbols)
	}

	return dialTicker(ctx, symbols)
}
