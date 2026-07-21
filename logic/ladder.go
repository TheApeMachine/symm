package logic

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
returnLadderRungs is the structural depth of the dyadic horizon ladder: rung k
resolves against the realized return 2^k epochs ahead, so five rungs span 1
through 16 events. Like the resonance architecture it is a multi-resolution
shape, not a tuned threshold — every rung must still independently prove
positive skill before it may forecast.
*/
const returnLadderRungs = 5

/*
returnLadder trains one strict-prior return head per dyadic horizon over a
shared epoch history, so a symbol whose next-event returns are noise can still
prove skill at the horizon where its structure actually lives. Selection is
model-level — the ready rung with the strongest lower skill bound — never the
rung whose current prediction happens to look best, so choosing a horizon can
not peek at the outcome it is about to claim.
*/
type returnLadder struct {
	rungs   []*returnHead
	history []ladderEntry
}

/*
ladderEntry retains one epoch's feature row, midpoint, and each rung's
strict-prior prediction for that row, awaiting resolution as epochs elapse.
*/
type ladderEntry struct {
	features    []float64
	mid         *decimal.Decimal
	predictions []float64
	predicted   []bool
}

/*
ladderForecast is the selected rung's calibrated view for the current row.
*/
type ladderForecast struct {
	ExpectedReturn float64
	Uncertainty    float64
	MeanMSE        float64
	SkillLower     float64
	Samples        uint64
	HorizonEvents  uint64
	Ready          bool
}

/*
newReturnLadder builds the rung heads; any invalid configuration fails whole.
*/
func newReturnLadder() (*returnLadder, error) {
	rungs := make([]*returnHead, returnLadderRungs)

	for index := range rungs {
		head, err := newReturnHead()

		if err != nil {
			return nil, err
		}

		rungs[index] = head
	}

	return &returnLadder{rungs: rungs}, nil
}

/*
horizon returns rung index's event horizon: 1, 2, 4, 8, ...
*/
func (ladder *returnLadder) horizon(index int) int {
	return 1 << index
}

/*
Advance resolves every rung whose horizon has elapsed against the new
midpoint, teaches those rungs their realized returns, then records this
epoch's row with fresh strict-prior predictions from every rung.
*/
func (ladder *returnLadder) Advance(
	features []float64,
	mid *decimal.Decimal,
) error {
	for index, head := range ladder.rungs {
		events := ladder.horizon(index)

		if len(ladder.history) < events {
			continue
		}

		entry := ladder.history[len(ladder.history)-events]

		if err := head.ResolveAgainst(
			entry.features, entry.mid, mid,
			entry.predictions[index], entry.predicted[index],
		); err != nil {
			return err
		}
	}

	entry := ladderEntry{
		features:    append([]float64(nil), features...),
		mid:         mid.Copy(),
		predictions: make([]float64, len(ladder.rungs)),
		predicted:   make([]bool, len(ladder.rungs)),
	}

	for index, head := range ladder.rungs {
		prediction, err := head.PredictRow(features)

		if err != nil {
			return err
		}

		entry.predictions[index] = prediction
		entry.predicted[index] = true
	}

	ladder.history = append(ladder.history, entry)
	deepest := ladder.horizon(len(ladder.rungs) - 1)

	if len(ladder.history) > deepest {
		ladder.history = ladder.history[len(ladder.history)-deepest:]
	}

	return nil
}

/*
Forecast selects the ready rung with the strongest lower skill bound and
returns its calibrated prediction for the newest recorded row.
*/
func (ladder *returnLadder) Forecast() ladderForecast {
	selected := -1

	for index, head := range ladder.rungs {
		if !head.Ready() {
			continue
		}

		if selected < 0 || head.skillLower > ladder.rungs[selected].skillLower {
			selected = index
		}
	}

	if selected < 0 || len(ladder.history) == 0 {
		return ladderForecast{}
	}

	head := ladder.rungs[selected]
	newest := ladder.history[len(ladder.history)-1]

	return ladderForecast{
		ExpectedReturn: newest.predictions[selected],
		Uncertainty:    head.uncertainty,
		MeanMSE:        head.meanMSE,
		SkillLower:     head.skillLower,
		Samples:        head.samples,
		HorizonEvents:  uint64(ladder.horizon(selected)),
		Ready:          true,
	}
}
