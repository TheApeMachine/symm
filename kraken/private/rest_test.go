package private

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/kraken/public"
)

const (
	testSigningSecret = "kQH5HW/8p1uGOVjbgWA7FunAmGO8lsSUXNsu3eow76sz84Q18fWxnyRzBHCd3pd5nE9qa99HAZtuZuj6F1huXg=="
	testNonce         = "1616492376594"
	testBody          = "nonce=1616492376594&ordertype=limit&pair=XBTUSD&price=37500&type=buy&volume=1.25"
	testPath          = "/0/private/AddOrder"
	testExpectedSign  = "4/dpxb3iT4tp/ZCVEwSnEsLxx0bqyhLpdfOpc6fn7OR8+UClSV5n9E6aSS8MPtnRfp32bAb0nmbRn6H8ndwLUQ=="
)

func TestRestSign(testingTB *testing.T) {
	convey.Convey("Given a synthetic signing fixture", testingTB, func() {
		ctx := context.Background()
		tree := dmt.NewTree("")
		rest := NewRest(ctx, public.EndpointAddOrder, tree)

		rest.apiSecret = testSigningSecret

		convey.So(rest, convey.ShouldNotBeNil)

		signature, err := rest.sign(testPath, testNonce, testBody)

		convey.Convey("It should produce a deterministic API-Sign", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(signature, convey.ShouldEqual, testExpectedSign)

			again, againErr := rest.sign(testPath, testNonce, testBody)
			convey.So(againErr, convey.ShouldBeNil)
			convey.So(again, convey.ShouldEqual, signature)
		})
	})
}

func TestNewRestRequiresCredentials(testingTB *testing.T) {
	convey.Convey("Given empty credentials", testingTB, func() {
		rest := NewRest(context.Background(), public.EndpointAddOrder, dmt.NewTree(""))

		convey.Convey("It should reject construction", func() {
			convey.So(rest, convey.ShouldNotBeNil)
		})
	})
}

func TestRestForEndpoint(testingTB *testing.T) {
	convey.Convey("Given one private REST client", testingTB, func() {
		ctx := context.Background()
		tree := dmt.NewTree("")
		rest := NewRest(ctx, public.EndpointAddOrder, tree)

		convey.So(rest, convey.ShouldNotBeNil)

		cancelRest := NewRest(ctx, public.EndpointCancelOrder, tree)

		convey.So(cancelRest, convey.ShouldNotBeNil)

		convey.Convey("It should share credentials on another endpoint", func() {
			convey.So(cancelRest.endpoint, convey.ShouldEqual, public.EndpointCancelOrder)
		})
	})
}
