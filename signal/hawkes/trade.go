package hawkes

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Trade conditions precise trade arrival times into numerical point-process
evidence. Evidence formatting is composed separately so estimation cannot
quietly acquire category or strategy responsibilities.
*/
type Trade struct {
	sample   *excitation.Sample
	process  *excitation.Process
	evidence *Evidence
}

/*
NewTrade returns a symbol-local Hawkes trade measurement pipeline.
*/
func NewTrade() *Trade {
	return &Trade{
		sample:   excitation.NewSample(),
		process:  excitation.NewProcess(),
		evidence: NewEvidence(),
	}
}

/*
Measure updates the marked arrival stream and emits every numerical quantity
supported by the estimator's current readiness level.
*/
func (trade *Trade) Measure(row kraken.TradeData) ([]*types.Measurement, error) {
	input, ready, err := trade.sample.MeasureArrival(excitation.TradeInput{
		Symbol:    row.Symbol,
		Side:      row.Side,
		Timestamp: row.Timestamp,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	if !ready {
		return nil, nil
	}

	output, ready, err := trade.process.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	if !ready {
		return nil, nil
	}

	return trade.evidence.Measure(row.Symbol, output), nil
}
