package eval

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/engine"
)

/*
Record is one settled forecast used for offline calibration reports.
*/
type Record struct {
	Signal          string
	Symbol          string
	Regime          string
	Reason          string
	Confidence      float64
	PredictedReturn float64
	ActualReturn    float64
	Error           float64
	Hit             bool
	SettledAt       time.Time
	Unanchored      bool
}

/*
Collector stores settled prediction feedback from one replay run.
*/
type Collector struct {
	mu      sync.Mutex
	records []Record
}

/*
NewCollector creates an empty feedback collector.
*/
func NewCollector() *Collector {
	return &Collector{
		records: make([]Record, 0, 256),
	}
}

/*
Sink returns a callback suitable for trader.Crypto.BindFeedbackSink.
*/
func (collector *Collector) Sink() func(engine.PredictionFeedback) {
	return collector.Record
}

/*
Record ingests one settled forecast.
*/
func (collector *Collector) Record(feedback engine.PredictionFeedback) {
	if feedback.Source == "" || feedback.Unanchored {
		return
	}

	record := Record{
		Signal:          feedback.Source,
		Symbol:          feedback.Symbol,
		Regime:          feedback.Regime,
		Reason:          feedback.Reason,
		Confidence:      feedback.Confidence,
		PredictedReturn: feedback.PredictedReturn,
		ActualReturn:    feedback.ActualReturn,
		Error:           feedback.Error,
		Hit:             forecastHit(feedback),
		SettledAt:       feedback.SettledAt,
	}

	collector.mu.Lock()
	collector.records = append(collector.records, record)
	collector.mu.Unlock()
}

/*
Records returns a snapshot of collected settlements.
*/
func (collector *Collector) Records() []Record {
	collector.mu.Lock()
	defer collector.mu.Unlock()

	return append([]Record(nil), collector.records...)
}

func forecastHit(feedback engine.PredictionFeedback) bool {
	if feedback.PredictedReturn <= 0 {
		return false
	}

	return feedback.ActualReturn >= 0
}
