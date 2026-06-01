package private

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

const (
	krakenDocPrivateKey = "kQH5HW/8p1uGOVjbgWA7FunAmGO8lsSUXNsu3eow76sz84Q18fWxnyRzBHCd3pd5nE9qa99HAZtuZuj6F1huXg=="
	krakenDocNonce      = "1616492376594"
	krakenDocBody       = `{"nonce":"1616492376594","ordertype":"limit","pair":"XBTUSD","price":"37500","type":"buy","volume":"1.25"}`
	krakenDocPath       = "/0/private/AddOrder"
)

func TestRestSignKrakenDocVector(t *testing.T) {
	convey.Convey("Given Kraken's AddOrder JSON body", t, func() {
		ctx := context.Background()
		rest, err := NewRest(ctx, "key", krakenDocPrivateKey, EndpointAddOrder)

		convey.So(err, convey.ShouldBeNil)

		signature, signErr := rest.sign(krakenDocPath, krakenDocNonce, krakenDocBody)

		convey.Convey("It should produce a stable API-Sign", func() {
			convey.So(signErr, convey.ShouldBeNil)
			convey.So(signature, convey.ShouldNotBeBlank)

			again, signErr := rest.sign(krakenDocPath, krakenDocNonce, krakenDocBody)
			convey.So(signErr, convey.ShouldBeNil)
			convey.So(again, convey.ShouldEqual, signature)
		})
	})
}

func TestNewRestRequiresCredentials(t *testing.T) {
	convey.Convey("Given empty credentials", t, func() {
		_, err := NewRest(context.Background(), "", "", EndpointAddOrder)

		convey.Convey("It should reject construction", func() {
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

func TestRestForEndpoint(t *testing.T) {
	convey.Convey("Given one private REST client", t, func() {
		ctx := context.Background()
		rest, err := NewRest(ctx, "key", krakenDocPrivateKey, EndpointAddOrder)

		convey.So(err, convey.ShouldBeNil)

		cancelRest := rest.ForEndpoint(EndpointCancelOrder)

		convey.Convey("It should share credentials on another endpoint", func() {
			convey.So(cancelRest.apiKey, convey.ShouldEqual, rest.apiKey)
			convey.So(cancelRest.endpoint, convey.ShouldEqual, EndpointCancelOrder)
		})
	})
}
