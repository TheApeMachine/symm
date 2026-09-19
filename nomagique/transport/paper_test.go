package transport_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/transport"
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

		paper := transport.NewPaper(
			context.Background(),
			func(b map[string]any) { lastBalance = b },
			func(e map[string]any) { lastExec = e },
		)
		defer paper.Close()

		Convey("When requesting balance", func() {
			bal, err := paper.Balance()

			Convey("Then balances are correctly parsed and onBalance callback invoked", func() {
				So(err, ShouldBeNil)
				So(bal, ShouldNotBeNil)
				So(lastBalance, ShouldNotBeNil)
				balances, ok := bal["balances"].(map[string]any)
				So(ok, ShouldBeTrue)
				usd, ok := balances["USD"].(map[string]any)
				So(ok, ShouldBeTrue)
				So(usd["total"], ShouldEqual, "200.00")
			})
		})

		Convey("When requesting status", func() {
			status, err := paper.Status()

			Convey("Then status is correctly returned", func() {
				So(err, ShouldBeNil)
				So(status["starting_currency"], ShouldEqual, "USD")
				So(status["starting_balance"], ShouldEqual, 200.0)
			})
		})

		Convey("When submitting buy order", func() {
			res, err := paper.Buy("BTC/USD", "0.001", "")

			Convey("Then execution is returned and onExecution invoked", func() {
				So(err, ShouldBeNil)
				So(res["order_id"], ShouldEqual, "test-123")
				So(lastExec["order_id"], ShouldEqual, "test-123")
			})
		})
	})
}
