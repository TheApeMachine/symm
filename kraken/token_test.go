package kraken

import (
	"encoding/base64"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTokenExpired(t *testing.T) {
	Convey("Given token expiry semantics", t, func() {
		Convey("It should treat nil and empty tokens as expired", func() {
			var nilToken *Token
			So(nilToken.Expired(), ShouldBeTrue)
			So((&Token{}).Expired(), ShouldBeTrue)
		})

		Convey("It should treat a fresh token as live", func() {
			token := &Token{
				issuedAt: time.Now(),
				Result: struct {
					Token   string `json:"token"`
					Expires int    `json:"expires"`
				}{Token: "abc", Expires: 600},
			}
			So(token.Expired(), ShouldBeFalse)
			So(token.Value(), ShouldEqual, "abc")
		})

		Convey("It should expire after the lifetime minus skew", func() {
			token := &Token{
				issuedAt: time.Now().Add(-10 * time.Minute),
				Result: struct {
					Token   string `json:"token"`
					Expires int    `json:"expires"`
				}{Token: "abc", Expires: 60},
			}
			So(token.Expired(), ShouldBeTrue)
		})
	})
}

func TestSign(t *testing.T) {
	Convey("Given a base64-encoded HMAC key", t, func() {
		key := base64.StdEncoding.EncodeToString([]byte("secret"))
		signature, err := sign(key, []byte("payload"))

		Convey("It should produce a base64 signature", func() {
			So(err, ShouldBeNil)
			So(signature, ShouldNotBeEmpty)
		})
	})
}

func BenchmarkTokenValue(b *testing.B) {
	token := &Token{
		Result: struct {
			Token   string `json:"token"`
			Expires int    `json:"expires"`
		}{Token: "abc", Expires: 600},
	}

	for b.Loop() {
		_ = token.Value()
	}
}
