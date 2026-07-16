package kraken

import (
	"encoding/base64"
	"io"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewTradeVolume(t *testing.T) {
	Convey("Given a typed Kraken TradeVolume response", t, func() {
		result := NewTradeVolume([]byte(`{
			"error":[],"result":{"fees":{"XXBTZUSD":{"fee":"0.2600"}}}
		}`))

		Convey("It should decode the result without rebuilding its fields", func() {
			So(result, ShouldNotBeNil)
			So(result.Fees["XXBTZUSD"].Fee.Float64(), ShouldEqual, 0.26)
		})
	})
}

func TestNewTradeVolumeRequest(t *testing.T) {
	Convey("Given websocket v2 symbols", t, func() {
		request := NewTradeVolumeRequest([]string{"BTC/USD", "ETH/USD"})

		Convey("When the private REST request is encoded", func() {
			encoded, err := request.MarshalJSON()
			So(err, ShouldBeNil)

			body := map[string]any{}
			err = sonic.Unmarshal(encoded, &body)
			So(err, ShouldBeNil)

			Convey("Then pairs stay plain websocket v2 symbols and nonce remains transport-owned", func() {
				_, hasNonce := body["nonce"]
				So(hasNonce, ShouldBeFalse)

				So(body["pair"], ShouldEqual, "BTC/USD,ETH/USD")
				So(body["fee_schedule"], ShouldBeTrue)
			})
		})

		Convey("When the official SDK builds the authenticated REST request", func() {
			rest := spot.NewREST()
			rest.PublicKey = "test-key"
			rest.PrivateKey = base64.StdEncoding.EncodeToString([]byte("test-secret"))
			rest.Nonce = func() string { return "123456789" }
			signed, err := rest.NewRequest(spot.RequestOptions{
				Auth:   true,
				Method: "POST",
				Path:   "/0/private/TradeVolume",
				Body:   request,
			})
			So(err, ShouldBeNil)

			bodyReader, err := signed.GetBody()
			So(err, ShouldBeNil)
			encoded, err := io.ReadAll(bodyReader)
			So(err, ShouldBeNil)
			So(bodyReader.Close(), ShouldBeNil)

			body := map[string]any{}
			err = sonic.Unmarshal(encoded, &body)
			So(err, ShouldBeNil)

			Convey("Then the transport injects its monotonic nonce", func() {
				So(body["nonce"], ShouldEqual, "123456789")
			})
		})
	})
}
