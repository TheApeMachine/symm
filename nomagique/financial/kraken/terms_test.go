package kraken

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/* tradingVenue is the one authenticated SDK HTTP boundary fixture. */
type tradingVenue struct {
	server  *TermsServer
	venue   *httptest.Server
	quoted  atomic.Int64
	missing atomic.Bool
	orders  http.HandlerFunc
}

func newTradingVenue(t testing.TB, orders http.HandlerFunc) *tradingVenue {
	t.Helper()
	fixture := &tradingVenue{orders: orders}
	venue := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		payload := `{"error":[],"result":{}}`
		switch request.URL.Path {
		case "/0/public/Assets":
			payload = `{"error":[],"result":{"BTC":{"altname":"XBT","decimals":10,"display_decimals":5},"USD":{"altname":"USD","decimals":4,"display_decimals":2}}}`
		case "/0/public/AssetPairs":
			payload = `{"error":[],"result":{"BTCUSD":{"altname":"BTCUSD","wsname":"XBT/USD","base":"BTC","quote":"USD","lot_decimals":8,"lot_multiplier":1,"cost_decimals":5,"pair_decimals":1,"ordermin":"0.0001","costmin":"0.5","tick_size":"0.1","status":"online"}}}`
		case "/0/private/BalanceEx":
			payload = `{"error":[],"result":{"USD":{"balance":"200.00000001","hold_trade":"2.00000000","credit":"1000","credit_used":"0"}}}`
		case "/0/private/TradeVolume":
			if request.Header.Get("API-Key") != "fixture-key" || request.Header.Get("API-Sign") == "" {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			var body struct {
				Pair string `json:"pair"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Error(err)
				writer.WriteHeader(http.StatusBadRequest)
				return
			}

			if body.Pair != "BTCUSD" {
				t.Errorf("wrong normalized pair: %q", body.Pair)
			}
			fixture.quoted.Add(1)
			payload = `{"error":[],"result":{"fees":{"BTCUSD":{"fee":"0.26"}}}}`

			if fixture.missing.Load() {
				payload = `{"error":[],"result":{"fees":{}}}`
			}
		default:
			if fixture.orders == nil {
				t.Errorf("unexpected endpoint %s", request.URL.Path)
				return
			}
			fixture.orders(writer, request)
			return
		}
		if _, err := io.WriteString(writer, payload); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(venue.Close)
	server := NewTerms()
	server.api.BaseURL = venue.URL
	server.api.Executor = venue.Client().Do
	fixture.server, fixture.venue = server, venue
	return fixture
}

func configureTerms(t testing.TB, server *TermsServer) Terms {
	t.Helper()
	client := Terms_ServerToClient(server)
	t.Cleanup(client.Release)
	if err := client.Write(context.Background(), func(params Terms_write_Params) error {
		if err := params.SetPublicKey([]byte("fixture-key")); err != nil {
			return err
		}
		return params.SetPrivateKey([]byte(base64.StdEncoding.EncodeToString([]byte("fixture-secret"))))
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.WaitStreaming(); err != nil {
		t.Fatal(err)
	}
	return client
}

func TestTermsQuote(t *testing.T) {
	Convey("Authoritative terms use Kraken authentication and SDK alias normalization", t, func() {
		fixture := newTradingVenue(t, nil)
		client := configureTerms(t, fixture.server)
		ctx := context.Background()

		Convey("A quote preserves decimal fees, venue minima and the SDK increment", func() {
			future, release := client.Quote(ctx, func(params Terms_quote_Params) error { return params.SetSymbol("BTC/USD") })
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			terms, err := result.Terms()
			So(err, ShouldBeNil)
			fee, err := terms.TakerFee()
			So(err, ShouldBeNil)
			So(fee, ShouldEqual, "0.0026")
			minimum, err := terms.MinimumQuantity()
			So(err, ShouldBeNil)
			So(minimum, ShouldEqual, "0.00010000")
			increment, err := terms.QuantityIncrement()
			So(err, ShouldBeNil)
			So(increment, ShouldEqual, "0.00000001")
			So(fixture.quoted.Load(), ShouldEqual, 1)
		})

		Convey("An omitted account fee is an error rather than a synthetic fee", func() {
			fixture.missing.Store(true)
			future, release := client.Quote(ctx, func(params Terms_quote_Params) error { return params.SetSymbol("BTC/USD") })
			defer release()
			_, err := future.Struct()
			So(err, ShouldNotBeNil)
		})
	})
}

func TestTermsBalance(t *testing.T) {
	Convey("Cash available excludes trade holds and borrowed credit", t, func() {
		fixture := newTradingVenue(t, nil)
		client := configureTerms(t, fixture.server)
		future, release := client.Balance(context.Background(), func(params Terms_balance_Params) error { return params.SetSymbol("BTC/USD") })
		defer release()
		result, err := future.Struct()
		So(err, ShouldBeNil)
		available, err := result.Available()
		So(err, ShouldBeNil)
		So(available, ShouldEqual, "198.00000001")
	})
}
