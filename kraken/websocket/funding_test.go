package websocket

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFundingLedgerObserve(t *testing.T) {
	Convey("Given paginated authoritative ledger entries and overlapping API seconds", t, func() {
		funding := FundingLedger{}
		at := time.Unix(100, 0)
		calls := 0
		post := func(path string, body json.Marshaler) ([]byte, error) {
			So(path, ShouldEqual, "/0/private/Ledgers")
			request := body.(ledgerRequest)
			So(request.Start, ShouldEqual, 100)
			calls++

			if request.Offset == 0 {
				return []byte(`{"error":[],"result":{"ledger":{"deposit":{"time":100.2,"type":"deposit","asset":"USD","amount":"50","fee":"1"}},"count":2}}`), nil
			}
			return []byte(`{"error":[],"result":{"ledger":{"buy":{"time":100.3,"type":"trade","asset":"USD","amount":"-20","fee":"0.1"}},"count":2}}`), nil
		}
		normalize := func(asset string) string { return asset }
		total, reason, err := funding.Observe(post, normalize, "USD", at)
		So(err, ShouldBeNil)
		So(reason, ShouldBeEmpty)
		So(total.Rat().RatString(), ShouldEqual, "49")
		So(calls, ShouldEqual, 2)
		total, _, err = funding.Observe(post, normalize, "USD", at)
		So(err, ShouldBeNil)
		So(total.Rat().RatString(), ShouldEqual, "49")
		Convey("Non-quote funding cannot be silently valued at a made-up conversion", func() {
			post = func(string, json.Marshaler) ([]byte, error) {
				return []byte(`{"error":[],"result":{"ledger":{"crypto":{"time":101,"type":"deposit","asset":"BTC","amount":"1","fee":"0"}},"count":1}}`), nil
			}
			total, reason, err = funding.Observe(post, normalize, "USD", at.Add(time.Second))
			So(err, ShouldBeNil)
			So(total, ShouldBeNil)
			So(reason, ShouldContainSubstring, "historical valuation")
		})

		Convey("Absent ledger metadata is unavailable, never a fabricated zero flow", func() {
			for _, payload := range []string{`{}`, `{"result":{"ledger":{}}}`} {
				post := func(string, json.Marshaler) ([]byte, error) { return []byte(payload), nil }
				total, _, err := funding.Observe(post, normalize, "USD", at)
				So(err, ShouldNotBeNil)
				So(total, ShouldBeNil)
			}
		})
		Convey("An incomplete response cannot advance the cursor or train guessed funding", func() {
			post = func(string, json.Marshaler) ([]byte, error) {
				return []byte(`{"error":[],"result":{"ledger":{},"count":1}}`), nil
			}
			_, _, err = funding.Observe(post, normalize, "USD", at.Add(time.Second))
			So(err, ShouldNotBeNil)
			So(funding.cursor, ShouldEqual, 100)
			So(funding.total.Rat().RatString(), ShouldEqual, "49")
		})
	})
}

func BenchmarkFundingLedgerObserve(b *testing.B) {
	funding := FundingLedger{}
	post := func(string, json.Marshaler) ([]byte, error) {
		return []byte(`{"error":[],"result":{"ledger":{},"count":0}}`), nil
	}
	normalize := func(asset string) string { return asset }
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := funding.Observe(post, normalize, "USD", time.Unix(100, 0)); err != nil {
			b.Fatal(err)
		}
	}
}
