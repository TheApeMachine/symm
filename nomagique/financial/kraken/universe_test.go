package kraken

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/manifest"
)

/* universeFixture loads the shipping policy rather than inventing a second exclusion list. */
func universeFixture(t testing.TB) (string, []string) {
	t.Helper()
	encoded, err := manifest.ReadFile("live_spot")
	if err != nil {
		t.Fatal(err)
	}
	var graph struct {
		Nodes map[string]struct {
			InputData struct {
				Quote    struct{ Value string }
				Excluded struct{ Value []string }
			}
		}
	}
	if err := json.Unmarshal(encoded, &graph); err != nil {
		t.Fatal(err)
	}
	policy := graph.Nodes["universe"].InputData
	return policy.Quote.Value, policy.Excluded.Value
}

/* TestUniverseWrite verifies the shipping exclusion policy and complete source population. */
func TestUniverseWrite(t *testing.T) {
	Convey("The authored universe restores the legacy base-asset exclusions", t, func() {
		quote, excluded := universeFixture(t)
		So(quote, ShouldEqual, "USD")
		So(excluded, ShouldResemble, []string{"USD", "EUR", "GBP", "AUD", "CAD", "CHF", "JPY", "NZD", "USDT", "USDC", "DAI", "PYUSD", "FDUSD", "TUSD", "USDG", "USDE", "EURT", "EURC", "GUSD", "BUSD", "FRAX", "LUSD", "CUSD", "USD0", "USDS", "RLUSD", "UST"})
		client := Universe_ServerToClient(NewUniverse())
		defer client.Release()
		send := func(payload []byte) UniverseResult {
			So(client.Write(context.Background(), func(args Universe_write_Params) error {
				if err := args.SetData(payload); err != nil {
					return err
				}
				if err := args.SetQuote(quote); err != nil {
					return err
				}
				values, err := args.NewExcluded(int32(len(excluded)))
				if err != nil {
					return err
				}
				for index, asset := range excluded {
					if err := values.Set(index, asset); err != nil {
						return err
					}
				}
				return nil
			}), ShouldBeNil)
			future, release := client.Done(context.Background(), nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			// Keep the result alive through this test's assertions.
			t.Cleanup(release)
			return result
		}
		rows := []map[string]string{}
		for index := range 1001 {
			rows = append(rows, map[string]string{"symbol": fmt.Sprintf("COIN%d/USD", index), "base": fmt.Sprintf("COIN%d", index), "quote": "USD", "status": "online"})
		}
		for _, base := range append(excluded, "USDe") {
			rows = append(rows, map[string]string{"symbol": base + "/USD", "base": base, "quote": "USD", "status": "online"})
		}
		rows = append(rows, map[string]string{"symbol": "BTC/EUR", "base": "BTC", "quote": "EUR", "status": "online"}, map[string]string{"symbol": "HALTED/USD", "base": "HALTED", "quote": "USD", "status": "cancel_only"}, rows[0])
		encoded, err := json.Marshal(map[string]any{"channel": "instrument", "data": map[string]any{"pairs": rows}})
		So(err, ShouldBeNil)
		result := send(encoded)
		So(result.Which(), ShouldEqual, UniverseResult_Which_ready)
		symbols, err := result.Ready().Symbols()
		So(err, ShouldBeNil)
		So(symbols.Len(), ShouldEqual, 1001)
		for _, read := range []func() ([]byte, error){result.Ready().Ticker, result.Ready().Trade} {
			payload, err := read()
			So(err, ShouldBeNil)
			var subscription struct{ Params struct{ Symbol []string } }
			So(json.Unmarshal(payload, &subscription), ShouldBeNil)
			So(len(subscription.Params.Symbol), ShouldEqual, 1001)
			So(subscription.Params.Symbol[1000], ShouldEqual, "COIN1000/USD")
		}
		Convey("Market arrays and heartbeat frames do not produce subscriptions", func() {
			So(send([]byte(`{"channel":"ticker","data":[{"symbol":"BTC/USD"}]}`)).Which(), ShouldEqual, UniverseResult_Which_idle)
			So(send([]byte(`{"channel":"heartbeat"}`)).Which(), ShouldEqual, UniverseResult_Which_idle)
		})
		Convey("A reconnect snapshot and later listing remain eligible", func() {
			again := send(encoded)
			items, err := again.Ready().Symbols()
			So(err, ShouldBeNil)
			So(items.Len(), ShouldEqual, 1001)
			update := send([]byte(`{"channel":"instrument","type":"update","data":{"pairs":[{"symbol":"NEW/USD","base":"NEW","quote":"USD","status":"online"}]}}`))
			items, err = update.Ready().Symbols()
			So(err, ShouldBeNil)
			So(items.Len(), ShouldEqual, 1)
		})
	})
}

/* BenchmarkUniverseWrite covers a full population greater than the former ceiling. */
func BenchmarkUniverseWrite(b *testing.B) {
	quote, excluded := universeFixture(b)
	rows := make([]map[string]string, 1001)
	for index := range rows {
		rows[index] = map[string]string{"symbol": fmt.Sprintf("COIN%d/USD", index), "base": fmt.Sprintf("COIN%d", index), "quote": "USD", "status": "online"}
	}
	payload, err := json.Marshal(map[string]any{"channel": "instrument", "data": map[string]any{"pairs": rows}})
	if err != nil {
		b.Fatal(err)
	}
	client := Universe_ServerToClient(NewUniverse())
	defer client.Release()
	b.ReportAllocs()
	for b.Loop() {
		if err := client.Write(context.Background(), func(args Universe_write_Params) error {
			if err := args.SetData(payload); err != nil {
				return err
			}
			if err := args.SetQuote(quote); err != nil {
				return err
			}
			values, err := args.NewExcluded(int32(len(excluded)))
			if err != nil {
				return err
			}
			for index, value := range excluded {
				if err := values.Set(index, value); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(context.Background(), nil)
		if _, err := future.Struct(); err != nil {
			release()
			b.Fatal(err)
		}
		release()
	}
}
