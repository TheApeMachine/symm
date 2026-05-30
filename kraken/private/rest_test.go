package private

import (
	"encoding/base64"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestRestSign(t *testing.T) {
	convey.Convey("Given a known secret and body", t, func() {
		secret := base64.StdEncoding.EncodeToString([]byte("secret"))
		rest, err := NewRest("key", secret)

		convey.So(err, convey.ShouldBeNil)

		signature, signErr := rest.sign("/0/private/GetWebSocketsToken", "nonce=1")

		convey.Convey("It should produce a non-empty API-Sign", func() {
			convey.So(signErr, convey.ShouldBeNil)
			convey.So(signature, convey.ShouldNotBeBlank)
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
