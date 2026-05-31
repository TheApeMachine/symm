package trader

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/focus"
	decision "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/trader/economics"
	"github.com/theapemachine/symm/wallet"
)

func newTestCrypto() *Crypto {
	ctx := context.Background()

	return &Crypto{
		ctx:       ctx,
		wallet:    wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26),
		tracker:   focus.NewSet(),
		story:     decision.NewStory(),
		positions: newPositionBook(),
		quotes:    newQuoteCache(),
		economics: economics.NewDesk(),
		paper:     NewPaperSession(ctx),
		makers:    newMakerDesk(),
		readings:  make(map[string]map[perspectives.SourceType]timedMeasurement),
		runtime:   config.Runtime,
	}
}

func traderMeasurement(
	symbol string,
	source perspectives.SourceType,
	category perspectives.CategoryType,
	snr float64,
) perspectives.Measurement {
	return perspectives.Measurement{
		Symbol:   symbol,
		Source:   source,
		Category: category,
		SNR:      snr,
		Last:     100,
	}
}

func TestCryptoRecord(t *testing.T) {
	convey.Convey("Given two contradictory categories from the same source", t, func() {
		crypto := newTestCrypto()
		crypto.record(traderMeasurement(
			"BTC/EUR",
			perspectives.SourceCVD,
			perspectives.CategoryAggressiveDrive,
			2.0,
		))
		crypto.record(traderMeasurement(
			"BTC/EUR",
			perspectives.SourceCVD,
			perspectives.CategoryStochasticBalance,
			0.2,
		))

		convey.Convey("It should keep only the newest category for that source", func() {
			measurements := crypto.snapshot("BTC/EUR")

			convey.So(measurements, convey.ShouldHaveLength, 1)
			convey.So(measurements[0].Category, convey.ShouldEqual, perspectives.CategoryStochasticBalance)
		})
	})
}

func TestCryptoMarketCalibrated(t *testing.T) {
	convey.Convey("Given the whole observed market emits the same entry signal", t, func() {
		crypto := newTestCrypto()

		for index := range 20 {
			symbol := fmt.Sprintf("COIN%d/EUR", index)
			crypto.record(traderMeasurement(
				symbol,
				perspectives.SourceCVD,
				perspectives.CategoryAggressiveDrive,
				2.0,
			))
		}

		candidate, ok := crypto.entryOpportunity("COIN0/EUR", crypto.snapshot("COIN0/EUR"))

		convey.Convey("It should identify the local playbook but reject it as non-distinct", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(crypto.marketCalibrated(candidate), convey.ShouldBeFalse)
		})
	})

	convey.Convey("Given one symbol stands above the live cross-section", t, func() {
		crypto := newTestCrypto()

		for index := range 19 {
			symbol := fmt.Sprintf("COIN%d/EUR", index)
			crypto.record(traderMeasurement(
				symbol,
				perspectives.SourceCVD,
				perspectives.CategoryStochasticBalance,
				0.2,
			))
		}

		crypto.record(traderMeasurement(
			"LEADER/EUR",
			perspectives.SourceCVD,
			perspectives.CategoryAggressiveDrive,
			3.0,
		))

		candidate, ok := crypto.entryOpportunity("LEADER/EUR", crypto.snapshot("LEADER/EUR"))
		calibrated, allowed := crypto.calibrateOpportunity(candidate)

		convey.Convey("It should allow the outlier and assign it positive edge", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(allowed, convey.ShouldBeTrue)
			convey.So(calibrated.Edge, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestCryptoOpportunityShare(t *testing.T) {
	convey.Convey("Given two live opportunities with unequal edge", t, func() {
		crypto := newTestCrypto()

		for index := range 18 {
			symbol := fmt.Sprintf("COIN%d/EUR", index)
			crypto.record(traderMeasurement(
				symbol,
				perspectives.SourceCVD,
				perspectives.CategoryStochasticBalance,
				0.2,
			))
		}

		crypto.record(traderMeasurement(
			"FAST/EUR",
			perspectives.SourceCVD,
			perspectives.CategoryAggressiveDrive,
			4.0,
		))
		crypto.record(traderMeasurement(
			"SLOW/EUR",
			perspectives.SourceCVD,
			perspectives.CategoryAggressiveDrive,
			2.0,
		))

		fast, _ := crypto.entryOpportunity("FAST/EUR", crypto.snapshot("FAST/EUR"))
		slow, _ := crypto.entryOpportunity("SLOW/EUR", crypto.snapshot("SLOW/EUR"))
		fast, _ = crypto.calibrateOpportunity(fast)
		slow, _ = crypto.calibrateOpportunity(slow)

		convey.Convey("It should give the stronger candidate the larger wallet share", func() {
			convey.So(crypto.opportunityShare(fast), convey.ShouldBeGreaterThan, crypto.opportunityShare(slow))
		})
	})
}

func TestPublishAudit(t *testing.T) {
	convey.Convey("Given a crypto desk wired to the ui bus", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		crypto := newTestCrypto()
		crypto.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
		subscription := crypto.ui.Subscribe("test:audit", 16)

		crypto.publishAudit("gate_reject", "BTC/EUR", "systemic_slump_wait", nil)
		crypto.publishAudit("entry_submit", "BTC/EUR", "submitting", nil)
		crypto.publishAudit("entry", "BTC/EUR", "filled", nil)
		crypto.publishAudit("forward", "BTC/EUR", "matured", nil)
		crypto.publishAudit("exit", "ETH/EUR", "closed", nil)

		events := make([]string, 0, 2)
		deadline := time.After(200 * time.Millisecond)

		for len(events) < 2 {
			select {
			case value := <-subscription.Incoming:
				frame, ok := value.Value.(map[string]any)

				if !ok {
					continue
				}

				event, ok := frame["audit_event"].(string)

				if ok {
					events = append(events, event)
				}
			case <-deadline:
				t.Fatalf("timed out waiting for audit frames, got %v", events)
			}
		}

		convey.Convey("It should publish only entry and exit lifecycle audits", func() {
			convey.So(events, convey.ShouldResemble, []string{"entry", "exit"})
		})
	})

	convey.Convey("Given a crypto desk with disk audit logging enabled", t, func() {
		path := t.TempDir() + "/audit.jsonl"
		auditLog, err := OpenAuditLog(path, 1<<20, 3, time.Second)
		convey.So(err, convey.ShouldBeNil)
		defer auditLog.Close()

		crypto := newTestCrypto()
		crypto.auditLog = auditLog

		crypto.publishAudit("gate_reject", "BTC/EUR", "systemic_slump_wait", map[string]any{
			"playbook": "trend",
		})
		crypto.publishAudit("gate_reject", "BTC/EUR", "systemic_slump_wait", map[string]any{
			"playbook": "trend",
		})
		crypto.publishAudit("entry", "BTC/EUR", "filled", nil)

		lines, readErr := readAuditLines(path)

		convey.Convey("It should log gate rejects with dedupe and lifecycle events", func() {
			convey.So(readErr, convey.ShouldBeNil)
			convey.So(lines, convey.ShouldHaveLength, 2)
			convey.So(lines[0]["audit_event"], convey.ShouldEqual, "gate_reject")
			convey.So(lines[1]["audit_event"], convey.ShouldEqual, "entry")
		})
	})
}

func BenchmarkCryptoMarketCalibrated(b *testing.B) {
	crypto := newTestCrypto()

	for index := range 64 {
		symbol := fmt.Sprintf("COIN%d/EUR", index)
		strength := 0.2

		if index == 7 {
			strength = 3.0
		}

		category := perspectives.CategoryStochasticBalance

		if index == 7 {
			category = perspectives.CategoryAggressiveDrive
		}

		crypto.record(traderMeasurement(symbol, perspectives.SourceCVD, category, strength))
	}

	candidate, _ := crypto.entryOpportunity("COIN7/EUR", crypto.snapshot("COIN7/EUR"))

	for b.Loop() {
		_ = crypto.marketCalibrated(candidate)
	}
}
