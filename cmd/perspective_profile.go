package cmd

import (
	"context"
	"errors"
	"runtime"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/signal/causal"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/fluid"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/toxicity"
)

type perspectiveProfileSystem struct {
	ctx          context.Context
	cancel       context.CancelFunc
	measurements *qpool.Subscriber
	builder      *perspectives.ProfileBuilder
}

func newPerspectiveProfileSystem(
	ctx context.Context,
	pool *qpool.Q,
) *perspectiveProfileSystem {
	ctx, cancel := context.WithCancel(ctx)
	group := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	return &perspectiveProfileSystem{
		ctx:          ctx,
		cancel:       cancel,
		measurements: group.Subscribe("perspective-profile:measurements", 4096),
		builder:      perspectives.NewProfileBuilder(nil),
	}
}

func (system *perspectiveProfileSystem) Tick() error {
	for {
		select {
		case <-system.ctx.Done():
			return system.ctx.Err()
		case value, ok := <-system.measurements.Incoming:
			if !ok {
				return nil
			}

			measurement, measurementOK := value.Value.(perspectives.Measurement)

			if !measurementOK {
				continue
			}

			system.builder.Record(measurement)
		}
	}
}

func (system *perspectiveProfileSystem) Close() error {
	system.cancel()

	return nil
}

func (system *perspectiveProfileSystem) Profile() perspectives.SearchProfile {
	return system.builder.Profile()
}

func profileReplayPrimitives(
	parent context.Context,
	replayFile string,
) (perspectives.SearchProfile, error) {
	restorer := overrideProfileConfig(replayFile)
	defer restorer()

	profileCtx, cancel := context.WithCancel(parent)
	defer cancel()

	pool := qpool.NewQ(profileCtx, 1, runtime.NumCPU()*4, qpool.NewConfig())
	restoreLogging := qpool.SuppressLogging()
	defer restoreLogging()
	applyReplayMeta(replayFile)
	config.SyncRuntime()
	krakenmarket.ConfigureCatalogFees(
		config.System.Fee30DVolume,
		config.System.TakerFeePct,
		config.System.MakerFeePct,
	)

	if err := krakenmarket.ConfigureLevel3(
		config.System.KrakenAPIKey,
		config.System.KrakenAPISecret,
	); err != nil {
		return perspectives.SearchProfile{}, err
	}

	krakenmarket.BootPairCatalog(
		profileCtx,
		config.System.Fee30DVolume,
		config.System.TakerFeePct,
		config.System.MakerFeePct,
	)

	booter, err := NewBooter(profileCtx, pool)

	if err != nil {
		return perspectives.SearchProfile{}, err
	}

	runCtx := booter.Context()
	profiler := newPerspectiveProfileSystem(runCtx, pool)

	if err := booter.AddSystems(
		pumpdump.NewSignal(runCtx, pool),
		correlation.NewSignal(runCtx, pool),
		depthflow.NewSignal(runCtx, pool),
		hawkes.NewSignal(runCtx, pool),
		leadlag.NewSignal(runCtx, pool),
		liquidity.NewSignal(runCtx, pool),
		sentiment.NewSignal(runCtx, pool),
		fluid.NewSignal(runCtx, pool),
		causal.NewSignal(runCtx, pool),
		cvd.NewSignal(runCtx, pool),
		toxicity.NewToxicity(runCtx, pool),
		exhaust.NewSignal(runCtx, pool),
		profiler,
	); err != nil {
		return perspectives.SearchProfile{}, err
	}

	if done := krakenmarket.ReplayDone(); done != nil {
		go func() {
			<-done
			booter.Cancel()
		}()
	}

	if err := booter.Boot(); err != nil && !errors.Is(err, context.Canceled) {
		return perspectives.SearchProfile{}, err
	}

	profile := profiler.Profile()

	if err := profile.Validate(); err != nil {
		return perspectives.SearchProfile{}, err
	}

	return profile, nil
}

func overrideProfileConfig(replayFile string) func() {
	previousReplay := config.System.ReplayFile
	previousHeadless := config.System.Headless
	previousPace := config.System.ReplayPace
	previousSymbols := append([]string(nil), config.System.Symbols...)

	config.System.ReplayFile = replayFile
	config.System.Headless = true
	config.System.ReplayPace = 0
	config.SyncRuntime()

	return func() {
		config.System.ReplayFile = previousReplay
		config.System.Headless = previousHeadless
		config.System.ReplayPace = previousPace
		config.System.Symbols = previousSymbols
		config.SyncRuntime()
	}
}
