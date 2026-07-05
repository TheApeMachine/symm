package trader

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	causalsignal "github.com/theapemachine/symm/signal/causal"
	correlationsignal "github.com/theapemachine/symm/signal/correlation"
	cvdsignal "github.com/theapemachine/symm/signal/cvd"
	depthflowsignal "github.com/theapemachine/symm/signal/depthflow"
	exhaustsignal "github.com/theapemachine/symm/signal/exhaust"
	fluidsignal "github.com/theapemachine/symm/signal/fluid"
	hawkessignal "github.com/theapemachine/symm/signal/hawkes"
	leadlagsignal "github.com/theapemachine/symm/signal/leadlag"
	liquiditysignal "github.com/theapemachine/symm/signal/liquidity"
	manifoldsignal "github.com/theapemachine/symm/signal/manifold"
	pumpdumpsignal "github.com/theapemachine/symm/signal/pumpdump"
	resonancesignal "github.com/theapemachine/symm/signal/resonance"
	sentimentsignal "github.com/theapemachine/symm/signal/sentiment"
	toxicitysignal "github.com/theapemachine/symm/signal/toxicity"
)

type Signals struct {
	ctx          context.Context
	cancel       context.CancelFunc
	crossSection *market.CrossSection
	bindings     []*SignalBinding
}

type SignalSnapshot struct {
	Source  logic.SourceType
	Payload map[string]any
}

type SignalBinding struct {
	signal market.Signal
	roles  map[string]struct{}
}

type dashboardSnapshotter interface {
	DashboardSnapshot() (logic.SourceType, map[string]any, error)
}

func NewSignals(ctx context.Context) (*Signals, error) {
	ctx, cancel := context.WithCancel(ctx)

	crossSection, err := market.NewCrossSection()

	if err != nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, err.Error(), err,
		))
	}

	signals := &Signals{
		ctx:          ctx,
		cancel:       cancel,
		crossSection: crossSection,
	}

	signals.Bind(causalsignal.NewSignal(ctx))
	signals.Bind(correlationsignal.NewSignal(ctx))
	signals.Bind(cvdsignal.NewSignal(ctx))
	signals.Bind(depthflowsignal.NewSignal(ctx))
	signals.Bind(exhaustsignal.NewSignal(ctx))
	signals.Bind(fluidsignal.NewSignal(ctx))
	signals.Bind(hawkessignal.NewSignal(ctx))
	signals.Bind(leadlagsignal.NewSignal(ctx))
	signals.Bind(liquiditysignal.NewSignal(ctx))
	signals.Bind(manifoldsignal.NewSignal(ctx))
	signals.Bind(pumpdumpsignal.NewSignal(ctx))
	signals.Bind(resonancesignal.NewSignal(ctx, nil, 0, 0))
	signals.Bind(sentimentsignal.NewSignal(ctx))
	signals.Bind(toxicitysignal.NewSignal(ctx))

	return signals, nil
}

func (signals *Signals) Bind(signal market.Signal) {
	binding := &SignalBinding{
		signal: signal,
		roles:  map[string]struct{}{},
	}

	for _, role := range signal.IngestRoles() {
		binding.roles[role] = struct{}{}
	}

	signals.bindings = append(signals.bindings, binding)
}

func (signals *Signals) Measure(
	role string,
	payload []byte,
	at time.Time,
) ([]*logic.Measurement, []SignalSnapshot, error) {
	if role == "" {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.Validation, "trader: signal role required", nil,
		))
	}

	input, err := market.NewInput(role, payload, at)

	if err != nil {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.Validation, err.Error(), err,
		))
	}

	if role == channelTicker {
		if err := signals.crossSection.Observe(input.Ticker); err != nil {
			return nil, nil, errnie.Error(errnie.Err(
				errnie.Validation, err.Error(), err,
			))
		}
	}

	measurements := make([]*logic.Measurement, 0)
	snapshots := make([]SignalSnapshot, 0)

	for _, binding := range signals.bindings {
		if !binding.Observes(role) {
			continue
		}

		measured, err := binding.signal.Measure(input, signals.crossSection)

		if err != nil {
			return nil, nil, errnie.Error(errnie.Err(
				errnie.Validation, err.Error(), err,
			))
		}

		for _, measurement := range measured {
			if measurement == nil {
				return nil, nil, errnie.Error(errnie.Err(
					errnie.Validation, "trader: nil signal measurement", nil,
				))
			}

			measurements = append(measurements, measurement)
		}

		snapshotter, ok := binding.signal.(dashboardSnapshotter)

		if !ok {
			continue
		}

		source, snapshot, err := snapshotter.DashboardSnapshot()

		if err != nil {
			return nil, nil, errnie.Error(errnie.Err(
				errnie.Validation, err.Error(), err,
			))
		}

		if snapshot != nil {
			snapshots = append(snapshots, SignalSnapshot{
				Source:  source,
				Payload: snapshot,
			})
		}
	}

	return measurements, snapshots, nil
}

func (binding *SignalBinding) Observes(role string) bool {
	_, ok := binding.roles[role]
	return ok
}

func (signals *Signals) Close() error {
	signals.cancel()

	for _, binding := range signals.bindings {
		if err := binding.signal.Close(); err != nil {
			return err
		}
	}

	return nil
}
