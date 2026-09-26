package kraken

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/* newOrderFixture keeps all SDK requests inside an authenticated local venue. */
func newOrderFixture(t testing.TB, handler http.HandlerFunc) Orders {
	t.Helper()
	fixture := newTradingVenue(t, handler)
	terms := configureTerms(t, fixture.server)
	server := NewOrders()
	server.api.BaseURL, server.api.Executor = fixture.venue.URL, fixture.venue.Client().Do
	client := Orders_ServerToClient(server)
	t.Cleanup(client.Release)
	if err := client.Write(context.Background(), func(params Orders_write_Params) error {
		params.SetTermsReady(true)
		for _, err := range []error{params.SetPublicKey([]byte("fixture-key")), params.SetPrivateKey([]byte(base64.StdEncoding.EncodeToString([]byte("fixture-secret")))), params.SetTerms(terms.AddRef())} {
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.WaitStreaming(); err != nil {
		t.Fatal(err)
	}
	return client
}

func TestOrdersSubmit(t *testing.T) {
	Convey("The native order node submits one SDK-normalized request without retrying", t, func(c C) {
		requests := 0
		client := newOrderFixture(t, func(writer http.ResponseWriter, request *http.Request) {
			requests++
			c.So(request.URL.Path, ShouldEqual, "/0/private/AddOrder")
			c.So(request.Header.Get("API-Key"), ShouldEqual, "fixture-key")
			c.So(request.Header.Get("API-Sign"), ShouldNotBeEmpty)
			var order map[string]any
			c.So(json.NewDecoder(request.Body).Decode(&order), ShouldBeNil)
			c.So(order["pair"], ShouldEqual, "BTCUSD")
			c.So(order["volume"], ShouldEqual, "0.12345678")
			c.So(order["type"], ShouldEqual, "buy")
			c.So(order["ordertype"], ShouldEqual, "market")
			c.So(order["cl_ord_id"], ShouldEqual, "fixture-001")
			_, err := io.WriteString(writer, `{"error":[],"result":{"txid":["VENUE-001"]}}`)
			c.So(err, ShouldBeNil)
		})
		future, release := client.Submit(context.Background(), func(params Orders_submit_Params) error {
			order, err := params.NewRequest()
			if err != nil {
				return err
			}
			for _, err := range []error{order.SetSymbol("BTC/USD"), order.SetQuantity("0.123456789"), order.SetSide("buy"), order.SetClientId("fixture-001")} {
				if err != nil {
					return err
				}
			}
			return nil
		})
		defer release()
		result, err := future.Struct()
		c.So(err, ShouldBeNil)
		identity, err := result.Id()
		c.So(err, ShouldBeNil)
		c.So(identity, ShouldEqual, "VENUE-001")
		c.So(requests, ShouldEqual, 1)
	})
}

func TestOrdersInspect(t *testing.T) {
	Convey("Execution facts retain authoritative cumulative amounts through partial and closed states", t, func(c C) {
		status := "open"
		client := newOrderFixture(t, func(writer http.ResponseWriter, request *http.Request) {
			c.So(request.URL.Path, ShouldEqual, "/0/private/QueryOrders")
			_, err := io.WriteString(writer, `{"error":[],"result":{"VENUE-001":{"status":"`+status+`","cl_ord_id":"fixture-001","descr":{"pair":"BTCUSD","type":"buy"},"vol_exec":"0.003","cost":"1.00000001","fee":"0.002600000026","price":"333.33"}}}`)
			c.So(err, ShouldBeNil)
		})
		for _, phase := range []string{"open", "closed"} {
			status = phase
			future, release := client.Inspect(context.Background(), func(params Orders_inspect_Params) error { return params.SetId("VENUE-001") })
			result, err := future.Struct()
			c.So(err, ShouldBeNil)
			execution, err := result.Order()
			c.So(err, ShouldBeNil)
			cost, err := execution.Cost()
			c.So(err, ShouldBeNil)
			c.So(cost, ShouldEqual, "1.00000001")
			fee, err := execution.Fee()
			c.So(err, ShouldBeNil)
			c.So(fee, ShouldEqual, "0.002600000026")
			price, err := execution.AveragePrice()
			c.So(err, ShouldBeNil)
			c.So(price, ShouldEqual, "333.33")
			actual, err := execution.Status()
			c.So(err, ShouldBeNil)
			c.So(actual, ShouldEqual, phase)
			release()
		}
	})
}

func TestOrdersCancel(t *testing.T) {
	Convey("Cancellation acknowledges the venue count without synthesizing execution", t, func(c C) {
		client := newOrderFixture(t, func(writer http.ResponseWriter, request *http.Request) {
			c.So(request.URL.Path, ShouldEqual, "/0/private/CancelOrder")
			_, err := io.WriteString(writer, `{"error":[],"result":{"count":1}}`)
			c.So(err, ShouldBeNil)
		})
		future, release := client.Cancel(context.Background(), func(params Orders_cancel_Params) error { return params.SetId("VENUE-001") })
		defer release()
		result, err := future.Struct()
		c.So(err, ShouldBeNil)
		c.So(result.Count(), ShouldEqual, 1)
	})
}

func BenchmarkOrdersInspect(b *testing.B) {
	client := newOrderFixture(b, func(writer http.ResponseWriter, request *http.Request) {
		if _, err := io.WriteString(writer, `{"error":[],"result":{"VENUE-001":{"status":"closed","cl_ord_id":"fixture-001","descr":{"pair":"BTCUSD","type":"buy"},"vol_exec":"0.003","cost":"1.00000001","fee":"0.002600000026","price":"333.33"}}}`); err != nil {
			b.Error(err)
		}
	})
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		future, release := client.Inspect(context.Background(), func(params Orders_inspect_Params) error { return params.SetId("VENUE-001") })
		_, err := future.Struct()
		release()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestOrdersFind(t *testing.T) {
	Convey("An uncertain submit is reconciled by client identity without issuing another order", t, func(c C) {
		client := newOrderFixture(t, func(writer http.ResponseWriter, request *http.Request) {
			payload := `{"error":[],"result":{"open":{}}}`
			c.So(request.URL.Path, ShouldBeIn, "/0/private/OpenOrders", "/0/private/ClosedOrders")
			var body map[string]any
			c.So(json.NewDecoder(request.Body).Decode(&body), ShouldBeNil)
			c.So(body["cl_ord_id"], ShouldEqual, "fixture-001")
			if request.URL.Path == "/0/private/ClosedOrders" {
				payload = `{"error":[],"result":{"closed":{"VENUE-001":{"status":"closed","cl_ord_id":"fixture-001","descr":{"pair":"BTCUSD","type":"buy"},"vol_exec":"0.003","cost":"1.00000001","fee":"0.002600000026","price":"333.33"}}}}`
			}
			_, err := io.WriteString(writer, payload)
			c.So(err, ShouldBeNil)
		})
		future, release := client.Find(context.Background(), func(params Orders_find_Params) error { return params.SetClientId("fixture-001") })
		defer release()
		result, err := future.Struct()
		c.So(err, ShouldBeNil)
		c.So(result.Found(), ShouldBeTrue)
		execution, err := result.Order()
		c.So(err, ShouldBeNil)
		identity, err := execution.Id()
		c.So(err, ShouldBeNil)
		c.So(identity, ShouldEqual, "VENUE-001")
	})
}
