package leadlag

import (
	"math"
	"sync"
	"time"

	nmcorrelation "github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
)

const cohortLiftWindow = 10 * time.Second

/*
cohortDirectionLift is the share of benchmark peers whose recent return sign
matches the focal symbol. Unlike led_peer_share (temporal leadership), this
measures directional support from the cohort.
*/
func cohortDirectionLift(
	focalPath []nmcorrelation.Sample,
	paths *sync.Map,
	focalSymbol string,
	now time.Time,
) (lift float64, ok bool) {
	focalRet, focalOK := returnOver(focalPath, now, cohortLiftWindow)
	if !focalOK || focalRet == 0 {
		return 0, false
	}

	focalUp := focalRet > 0
	matched := 0
	total := 0

	paths.Range(func(peerSymbol, peerPath any) bool {
		symbol := peerSymbol.(string)
		if symbol == focalSymbol {
			return true
		}

		path := peerPath.([]nmcorrelation.Sample)
		if len(path) < 3 {
			return true
		}

		ret, retOK := returnOver(path, now, cohortLiftWindow)
		if !retOK || ret == 0 {
			return true
		}

		total++
		if (ret > 0) == focalUp {
			matched++
		}

		return true
	})

	if total == 0 {
		return 0, false
	}

	return float64(matched) / float64(total), true
}

func (signal *Signal) appendLagDispersion(value *data.Value) {
	if signal.Status() != runtime.READY || value == nil {
		return
	}

	var shares []float64
	signal.lastLedShare.Range(func(_, untyped any) bool {
		if share, ok := untyped.(float64); ok {
			shares = append(shares, share)
		}
		return true
	})

	if len(shares) < 2 {
		return
	}

	mean := 0.0
	for _, share := range shares {
		mean += share
	}
	mean /= float64(len(shares))

	variance := 0.0
	for _, share := range shares {
		delta := share - mean
		variance += delta * delta
	}
	variance /= float64(len(shares) - 1)

	dispersion := math.Sqrt(variance)
	if dispersion <= 0 {
		return
	}

	signal.emit.Metric(value, []float64{dispersion}, append([]string{
		signal.Name(),
		"aggregate",
	}, "lag_dispersion")...)
}
