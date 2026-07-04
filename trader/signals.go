package trader

import (
	"context"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken"
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

type SignalBinding struct {
	signal market.Signal
	roles  map[string]struct{}
}

func NewSignals(ctx context.Context, pool *qpool.Q[any]) (*Signals, error) {
	ctx, cancel := context.WithCancel(ctx)

	crossSection, err := market.NewCrossSection()
	if err != nil {
		cancel()
		return nil, err
	}

	signals := &Signals{
		ctx:          ctx,
		cancel:       cancel,
		crossSection: crossSection,
	}

	signals.Bind(causalsignal.NewSignal(ctx, nil))
	signals.Bind(correlationsignal.NewSignal(ctx, nil))
	signals.Bind(cvdsignal.NewSignal(ctx, nil))
	signals.Bind(depthflowsignal.NewSignal(ctx, nil))
	signals.Bind(exhaustsignal.NewSignal(ctx, nil))
	signals.Bind(fluidsignal.NewSignal(ctx, pool, nil))
	signals.Bind(hawkessignal.NewSignal(ctx, nil))
	signals.Bind(leadlagsignal.NewSignal(ctx, nil))
	signals.Bind(liquiditysignal.NewSignal(ctx, nil))
	signals.Bind(pumpdumpsignal.NewSignal(ctx, nil))
	signals.Bind(resonancesignal.NewSignal(ctx, pool, nil, nil, 0, 0))
	signals.Bind(sentimentsignal.NewSignal(ctx, nil))
	signals.Bind(toxicitysignal.NewSignal(ctx, nil))

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
) ([]*datura.Artifact, error) {
	if role == "" {
		return nil, errnie.Err(errnie.Validation, "trader: signal role required", nil)
	}

	if role == channelTicker {
		if err := signals.crossSection.Observe(kraken.NewTickerDataSlice(payload)); err != nil {
			return nil, err
		}
	}

	frame, err := signals.frame(role, payload, at)
	if err != nil {
		return nil, err
	}

	measurements := make([]*datura.Artifact, 0)
	for _, binding := range signals.bindings {
		if !binding.Observes(role) {
			continue
		}

		for measurement := range binding.signal.Measure(frame, signals.crossSection) {
			if measurement == nil {
				return nil, errnie.Err(errnie.Validation, "trader: nil signal measurement", nil)
			}

			measurements = append(measurements, measurement)
		}
	}

	return measurements, nil
}

func (signals *Signals) frame(
	role string,
	payload []byte,
	at time.Time,
) (*datura.Artifact, error) {
	var rows []map[string]any
	if err := sonic.Unmarshal(payload, &rows); err != nil {
		return nil, errnie.Err(errnie.Validation, "trader: decode signal payload", err)
	}

	frame := datura.Acquire("trader", datura.APPJSON)
	frame.WithRole(role)
	frame.WithScope("update")
	frame.WithPayload(datura.Map[any]{
		"channel": role,
		"type":    "update",
		"data":    rows,
	}.Marshal())

	if !at.IsZero() {
		frame.SetTimestamp(at.UnixNano())
	}

	return frame, nil
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
