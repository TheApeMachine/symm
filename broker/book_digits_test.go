package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAppendDecimalDigits(t *testing.T) {
	Convey("Kraken L3 CRC32 checksum decimal formatting", t, func() {
		Convey("Strips decimal points and trims leading zeros", func() {
			cases := []struct {
				val      string
				expected string
			}{
				{"0.00100000", "100000"},
				{"0.00316", "316"},
				{"0.1", "1"},
				{"1.0", "10"},
				{"100.50", "10050"},
				{"1000", "1000"},
				{"0.0", ""},
			}

			for _, c := range cases {
				dec, err := decimal.NewFromString(c.val)
				So(err, ShouldBeNil)
				res := string(appendDecimalDigits(nil, dec))
				So(res, ShouldEqual, c.expected)
			}
		})
	})
}
