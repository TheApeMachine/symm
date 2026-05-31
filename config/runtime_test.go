package config

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
)

func TestExecutionScopeFrom(t *testing.T) {
	convey.Convey("Given a config with execution fields", t, func() {
		cfg := &Config{
			QuoteCurrency:          "EUR",
			MaxSpreadBPS:           12,
			SnapshotFreshnessTTL:   150 * time.Millisecond,
			PaperOrderRejectRate:   0.1,
			ExecutionStressEnabled: true,
		}

		scope := ExecutionScopeFrom(cfg)

		convey.Convey("It should copy execution fields into an isolated scope", func() {
			convey.So(scope.QuoteCurrency, convey.ShouldEqual, "EUR")
			convey.So(scope.MaxSpreadBPS, convey.ShouldEqual, 12)
			convey.So(scope.SnapshotFreshnessTTL, convey.ShouldEqual, 150*time.Millisecond)
			convey.So(scope.PaperOrderRejectRate, convey.ShouldEqual, 0.1)
			convey.So(scope.ExecutionStressEnabled, convey.ShouldBeTrue)
		})
	})
}

func TestSyncRuntime(t *testing.T) {
	convey.Convey("Given a mutated system config", t, func() {
		original := System.MaxSpreadBPS
		t.Cleanup(func() {
			System.MaxSpreadBPS = original
			SyncRuntime()
		})

		System.MaxSpreadBPS = 42
		SyncRuntime()

		convey.Convey("It should refresh runtime execution scope", func() {
			convey.So(Runtime.Execution.MaxSpreadBPS, convey.ShouldEqual, 42)
		})
	})
}
