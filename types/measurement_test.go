package types

import (
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFilterLatestSourceEpochs(t *testing.T) {
	Convey("Given retained measurements from several source epochs", t, func() {
		oldLeadLag := &Measurement{
			ID: "old", Source: SourceLeadLag, Peer: "UNFI/USD", At: time.Unix(1, 0),
		}
		currentLeadLag := &Measurement{
			ID: "current", Source: SourceLeadLag, Peer: "SOSO/USD", At: time.Unix(2, 0),
		}
		currentPeer := &Measurement{
			ID: "current-peer", Source: SourceLeadLag, Peer: "TRU/USD", At: time.Unix(2, 0),
		}
		pumpDump := &Measurement{
			ID: "pumpdump", Source: SourcePumpDump, At: time.Unix(1, 0),
		}

		filtered := FilterLatestSourceEpochs([]*Measurement{
			oldLeadLag,
			currentLeadLag,
			currentPeer,
			pumpDump,
		})

		Convey("It retains the complete newest epoch independently for each source", func() {
			So(filtered, ShouldResemble, []*Measurement{
				currentLeadLag,
				currentPeer,
				pumpDump,
			})
		})
	})
}

func TestObservationCount(t *testing.T) {
	Convey("Given thesis measurements grouped by source", t, func() {
		measurements := &sync.Map{}
		measurements.Store(SourceCVD, []*Measurement{{Symbol: "BTC/USD"}})
		measurements.Store(SourceLiquidity, []*Measurement{{Symbol: "ETH/USD"}, {Symbol: "BTC/USD"}})

		Convey("It counts distinct symbols across slice-backed source rows", func() {
			So(ObservationCount(measurements), ShouldEqual, 2)
		})
	})
}

func BenchmarkFilterLatestSourceEpochs(b *testing.B) {
	measurements := make([]*Measurement, 0, 64)

	for index := range 32 {
		measurements = append(measurements,
			&Measurement{
				Source: SourceLeadLag,
				Peer:   fmt.Sprintf("PAIR-%d/USD", index),
				At:     time.Unix(int64(index), 0),
			},
			&Measurement{
				Source: SourcePumpDump,
				At:     time.Unix(int64(index), 0),
			},
		)
	}

	for b.Loop() {
		FilterLatestSourceEpochs(measurements)
	}
}
