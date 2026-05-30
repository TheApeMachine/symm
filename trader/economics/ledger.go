package economics

import (
	"math"
	"sync"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
)

const maxPlaybookSamples = 512

/*
Ledger accumulates post-fee net returns per playbook for entry gating.
*/
type Ledger struct {
	mu      sync.Mutex
	samples map[string][]float64
}

/*
NewLedger instantiates an empty playbook economics ledger.
*/
func NewLedger() *Ledger {
	return &Ledger{samples: make(map[string][]float64)}
}

/*
RecordNet appends one net return observation for a playbook.
*/
func (ledger *Ledger) RecordNet(playbook string, netReturn float64) {
	if playbook == "" {
		return
	}

	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	bucket := ledger.samples[playbook]
	bucket = append(bucket, netReturn)

	if len(bucket) > maxPlaybookSamples {
		bucket = bucket[len(bucket)-maxPlaybookSamples:]
	}

	ledger.samples[playbook] = bucket
}

/*
AllowsEntry returns whether a playbook may open new risk. Cold playbooks
gather samples; warm playbooks require positive net edge at significance Z.
*/
func (ledger *Ledger) AllowsEntry(playbook string) bool {
	if !config.System.ExecutionEconomicsEnabled {
		return true
	}

	minSamples := config.System.ForwardReturnMinSamples

	if playbook == string(perspectives.PlaybookPump) {
		minSamples = config.System.PumpForwardReturnMinSamples
	}

	ledger.mu.Lock()
	bucket := append([]float64(nil), ledger.samples[playbook]...)
	ledger.mu.Unlock()

	if len(bucket) < minSamples {
		return true
	}

	mean := numeric.Mean(bucket)

	if mean <= 0 {
		return false
	}

	z := oneSampleZ(mean, bucket)

	return z >= config.System.ForwardReturnSignificanceZ
}

/*
Stats returns sample count and mean net return for one playbook.
*/
func (ledger *Ledger) Stats(playbook string) (count int, mean float64) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	bucket := ledger.samples[playbook]

	if len(bucket) == 0 {
		return 0, 0
	}

	return len(bucket), numeric.Mean(bucket)
}

func oneSampleZ(mean float64, values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	variance := 0.0

	for _, value := range values {
		delta := value - mean
		variance += delta * delta
	}

	variance /= float64(len(values) - 1)
	std := math.Sqrt(variance)

	if std <= 0 {
		if mean > 0 {
			return math.MaxFloat64
		}

		return 0
	}

	return mean / (std / math.Sqrt(float64(len(values))))
}
