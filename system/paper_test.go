package system_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestPaper(t *testing.T) {
	Convey("Given a mock kraken CLI script", t, func() {
		tmpDir, err := os.MkdirTemp("", "paper-test-*")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tmpDir)

		mockScript := filepath.Join(tmpDir, "kraken")
		scriptContent := `#!/bin/sh
if [ "$1" = "paper" ] && [ "$2" = "balance" ]; then
    echo '{"balances":{"USD":{"available":"200.00","hold":"0.00","total":"200.00"}}}'
    exit 0
fi
if [ "$1" = "paper" ] && [ "$2" = "status" ]; then
    echo '{"starting_balance":200.0,"starting_currency":"USD"}'
    exit 0
fi
if [ "$1" = "paper" ] && [ "$2" = "buy" ]; then
    echo '{"order_id":"test-123","status":"filled","pair":"BTC/USD"}'
    exit 0
fi
echo '{"error":"unknown command"}'
exit 1
`
		err = os.WriteFile(mockScript, []byte(scriptContent), 0755)
		So(err, ShouldBeNil)

		origBin := os.Getenv("KRAKEN_BIN")
		_ = os.Setenv("KRAKEN_BIN", mockScript)
		defer func() {
			if origBin != "" {
				_ = os.Setenv("KRAKEN_BIN", origBin)
			}
			if origBin == "" {
				_ = os.Unsetenv("KRAKEN_BIN")
			}
		}()

		var lastBalance map[string]any
		var lastExec map[string]any

		paper := system.NewPaper(
			context.Background(),
			func(b map[string]any) { lastBalance = b },
			func(e map[string]any) { lastExec = e },
		)
		defer paper.Close()

		Convey("When requesting balance", func() {
			bal, err := paper.Balance()
			So(err, ShouldBeNil)
			So(bal, ShouldNotBeNil)
			So(lastBalance, ShouldNotBeNil)

			balances, ok := bal["balances"].(map[string]any)
			So(ok, ShouldBeTrue)
			usd, ok := balances["USD"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(usd["available"], ShouldEqual, "200.00")
			So(usd["total"], ShouldEqual, "200.00")
		})

		Convey("When requesting status", func() {
			st, err := paper.Status()
			So(err, ShouldBeNil)
			So(st, ShouldNotBeNil)
			So(st["starting_balance"], ShouldEqual, 200.0)
			So(st["starting_currency"], ShouldEqual, "USD")
		})

		Convey("When submitting buy order", func() {
			execRes, err := paper.Execute("buy", "BTC/USD", 0.01, 50000.0)
			So(err, ShouldBeNil)
			So(execRes, ShouldNotBeNil)
			So(execRes["order_id"], ShouldEqual, "test-123")
			So(execRes["status"], ShouldEqual, "filled")
			So(lastExec, ShouldNotBeNil)
		})
	})
}
