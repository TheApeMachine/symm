package kraken

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewBalanceDataSlice(t *testing.T) {
	Convey("Given a balances channel snapshot frame", t, func() {
		total, err := decimal.NewFromString("200")
		So(err, ShouldBeNil)
		available, err := decimal.NewFromString("125")
		So(err, ShouldBeNil)
		reserved, err := decimal.NewFromString("75")
		So(err, ShouldBeNil)

		frame := Balance{
			Channel:  "balances",
			Type:     "snapshot",
			Sequence: 1,
			Data: []BalanceData{{
				Asset:      "USD",
				AssetClass: "currency",
				Balance:    *total,
				Available:  *available,
				Reserved:   *reserved,
			}},
		}

		buf, marshalErr := sonic.Marshal(frame)
		So(marshalErr, ShouldBeNil)

		Convey("When the frame is decoded", func() {
			rows := NewBalanceDataSlice(buf)

			Convey("Then it should unwrap the channel envelope", func() {
				So(*rows, ShouldHaveLength, 1)
				So((*rows)[0].Asset, ShouldEqual, "USD")
				So((*rows)[0].Balance.String(), ShouldEqual, "200")
			})
		})
	})
}

func TestNewBalanceDataSliceFromSpot(t *testing.T) {
	Convey("Given spot REST balances", t, func() {
		usd, err := decimal.NewFromString("200.25")
		So(err, ShouldBeNil)
		btc, err := decimal.NewFromString("0.1")
		So(err, ShouldBeNil)

		balances := map[string]*decimal.Decimal{
			"USD": usd,
			"BTC": btc,
		}
		rows := NewBalanceDataSliceFromSpot(balances)

		Convey("Then they should become sorted balance rows", func() {
			So(rows, ShouldHaveLength, 2)
			So(rows[0].Asset, ShouldEqual, "BTC")
			So(rows[0].Balance.String(), ShouldEqual, "0.1")
			So(rows[0].Available.String(), ShouldEqual, "0.1")
			So(rows[1].Asset, ShouldEqual, "USD")
			So(rows[1].Balance.String(), ShouldEqual, "200.25")
		})
	})
}

func BenchmarkNewBalanceDataSliceFromSpot(b *testing.B) {
	usd, _ := decimal.NewFromString("200.25")
	btc, _ := decimal.NewFromString("0.1")
	balances := map[string]*decimal.Decimal{
		"USD": usd,
		"BTC": btc,
	}

	b.ReportAllocs()
	for b.Loop() {
		_ = NewBalanceDataSliceFromSpot(balances)
	}
}
