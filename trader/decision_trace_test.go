package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	decision "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestRecordEntryVerdictsAudit(t *testing.T) {
	Convey("Given a desk with disk audit logging", t, func() {
		path := t.TempDir() + "/audit.jsonl"
		auditLog, err := OpenAuditLog(path, 1<<20, 3, 0)
		So(err, ShouldBeNil)
		defer auditLog.Close()

		crypto := newTestCrypto()
		crypto.auditLog = auditLog

		trace := perspectives.AcquireTrace(perspectives.PlaybookTrend)
		trace.RecordStep(
			perspectives.CategoryAggressiveDrive,
			perspectives.ActionEnter,
			2.4,
			1.35,
			perspectives.ConditionIsGreaterThan,
			1,
			true,
		)
		trace.RecordStep(
			perspectives.CategoryToxicBluff,
			perspectives.ActionDeny,
			2.0,
			1.35,
			perspectives.ConditionIsGreaterThan,
			0,
			true,
		)

		crypto.recordEntryVerdicts("BTC/EUR", nil, []decision.EntryVerdict{{
			Name:   "trend",
			Regime: perspectives.RegimeTrending,
			Action: perspectives.ActionDeny,
			Trace:  trace,
		}})

		allowTrace := perspectives.AcquireTrace(perspectives.PlaybookDrive)
		allowTrace.RecordStep(
			perspectives.CategoryAggressiveDrive,
			perspectives.ActionEnter,
			3.1,
			1.35,
			perspectives.ConditionIsGreaterThan,
			0,
			true,
		)

		crypto.recordEntryVerdicts("ETH/EUR", nil, []decision.EntryVerdict{{
			Name:   "drive",
			Regime: perspectives.RegimeTrending,
			Action: perspectives.ActionEnter,
			Trace:  allowTrace,
		}})

		lines, readErr := readAuditLines(path)

		Convey("It should persist gate rejects with full trace paths", func() {
			So(readErr, ShouldBeNil)
			So(lines, ShouldHaveLength, 2)
			So(lines[0]["audit_event"], ShouldEqual, "gate_reject")
			So(lines[0]["reason"], ShouldEqual, "toxic_bluff_deny")

			traceSteps, ok := lines[0]["trace"].([]any)
			So(ok, ShouldBeTrue)
			So(traceSteps, ShouldHaveLength, 2)
		})

		Convey("It should persist perspective allows", func() {
			So(lines[1]["audit_event"], ShouldEqual, "perspective_allow")
			So(lines[1]["playbook"], ShouldEqual, "drive")
		})

		perspectives.ReleaseTrace(trace)
		perspectives.ReleaseTrace(allowTrace)
	})
}

func TestPublishEntryReject(t *testing.T) {
	Convey("Given a desk with disk audit logging", t, func() {
		path := t.TempDir() + "/audit.jsonl"
		auditLog, err := OpenAuditLog(path, 1<<20, 3, 0)
		So(err, ShouldBeNil)
		defer auditLog.Close()

		crypto := newTestCrypto()
		crypto.auditLog = auditLog

		crypto.publishEntryReject("SOL/EUR", "edge_below_baseline", map[string]any{
			"score":    2.5,
			"baseline": 3.0,
			"edge":     -0.5,
		})

		lines, readErr := readAuditLines(path)

		Convey("It should persist desk-level entry rejects", func() {
			So(readErr, ShouldBeNil)
			So(lines, ShouldHaveLength, 1)
			So(lines[0]["audit_event"], ShouldEqual, "entry_reject")
			So(lines[0]["reason"], ShouldEqual, "edge_below_baseline")
			So(lines[0]["edge"], ShouldEqual, -0.5)
		})
	})
}

func BenchmarkTraceStepsWire(b *testing.B) {
	trace := perspectives.AcquireTrace(perspectives.PlaybookTrend)

	for index := range 8 {
		trace.RecordStep(
			perspectives.CategoryAggressiveDrive,
			perspectives.ActionEnter,
			float64(index)+1,
			1.35,
			perspectives.ConditionIsGreaterThan,
			index,
			true,
		)
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = traceStepsWire(trace)
	}

	perspectives.ReleaseTrace(trace)
}
