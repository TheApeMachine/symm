package leadlag

import (
	"context"
	"math"

	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

/*
LeadLag is the "Anchor" perspective, measuring the temporal correlation
between the current cross-section leader (the pair the universe is chasing,
derived live via CrossSection.Leader — no config major) and each follower.

# Summary of LeadLag Categories

| Category           | Lead/Lag Correlation | Lag Fraction | Market "Feel"             |
|:-------------------|:---------------------|:-------------|:--------------------------|
| Inefficient Lag    | High                 | High         | Catch-up Opportunity      |
| Synchronized Drift | High                 | Low          | Systemic Beta             |
| Decoupled Move     | Low                  | N/A          | Idiosyncratic Alpha       |
| Anchor Stall       | Low                  | Low          | Leadership Exhaustion     |
*/
/*
Signal measures temporal correlation between the anchor pair and each follower.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	Section *Section
	ticker  *Ticker
}

func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	section := NewSection()

	return &Signal{
		ctx:     ctx,
		cancel:  cancel,
		Section: section,
		ticker:  NewTicker(section),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(
	input market.Input,
	crossSection *market.CrossSection,
) ([]*logic.Measurement, error) {
	if input.Role != "ticker" {
		return nil, nil
	}

	measurements := make([]*logic.Measurement, 0, len(input.Ticker))
	for _, row := range input.Ticker {
		if row.Symbol == "" {
			continue
		}

		if row.Last <= 0 || math.IsNaN(row.Last) || math.IsInf(row.Last, 0) {
			continue
		}

		measurement, err := signal.ticker.Measure(row, crossSection)
		if err != nil {
			return nil, err
		}

		if measurement == nil {
			continue
		}

		measurements = append(measurements, measurement)
	}

	return measurements, nil
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
