package kraken

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestExtendedBalanceAvailable(t *testing.T) {
	Convey("Given full quote balances with external holds and credit", t, func() {
		payload := []byte(`{"error":[],"result":{"ZUSD":{"balance":"150","hold_trade":"30.25","credit":"500","credit_used":"10.5"}}}`)
		normalize := func(asset string) string {
			if asset == "ZUSD" {
				return "USD"
			}
			return asset
		}
		balance, err := NewExtendedBalance(payload)
		So(err, ShouldBeNil)
		available, err := balance.Available("USD", normalize)
		So(err, ShouldBeNil)
		So(available.String(), ShouldEqual, "109.25")
		Convey("an absent asset in a complete map is a known zero holding", func() {
			available, err := balance.Available("EUR", normalize)
			So(err, ShouldBeNil)
			So(available.Sign(), ShouldEqual, 0)
		})
		Convey("a missing held amount must not become spendable cash", func() {
			row := balance.Result["ZUSD"]
			row.Hold = nil
			balance.Result["ZUSD"] = row
			_, err := balance.Available("USD", normalize)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestNewExtendedBalance(t *testing.T) {
	Convey("Given unavailable or malformed exchange replies", t, func() {
		for _, payload := range []string{`{}`, `{"error":["EAPI:Invalid key"],"result":{}}`, `{"result":{"USD":{"balance":"bad"}}}`} {
			_, err := NewExtendedBalance([]byte(payload))
			So(err, ShouldNotBeNil)
		}
	})
}

func BenchmarkExtendedBalanceAvailable(b *testing.B) {
	balance, err := NewExtendedBalance([]byte(`{"error":[],"result":{"USD":{"balance":"150.00","hold_trade":"30.25"}}}`))

	if err != nil {
		b.Fatal(err)
	}
	normalize := func(asset string) string { return asset }
	b.ReportAllocs()
	for b.Loop() {
		if _, err := balance.Available("USD", normalize); err != nil {
			b.Fatal(err)
		}
	}
}
