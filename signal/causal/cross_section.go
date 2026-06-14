package causal

import (
	"sync"

	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/statistic"
)

var crossSection macroSection

/*
macroSection maps symbols to panel member keys and composes leave-one-out macro momentum.
*/
type macroSection struct {
	panel       statistic.Panel
	leaveOneOut *statistic.LeaveOneOutMedian
	macro       nomagique.Scalar
	memberKeys  sync.Map
	nextKey     float64
}

func init() {
	crossSection.leaveOneOut = statistic.NewLeaveOneOutMedian(&crossSection.panel)

	macro, err := nomagique.Number(crossSection.leaveOneOut)

	if err != nil {
		panic(err)
	}

	crossSection.macro = macro
}

func (section *macroSection) Observe(symbol string, changePct float64) {
	memberKey := section.memberKey(symbol)

	section.panel.Observe(
		nomagique.Scalar(memberKey),
		nomagique.Scalar(changePct),
	)
}

func (section *macroSection) MacroMomentum(symbol string) float64 {
	memberKey := section.memberKey(symbol)

	return float64(nomagique.Scalar(memberKey).Observe(section.macro))
}

func (section *macroSection) memberKey(symbol string) float64 {
	raw, loaded := section.memberKeys.Load(symbol)

	if loaded {
		return raw.(float64)
	}

	section.nextKey++
	memberKey := section.nextKey
	section.memberKeys.Store(symbol, memberKey)

	return memberKey
}

func (section *macroSection) reset() {
	section.panel.Reset()
	section.memberKeys = sync.Map{}
	section.nextKey = 0
}
