package private

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

const (
	krakenDocPrivateKey = "kQH5HW/8p1uGOVjbgWA7FunAmGO8lsSUXNsu3eow76sz84Q18fWxnyRzBHCd3pd5nE9qa99HAZtuZuj6F1huXg=="
	krakenDocNonce      = "1616492376594"
	krakenDocBody       = "nonce=1616492376594&ordertype=limit&pair=XBTUSD&price=37500&type=buy&volume=1.25"
	krakenDocPath       = "/0/private/AddOrder"
	krakenDocAPISign    = "4/dpxb3iT4tp/ZCVEwSnEsLxx0bqyhLpdfOpc6fn7OR8+UClSV5n9E6aSS8MPtnRfp32bAb0nmbRn6H8ndwLUQ=="
)

func TestRestSignKrakenDocVector(t *testing.T) {
	convey.Convey("Given Kraken's documented AddOrder example", t, func() {
		rest, err := NewRest("key", krakenDocPrivateKey)

		convey.So(err, convey.ShouldBeNil)

		signature, signErr := rest.sign(krakenDocPath, krakenDocNonce, krakenDocBody)

		convey.Convey("It should match the published API-Sign", func() {
			convey.So(signErr, convey.ShouldBeNil)
			convey.So(signature, convey.ShouldEqual, krakenDocAPISign)
		})
	})
}

func TestNewRestRequiresCredentials(t *testing.T) {
	convey.Convey("Given empty credentials", t, func() {
		_, err := NewRest("", "")

		convey.Convey("It should reject construction", func() {
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}
