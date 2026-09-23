package crypto_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"strconv"
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

		Convey("HMACSHA256 and HMACSHA512 authenticate under their key (RFC 4231 case 2)", func() {
			client256 := crypto.HMACSHA256_ServerToClient(crypto.NewHMACSHA256(t.Context()))
			So(client256.Write(ctx, func(params crypto.HMACSHA256_write_Params) error {
				if err := params.SetKey([]byte("Jefe")); err != nil {
					return err
				}
				return params.SetData([]byte("what do ya want for nothing?"))
			}), ShouldBeNil)
			So(client256.WaitStreaming(), ShouldBeNil)
			future256, release256 := client256.Done(ctx, nil)
			defer release256()
			results256, err := future256.Struct()
			So(err, ShouldBeNil)
			out256, err := results256.Out()
			So(err, ShouldBeNil)
			So(hex.EncodeToString(out256), ShouldEqual, "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843")

			client512 := crypto.HMACSHA512_ServerToClient(crypto.NewHMACSHA512(t.Context()))
			So(client512.Write(ctx, func(params crypto.HMACSHA512_write_Params) error {
				if err := params.SetKey([]byte("Jefe")); err != nil {
					return err
				}
				return params.SetData([]byte("what do ya want for nothing?"))
			}), ShouldBeNil)
			So(client512.WaitStreaming(), ShouldBeNil)
			future512, release512 := client512.Done(ctx, nil)
			defer release512()
			results512, err := future512.Struct()
			So(err, ShouldBeNil)
			out512, err := results512.Out()
			So(err, ShouldBeNil)
			So(hex.EncodeToString(out512), ShouldEqual,
				"164b7a7bfcf819e2e395fbe73b56e0a387bd64222e831fd610270cd7ea2505549758bf75c05a994a6d034f65f8f0e6fdcaeab1a34d4a6b4b636e070a38bce737")
		})

		Convey("HMACSHA512 without a key is an error, not an unkeyed digest", func() {
			client := crypto.HMACSHA512_ServerToClient(crypto.NewHMACSHA512(t.Context()))
			So(client.Write(ctx, func(params crypto.HMACSHA512_write_Params) error {
				return params.SetData([]byte("message"))
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})

		Convey("Nonce issues strictly increasing decimal numbers", func() {
			client := crypto.Nonce_ServerToClient(crypto.NewNonce(t.Context()))
			issue := func() int64 {
				So(client.Write(ctx, func(params crypto.Nonce_write_Params) error {
					return params.SetTrigger([]byte("go"))
				}), ShouldBeNil)
				So(client.WaitStreaming(), ShouldBeNil)
				future, release := client.Done(ctx, nil)
				defer release()
				results, err := future.Struct()
				So(err, ShouldBeNil)
				out, err := results.Out()
				So(err, ShouldBeNil)
				value, err := strconv.ParseInt(string(out), 10, 64)
				So(err, ShouldBeNil)
				return value
			}
			first := issue()
			So(issue(), ShouldBeGreaterThan, first)
		})

		Convey("Secret reads the named environment variable", func() {
			t.Setenv("SYMM_TEST_SECRET", "credential")
			read := func(name string) ([]byte, error) {
				client := crypto.Secret_ServerToClient(crypto.NewSecret(context.Background()))
				defer client.Release()
				if err := client.Write(ctx, func(params crypto.Secret_write_Params) error {
					return params.SetName(name)
				}); err != nil {
					return nil, err
				}
				if err := client.WaitStreaming(); err != nil {
					return nil, err
				}
				future, release := client.Done(ctx, nil)
				defer release()
				results, err := future.Struct()
				if err != nil {
					return nil, err
				}
				out, err := results.Out()
				return bytes.Clone(out), err
			}
			out, err := read("SYMM_TEST_SECRET")
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, "credential")

			_, err = read("SYMM_TEST_SECRET_THAT_IS_NOT_SET")
			So(err, ShouldNotBeNil)
		})
	})
}
