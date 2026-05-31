package perspectives

import "sync"

const maxTraceSteps = 16

/*
TraceStep is one branch evaluation along a playbook decision path.
*/
type TraceStep struct {
	Category  CategoryType
	Metric    string
	Action    ActionType
	SNR       float64
	Threshold float64
	Condition ConditionType
	Depth     int
	Matched   bool
}

/*
DecisionTrace records the playbook path that produced a flat-entry verdict.
Traces are pooled to avoid per-tick allocations on the measurement hot path.
*/
type DecisionTrace struct {
	Playbook    PlaybookName
	FinalAction ActionType
	Steps       [maxTraceSteps]TraceStep
	stepCount   int
}

var tracePool = sync.Pool{
	New: func() any {
		return &DecisionTrace{}
	},
}

/*
AcquireTrace borrows a pooled trace for one playbook evaluation.
*/
func AcquireTrace(playbook PlaybookName) *DecisionTrace {
	trace, ok := tracePool.Get().(*DecisionTrace)

	if !ok || trace == nil {
		trace = &DecisionTrace{}
	}

	trace.reset(playbook)

	return trace
}

/*
ReleaseTrace returns a trace to the pool.
*/
func ReleaseTrace(trace *DecisionTrace) {
	if trace == nil {
		return
	}

	trace.reset("")
	tracePool.Put(trace)
}

func (trace *DecisionTrace) reset(playbook PlaybookName) {
	trace.Playbook = playbook
	trace.FinalAction = ActionNone
	trace.stepCount = 0
}

/*
RecordStep appends one branch evaluation when capacity remains.
*/
func (trace *DecisionTrace) RecordStep(
	category CategoryType,
	action ActionType,
	snr float64,
	threshold float64,
	condition ConditionType,
	depth int,
	matched bool,
) {
	trace.RecordTraceStep(TraceStep{
		Category:  category,
		Action:    action,
		SNR:       snr,
		Threshold: threshold,
		Condition: condition,
		Depth:     depth,
		Matched:   matched,
	})
}

func (trace *DecisionTrace) RecordTraceStep(step TraceStep) {
	if trace == nil || trace.stepCount >= maxTraceSteps {
		return
	}

	trace.Steps[trace.stepCount] = step
	trace.stepCount++
}

/*
StepsSlice returns the recorded steps without exposing the fixed array.
*/
func (trace *DecisionTrace) StepsSlice() []TraceStep {
	if trace == nil || trace.stepCount == 0 {
		return nil
	}

	steps := make([]TraceStep, trace.stepCount)
	copy(steps, trace.Steps[:trace.stepCount])

	return steps
}

/*
LastStep returns the deepest recorded step, if any.
*/
func (trace *DecisionTrace) LastStep() (TraceStep, bool) {
	if trace == nil || trace.stepCount == 0 {
		return TraceStep{}, false
	}

	return trace.Steps[trace.stepCount-1], true
}

/*
ActionLabel returns the dashboard label for an action.
*/
func ActionLabel(action ActionType) string {
	switch action {
	case ActionEnter:
		return "enter"
	case ActionDeny:
		return "deny"
	case ActionWait:
		return "wait"
	case ActionStopLoss:
		return "stop_loss"
	case ActionTakeProfit:
		return "take_profit"
	case ActionShort:
		return "short"
	default:
		return "none"
	}
}
