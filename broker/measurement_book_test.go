package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
)

func TestMeasurementBookEnricher(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a quote cache", t, func() {
		cache := NewQuoteCache(t.Context(), nil)

		enricher, err := MeasurementBookEnricher(cache)

		Convey("It should return an enricher without error", func() {
			So(err, ShouldBeNil)
			So(enricher, ShouldNotBeNil)
		})
	})

	Convey("Given a nil quote cache", t, func() {
		_, err := MeasurementBookEnricher(nil)

		Convey("It should return an error", func() {
		 So(err, ShouldNotBeNil)
		})
	})
}
