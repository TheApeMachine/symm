package data

import (
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPersistCloneStripsPeers(t *testing.T) {
	Convey("PersistClone copies maps but drops the peer forest", t, func() {
		root := NewMeasurement[float64]("websocket", map[string]Metric[float64]{
			"price": {Raw: 100, Label: "price"},
		})
		root.Label = "BTC/USD"
		root.At = time.Unix(1, 0)

		peer := NewMeasurement[float64]("hawkes:trade", map[string]Metric[float64]{
			"arrival_rate": {Raw: 1.5, Label: "arrival_rate"},
		})
		peer.Label = "BTC/USD"
		root.Contribute(peer)

		So(len(root.Peers), ShouldEqual, 1)

		snap := root.PersistClone()
		So(snap, ShouldNotBeNil)
		So(snap.Peers, ShouldBeNil)
		So(snap.GetMetric("price").Raw, ShouldEqual, 100)
		So(len(root.Peers), ShouldEqual, 1)
	})
}

func TestContributeDoesNotDumpMetricsOntoShared(t *testing.T) {
	Convey("Contribute attaches Source-keyed peers without source-qualified metric flood", t, func() {
		shared := NewMeasurement[float64]("websocket", map[string]Metric[float64]{
			"price": {Raw: 100, Label: "price"},
		})
		shared.Label = "BTC/USD"

		owned := shared.Fork()
		owned.SetSource("hawkes:trade")
		owned.WriteMetric("arrival_rate", 2.0)

		shared.Contribute(owned)

		So(len(shared.Peers), ShouldEqual, 1)
		So(shared.Peers[0].Source, ShouldEqual, "hawkes:trade")
		_, flooded := shared.LookupMetric("hawkes:trade/arrival_rate")
		So(flooded, ShouldBeFalse)
		_, hasPrice := shared.LookupMetric("price")
		So(hasPrice, ShouldBeTrue)
	})
}

func TestContributeReplacesSameSource(t *testing.T) {
	Convey("Contribute replaces an existing peer with the same Source", t, func() {
		shared := NewMeasurement[float64]("websocket", nil)
		shared.Label = "BTC/USD"

		first := shared.Fork()
		first.SetSource("cvd:trade")
		first.WriteMetric("x", 1)
		shared.Contribute(first)

		second := shared.Fork()
		second.SetSource("cvd:trade")
		second.WriteMetric("x", 2)
		shared.Contribute(second)

		So(len(shared.Peers), ShouldEqual, 1)
		So(shared.Peers[0].GetMetric("x").Raw, ShouldEqual, 2)
	})
}

func TestForkContributeRace(t *testing.T) {
	shared := NewMeasurement[float64]("websocket", map[string]Metric[float64]{
		"price": {Raw: 1},
	})
	shared.Label = "ETH/USD"

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-start
			for n := 0; n < 100; n++ {
				owned := shared.Fork()
				owned.SetSource("signal:" + string(rune('a'+id%8)))
				owned.WriteMetric("v", float64(n))
				shared.Contribute(owned)
				_ = shared.PersistClone()
			}
		}(i)
	}
	close(start)
	wg.Wait()
}

func TestContributeMergeDeterministicUnderConcurrency(t *testing.T) {
	Convey("Concurrent Fork→Contribute merges Source-keyed peers and quality mins deterministically", t, func() {
		shared := NewMeasurement[float64]("websocket", map[string]Metric[float64]{
			"price": {Raw: 100},
		})
		shared.Label = "BTC/USD"
		shared.SetProvenance("ingress_channel", "ticker")
		shared.SetProvenance("channel", "ticker")

		sources := []string{"liquidity:ticker", "toxicity:level3", "cvd:trade", "hawkes:trade"}
		var wg sync.WaitGroup
		start := make(chan struct{})
		for _, source := range sources {
			wg.Add(1)
			go func(src string) {
				defer wg.Done()
				<-start
				for n := 0; n < 50; n++ {
					owned := shared.Fork()
					owned.SetSource(src)
					owned.SetQuality(0.5+float64(n%3)*0.1, 2.0+float64(n%5), true, false)
					owned.WriteMetric("v", float64(n))
					shared.Contribute(owned)
				}
			}(source)
		}
		close(start)
		wg.Wait()

		So(shared.GetSource(), ShouldEqual, "websocket")
		So(len(shared.Peers), ShouldEqual, len(sources))
		seen := map[string]bool{}
		for _, peer := range shared.Peers {
			So(peer, ShouldNotBeNil)
			So(peer.Source, ShouldNotBeEmpty)
			So(seen[peer.Source], ShouldBeFalse)
			seen[peer.Source] = true
			So(peer.Peers, ShouldBeNil)
		}
		for _, src := range sources {
			So(seen[src], ShouldBeTrue)
		}
		// Quality mins are order-invariant; maturity/SNR must be the global min.
		So(shared.Maturity, ShouldBeGreaterThan, 0)
		So(shared.SNRDefined, ShouldBeTrue)
		So(shared.SNR, ShouldBeGreaterThan, 0)
		// Shared ingress provenance survives Contribute.
		ch, ok := shared.GetProvenance("ingress_channel")
		So(ok, ShouldBeTrue)
		So(ch, ShouldEqual, "ticker")
	})
}

func TestForkDoesNotMutateSharedSource(t *testing.T) {
	Convey("Stage consumer SetSource on Fork never rewrites the shared slot Source", t, func() {
		shared := NewMeasurement[float64]("websocket", nil)
		shared.Label = "ETH/USD"
		shared.SetProvenance("ingress_channel", "trade")

		owned := shared.Fork()
		owned.SetSource("correlation:ticker")
		owned.SetQuality(0.9, 3, true, false)
		shared.Contribute(owned)

		So(shared.GetSource(), ShouldEqual, "websocket")
		So(owned.GetSource(), ShouldEqual, "correlation:ticker")
		So(len(shared.Peers), ShouldEqual, 1)
	})
}

func TestStampIntervalRejectsInvertedFrom(t *testing.T) {
	Convey("StampInterval drops From when it would land after At", t, func() {
		m := NewMeasurement[float64]("toxicity:level3", nil)
		at := time.Unix(10, 0)
		from := time.Unix(12, 0)
		StampInterval(m, at, from)
		So(m.At, ShouldEqual, at)
		So(m.From.IsZero(), ShouldBeTrue)

		StampInterval(m, at, time.Unix(8, 0))
		So(m.From, ShouldEqual, time.Unix(8, 0))
	})
}
