package kraken

import (
	"encoding/base64"
	"io"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
)

func TestTradeVolumeFeesEqual(t *testing.T) {
	Convey("Given two TradeVolume fee tiers", t, func() {
		left := TradeVolumeFees{Fee: "0.2600", MinFee: "0.1000", TierVolume: "0.0000"}
		right := TradeVolumeFees{Fee: "0.2600", MinFee: "0.1000", TierVolume: "0.0000"}
		different := TradeVolumeFees{Fee: "0.1800", MinFee: "0.1000", TierVolume: "0.0000"}

		Convey("When the tiers match", func() {
			So(left.Equal(right), ShouldBeTrue)
		})

		Convey("When the tiers differ", func() {
			So(left.Equal(different), ShouldBeFalse)
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

			Convey("Then spot pairs use Kraken's forex asset class and nonce remains transport-owned", func() {
				_, hasNonce := body["nonce"]
				So(hasNonce, ShouldBeFalse)

				pairs := body["pair"].([]any)
				So(pairs, ShouldHaveLength, 2)
				So(pairs[0].(map[string]any)["asset"], ShouldEqual, "BTC/USD")
				So(pairs[0].(map[string]any)["aclass"], ShouldEqual, "forex")
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
