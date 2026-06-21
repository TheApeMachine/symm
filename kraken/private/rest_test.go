package private

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/kraken/public"
)

const (
	testSigningSecret = "fixture-signing-secret-not-a-real-key"
	testNonce         = "1616492376594"
	testBody          = `{"nonce":"1616492376594","ordertype":"limit","pair":"XBTUSD","price":"37500","type":"buy","volume":"1.25"}`
	testPath          = "/0/private/AddOrder"
	testExpectedSign  = "ea375a680fb8fd09aaf698e0880a747c3928ec5f30e19c8ab66dd2a59fc9df0a"
)

func TestRestSign(testingTB *testing.T) {
	convey.Convey("Given a synthetic signing fixture", testingTB, func() {
		ctx := context.Background()
		tree := dmt.NewTree("")
		rest := NewRest(ctx, public.EndpointAddOrder, tree)

		rest.apiKey = testSigningSecret

		convey.So(rest, convey.ShouldNotBeNil)

		signature := rest.sign(testPath, testNonce, testBody)

		convey.Convey("It should produce a deterministic API-Sign", func() {
			convey.So(signature, convey.ShouldEqual, testExpectedSign)

			again := rest.sign(testPath, testNonce, testBody)
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
