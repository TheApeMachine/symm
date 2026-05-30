package economics

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestLedgerAllowsEntryColdStart(t *testing.T) {
	convey.Convey("Given a cold playbook", t, func() {
		ledger := NewLedger()

		convey.Convey("It should allow entry to gather samples", func() {
			convey.So(ledger.AllowsEntry(string(perspectives.PlaybookTrend)), convey.ShouldBeTrue)
		})
	})
}

func TestLedgerAllowsEntryBlocksNegativeEdge(t *testing.T) {
	convey.Convey("Given enough negative net returns", t, func() {
		originalMin := config.System.ForwardReturnMinSamples
		config.System.ForwardReturnMinSamples = 5
		t.Cleanup(func() { config.System.ForwardReturnMinSamples = originalMin })

		ledger := NewLedger()

		for range 6 {
			ledger.RecordNet("trend", -0.01)
		}

		convey.Convey("It should block further entries", func() {
			convey.So(ledger.AllowsEntry("trend"), convey.ShouldBeFalse)
		})
	})
}

func TestLedgerAllowsEntryPositiveEdge(t *testing.T) {
	convey.Convey("Given enough positive net returns", t, func() {
		originalMin := config.System.ForwardReturnMinSamples
		config.System.ForwardReturnMinSamples = 5
		t.Cleanup(func() { config.System.ForwardReturnMinSamples = originalMin })

		ledger := NewLedger()

		for range 6 {
			ledger.RecordNet("trend", 0.02)
		}

		convey.Convey("It should allow entries", func() {
			convey.So(ledger.AllowsEntry("trend"), convey.ShouldBeTrue)
		})
	})
}

func BenchmarkLedgerAllowsEntry(b *testing.B) {
	ledger := NewLedger()

	for index := range 40 {
		if index%2 == 0 {
			ledger.RecordNet("trend", 0.01)
			continue
		}

		ledger.RecordNet("trend", -0.005)
	}

	b.ResetTimer()

	for b.Loop() {
		_ = ledger.AllowsEntry("trend")
	}
}
