package sentiment

import (
	"context"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	marketsection "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/dist"
)

/*
Sentiment is the Bullish Breadth perspective, measuring global market conviction
by looking at the behavior of the entire universe simultaneously.

# Summary of Sentiment Categories

| Category       | Breadth | Leader Strength | Market "Feel"           |
|:---------------|:--------|:----------------|:------------------------|
| Risk-On Surge  | High    | Strong          | Rising Tide / Global Buy|
| Divergent Move | Low     | Strong          | Idiosyncratic Alpha     |
| Systemic Slump | Low     | Weak            | Global Risk-Off         |
*/
/*
Signal measures global market conviction from breadth and leadership performance.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	tree         *dmt.Tree
	CrossSection *marketsection.CrossSection
}

func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
	crossSection ...*marketsection.CrossSection,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	section := (*marketsection.CrossSection)(nil)

	if len(crossSection) > 0 {
		section = crossSection[0]
	}

	if section == nil {
		var err error

		section, err = marketsection.NewCrossSection(marketsection.DefaultCrossSectionConfig())

		if err != nil {
			cancel()

			return nil
		}
	}

	return &Signal{
		ctx:          ctx,
		cancel:       cancel,
		tree:         tree,
		CrossSection: section,
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil || signal.CrossSection == nil {
		return nil
	}

	if datura.Peek[string](datapoint, "channel") != "ticker" {
		return nil
	}

	row, rowErr := marketsection.SymbolFromTicker(datapoint)

	if rowErr != nil {
		return nil
	}

	if errnie.Error(signal.CrossSection.Observe(row)) != nil {
		return nil
	}

	breadth := signal.CrossSection.Breadth(row.Updated)
	signal.CrossSection.RecordBreadth(breadth)

	leaderStrength := 0.0
	relativeLead := 0.0

	if signal.CrossSection.IsLeader(row.Name, row.Value, row.Updated) {
		leaderStrength = math.Abs(row.Value)
		relativeLead = 1
	}

	surgeThreshold := signal.CrossSection.MajorityThreshold(row.Updated)

	if surgeThreshold <= 0 {
		return nil
	}

	breadthLift := math.Max(0, breadth-surgeThreshold)

	if breadthLift <= 0 && breadth > 0 {
		breadthLift = breadth * math.Max(0, 1-surgeThreshold)
	}

	leaderMass := leaderStrength / (1 + leaderStrength)

	shares := []dist.Share{
		{Key: "surgeScore", Category: logic.CategoryRiskOnSurge, Mass: breadth * leaderMass * math.Max(relativeLead, 1/(1+leaderStrength))},
		{Key: "divergentScore", Category: logic.CategoryDivergentMove, Mass: (1 - breadth) * relativeLead},
		{Key: "slumpScore", Category: logic.CategorySystemicSlump, Mass: (1 - breadth) * (1 - relativeLead) / (1 + leaderMass)},
	}

	measurement := datura.Acquire("sentiment", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(row.Name)
	errnie.Error(measurement.SetOrigin(string(logic.SourceSentiment)))
	measurement.SetTimestamp(datapoint.Timestamp())

	measurement.MergeOutput("breadth", breadth)
	measurement.MergeOutput("leaderStrength", leaderStrength)
	confidence := dist.Write(measurement, shares)

	if confidence <= 0 {
		measurement.Release()

		return nil
	}

	return measurement
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
