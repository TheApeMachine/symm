package kraken

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFloat64Fast(t *testing.T) {
	Convey("Given kraken Decimal instances", t, func() {
		testCases := []string{
			"0",
			"0.0",
			"1",
			"-1",
			"123.456",
			"-123.456",
			"0.00000001",
			"67890.12345678",
			"1000000000.5",
			"-987654321.123456789",
		}

		for _, tc := range testCases {
			d, err := decimal.NewFromString(tc)
			So(err, ShouldBeNil)
			expected := d.Float64()
			actual := Float64(d)
			So(actual, ShouldAlmostEqual, expected, 1e-6)
		}

		So(Float64(nil), ShouldEqual, 0)
	})
}

func BenchmarkFloat64SDK(b *testing.B) {
	d, _ := decimal.NewFromString("67890.12345678")
	b.ReportAllocs()

	for b.Loop() {
		_ = d.Float64()
	}
}

func BenchmarkFloat64Fast(b *testing.B) {
	d, _ := decimal.NewFromString("67890.12345678")
	b.ReportAllocs()

	for b.Loop() {
		_ = Float64(d)
	}
}
