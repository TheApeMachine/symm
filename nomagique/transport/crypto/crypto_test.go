package crypto_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/transport/crypto"
)

func TestCryptoPrimitives(t *testing.T) {
	ctx := context.Background()

	Convey("Given crypto primitives", t, func() {
		Convey("Base64Encode and Base64Decode roundtrip data", func() {
			encodeServer := crypto.NewBase64Encode(t.Context())
			encodeClient := crypto.Base64Encode_ServerToClient(encodeServer)
			So(encodeClient.IsValid(), ShouldBeTrue)

			rawPayload := []byte("secret_api_key_data")
			err := encodeClient.Write(ctx, func(params crypto.Base64Encode_write_Params) error {
				return params.SetData(rawPayload)
			})
			So(err, ShouldBeNil)
			So(encodeClient.WaitStreaming(), ShouldBeNil)

			encodeFuture, encodeRelease := encodeClient.Done(ctx, nil)
			defer encodeRelease()

			encodeResults, err := encodeFuture.Struct()
			So(err, ShouldBeNil)

			encodedData, err := encodeResults.Out()
			So(err, ShouldBeNil)
			So(string(encodedData), ShouldEqual, "c2VjcmV0X2FwaV9rZXlfZGF0YQ==")

			decodeServer := crypto.NewBase64Decode(t.Context())
			decodeClient := crypto.Base64Decode_ServerToClient(decodeServer)
			So(decodeClient.IsValid(), ShouldBeTrue)

			err = decodeClient.Write(ctx, func(params crypto.Base64Decode_write_Params) error {
				return params.SetData(encodedData)
			})
			So(err, ShouldBeNil)
			So(decodeClient.WaitStreaming(), ShouldBeNil)

			decodeFuture, decodeRelease := decodeClient.Done(ctx, nil)
			defer decodeRelease()

			decodeResults, err := decodeFuture.Struct()
			So(err, ShouldBeNil)

			decodedData, err := decodeResults.Out()
			So(err, ShouldBeNil)
			So(string(decodedData), ShouldEqual, string(rawPayload))
		})

		Convey("BearerAuth formats token with Bearer prefix", func() {
			server := crypto.NewBearerAuth(t.Context())
			client := crypto.BearerAuth_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params crypto.BearerAuth_write_Params) error {
				return params.SetData([]byte("test_token_123"))
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, "Bearer test_token_123")
		})

		Convey("HeaderAuth formats header with Header prefix", func() {
			server := crypto.NewHeaderAuth(t.Context())
			client := crypto.HeaderAuth_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params crypto.HeaderAuth_write_Params) error {
				return params.SetData([]byte("auth_signature_bytes"))
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, "Header: auth_signature_bytes")
		})

		Convey("SHA256 computes expected digest", func() {
			server := crypto.NewSHA256(t.Context())
			client := crypto.SHA256_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params crypto.SHA256_write_Params) error {
				return params.SetData([]byte("hello world"))
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(len(out), ShouldEqual, 32)
		})

		Convey("HMACSHA256 and HMACSHA512 compute digests", func() {
			server256 := crypto.NewHMACSHA256(t.Context())
			client256 := crypto.HMACSHA256_ServerToClient(server256)
			So(client256.IsValid(), ShouldBeTrue)

			err := client256.Write(ctx, func(params crypto.HMACSHA256_write_Params) error {
				return params.SetData([]byte("message_to_sign"))
			})
			So(err, ShouldBeNil)
			So(client256.WaitStreaming(), ShouldBeNil)

			future256, release256 := client256.Done(ctx, nil)
			defer release256()

			results256, err := future256.Struct()
			So(err, ShouldBeNil)

			out256, err := results256.Out()
			So(err, ShouldBeNil)
			So(len(out256), ShouldEqual, 32)

			server512 := crypto.NewHMACSHA512(t.Context())
			client512 := crypto.HMACSHA512_ServerToClient(server512)
			So(client512.IsValid(), ShouldBeTrue)

			err = client512.Write(ctx, func(params crypto.HMACSHA512_write_Params) error {
				return params.SetData([]byte("message_to_sign"))
			})
			So(err, ShouldBeNil)
			So(client512.WaitStreaming(), ShouldBeNil)

			future512, release512 := client512.Done(ctx, nil)
			defer release512()

			results512, err := future512.Struct()
			So(err, ShouldBeNil)

			out512, err := results512.Out()
			So(err, ShouldBeNil)
			So(len(out512), ShouldEqual, 64)
		})

		Convey("Nonce increments monotonically", func() {
			server := crypto.NewNonce(t.Context())
			client := crypto.Nonce_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params crypto.Nonce_write_Params) error {
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(len(out), ShouldEqual, 8)
		})
	})
}
