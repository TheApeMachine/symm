package kraken

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInstrumentSubscription(t *testing.T) {
	Convey("Given an instrument subscription", t, func() {
		subscription := NewInstrumentSubscription()

		Convey("When it is marshaled", func() {
			raw, err := subscription.MarshalJSON()

			Convey("Then it should request the instrument channel", func() {
				So(err, ShouldBeNil)

				var parsed map[string]any

				So(json.Unmarshal(raw, &parsed), ShouldBeNil)
				So(parsed["method"], ShouldEqual, "subscribe")
				So(parsed["params"], ShouldResemble, map[string]any{
					"channel": "instrument",
				})
			})
		})
	})
}
