package core

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestReadDecimal(t *testing.T) {
	Convey("SDK monetary values preserve exact external decimal text", t, func() {
		for _, entry := range []struct{ input, expected string }{
			{"1e-8", "0.00000001"}, {"-1e-8", "-0.00000001"},
			{"-123456789012345678901234567890.1234567890123456789", "-123456789012345678901234567890.1234567890123456789"},
			{`"0.0001000"`, "0.0001"}, {"99", "99"},
		} {
			value, err := ReadDecimal([]byte(entry.input), "fixture")
			So(err, ShouldBeNil)
			encoded, err := WriteDecimal(value)
			So(err, ShouldBeNil)
			So(string(encoded), ShouldEqual, entry.expected)
		}
		for _, invalid := range []string{"", "1/3", "no-price"} {
			_, err := ReadDecimal([]byte(invalid), "fixture")
			So(err, ShouldNotBeNil)
		}
	})
}

func BenchmarkReadDecimal(b *testing.B) {
	for b.Loop() {
		if _, err := ReadDecimal([]byte(`"65000.00000001"`), "price"); err != nil {
			b.Fatal(err)
		}
	}
}
