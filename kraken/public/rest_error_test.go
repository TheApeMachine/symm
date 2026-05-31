package public

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRestGetRejectsKrakenErrorPayload(t *testing.T) {
	Convey("Given a Kraken error envelope", t, func() {
		var envelope Response

		err := json.Unmarshal([]byte(`{
			"error": ["EGeneral:Invalid arguments"],
			"result": null
		}`), &envelope)

		Convey("It should expose the error strings", func() {
			So(err, ShouldBeNil)
			So(len(envelope.Error), ShouldEqual, 1)
			So(envelope.Error[0], ShouldContainSubstring, "Invalid arguments")
		})
	})
}
