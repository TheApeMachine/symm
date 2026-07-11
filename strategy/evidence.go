package strategy

import (
	"time"

	"github.com/theapemachine/datura"
)

/*
Evidence is a snapshot of every datapoint or other information that
was used to formulate a Thesis. It is primarily used during the
generation of the PostMortem, so the system can map the Thesis to
ground truth and extract highly granular learnings from that process.
*/
type Evidence struct {
	Source    string
	Symbol    string
	Timestamp time.Time
	Values    datura.Map[float64]
	Snapshot  any
}

/*
NewEvidence wraps a snapshot in the Evidence type.
*/
func NewEvidence(source string, symbol string, snapshot any) *Evidence {
	return &Evidence{
		Source:    source,
		Symbol:    symbol,
		Timestamp: time.Now(),
		Snapshot:  snapshot,
	}
}
