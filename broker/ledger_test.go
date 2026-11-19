package broker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

type pagingHistoryClient struct {
	pages   [][]byte
	balance []byte
	n       atomic.Int32
}

func (c *pagingHistoryClient) Read(...string) ([]byte, error) { return nil, nil }
func (c *pagingHistoryClient) Write([]byte) error             { return nil }
func (c *pagingHistoryClient) Watch(string, any, json.Unmarshaler, func()) {
}
func (c *pagingHistoryClient) Post(url string, _ any) ([]byte, error) {
	if strings.Contains(url, "QueryOrders") {
		return []byte(`{"error":[],"result":{}}`), nil
	}

	if strings.Contains(url, "Balance") && !strings.Contains(url, "TradeBalance") {
		if len(c.balance) == 0 {
			return []byte(`{"error":[],"result":{}}`), nil
		}

		return c.balance, nil
	}

	i := int(c.n.Add(1) - 1)

	if i >= len(c.pages) {
		return nil, errors.New("fasthttp: timeout")
	}

	if c.pages[i] == nil {
		return nil, errors.New("fasthttp: timeout")
	}

	return c.pages[i], nil
}

func historyTrade(id, order, pair, side, vol string, sec int64) kraken.HistoryTrade {
	volume := decimal.RequireFromString(vol)
	price := decimal.RequireFromString("1")

	return kraken.HistoryTrade{
		ID:      id,
		OrderID: order,
		Pair:    pair,
		Type:    side,
		Volume:  &volume,
		Price:   &price,
		Time:    time.Unix(sec, 0).UTC(),
	}
}

func TestLedgerFold(t *testing.T) {
	Convey("Setup", t, func() {
		ledger := NewLedger("real", nil, nil, nil)

		Convey("When one pair opens and closes twice", func() {
			_, closed := ledger.Fold(context.Background(), []kraken.HistoryTrade{
				historyTrade("t1", "o1", "XBTUSD", "buy", "1", 1),
				historyTrade("t2", "o2", "XBTUSD", "sell", "1", 2),
				historyTrade("t3", "o3", "XBTUSD", "buy", "2", 3),
				historyTrade("t4", "o4", "XBTUSD", "sell", "2", 4),
			})

			Convey("Each cycle is its own closed holding", func() {
				So(len(closed), ShouldEqual, 2)
				So(closed[0].ID, ShouldNotEqual, closed[1].ID)
				So(len(closed[0].Fills), ShouldEqual, 2)
				So(len(closed[1].Fills), ShouldEqual, 2)
				So(closed[0].EntryVenueOrderID, ShouldEqual, "o1")
				So(closed[0].ExitVenueOrderID, ShouldEqual, "o2")
				So(closed[1].EntryVenueOrderID, ShouldEqual, "o3")
				So(closed[1].ClosedAt, ShouldNotBeNil)
			})
		})

		Convey("When two buys flatten on one sell", func() {
			_, closed := ledger.Fold(context.Background(), []kraken.HistoryTrade{
				historyTrade("t2", "o1", "XBTUSD", "buy", "0.4", 2),
				historyTrade("t1", "o1", "XBTUSD", "buy", "0.6", 1),
				historyTrade("t3", "o2", "XBTUSD", "sell", "1", 3),
			})

			Convey("Partial prints stay on one holding", func() {
				So(len(closed), ShouldEqual, 1)
				So(len(closed[0].Fills), ShouldEqual, 3)
				So(closed[0].Qty.String(), ShouldEqual, "0")
				So(closed[0].Fills[0].ExecID, ShouldEqual, "t1")
			})
		})

		Convey("When a sell arrives before any buy", func() {
			open, closed := ledger.Fold(context.Background(), []kraken.HistoryTrade{
				historyTrade("t1", "o1", "XBTUSD", "sell", "1", 1),
				historyTrade("t2", "o2", "XBTUSD", "buy", "1", 2),
			})

			Convey("The sell without an open cycle is ignored and the buy stays open", func() {
				So(len(closed), ShouldEqual, 0)
				So(len(open), ShouldEqual, 1)
				So(open[0].Qty.String(), ShouldEqual, "1")
				So(len(open[0].Fills), ShouldEqual, 1)
			})
		})

		Convey("When a sell only partly covers the buy", func() {
			open, closed := ledger.Fold(context.Background(), []kraken.HistoryTrade{
				historyTrade("t1", "o1", "ZECUSD", "buy", "1", 1),
				historyTrade("t2", "o2", "ZECUSD", "sell", "0.4", 2),
			})

			Convey("The holding stays open for the remainder", func() {
				So(len(closed), ShouldEqual, 0)
				So(len(open), ShouldEqual, 1)
				So(open[0].Qty.String(), ShouldEqual, "0.6")
				So(open[0].ClosedAt, ShouldBeNil)
				So(open[0].ExitVenueOrderID, ShouldEqual, "")
				So(open[0].ExitOrder, ShouldBeNil)
			})
		})

		Convey("When sell then buy share a truncated second but keep nanos", func() {
			buy := historyTrade("t2", "o2", "XBTUSD", "buy", "1", 100)
			sell := historyTrade("t1", "o1", "XBTUSD", "sell", "1", 100)
			sell.Time = time.Unix(100, 100_000_000).UTC()
			buy.Time = time.Unix(100, 200_000_000).UTC()

			open, closed := ledger.Fold(context.Background(), []kraken.HistoryTrade{
				historyTrade("t0", "o0", "XBTUSD", "buy", "1", 99),
				sell,
				buy,
			})

			Convey("Sell closes the first cycle; buy opens a new one", func() {
				So(len(closed), ShouldEqual, 1)
				So(closed[0].ExitVenueOrderID, ShouldEqual, "o1")
				So(len(open), ShouldEqual, 1)
				So(open[0].EntryVenueOrderID, ShouldEqual, "o2")
				So(open[0].Qty.String(), ShouldEqual, "1")
			})
		})
	})
}

func TestLedgerRestore(t *testing.T) {
	Convey("Setup", t, func() {
		if system.Cfg == nil || system.Cfg.WebSocket == nil {
			system.Cfg = system.NewConfig()
		}

		page1 := []byte(`{
			"error": [],
			"result": {
				"trades": {
					"t1": {
						"ordertxid": "o1",
						"pair": "ZECUSD",
						"type": "buy",
						"ordertype": "market",
						"price": "40.00",
						"cost": "40.00",
						"fee": "0.01",
						"vol": "1.0",
						"time": 1700000100.0
					}
				},
				"cursor": { "next": "page2" }
			}
		}`)

		Convey("When the second history page times out", func() {
			transport := &pagingHistoryClient{pages: [][]byte{page1, nil}}
			ledger := NewLedger("real", nil, transport, nil)
			open, closed := ledger.Restore(t.Context())

			Convey("Restore admits nothing", func() {
				So(open, ShouldBeNil)
				So(closed, ShouldBeNil)
			})
		})

		Convey("When the first history page fails", func() {
			ledger := NewLedger("real", nil, &pagingHistoryClient{pages: [][]byte{nil}}, nil)
			open, closed := ledger.Restore(t.Context())

			Convey("Restore does not treat it as an empty account", func() {
				So(open, ShouldBeNil)
				So(closed, ShouldBeNil)
			})
		})

		Convey("When history is a single complete page", func() {
			complete := []byte(`{
				"error": [],
				"result": {
					"trades": {
						"t1": {
							"ordertxid": "o1",
							"pair": "ZECUSD",
							"type": "buy",
							"ordertype": "market",
							"price": "40.00",
							"cost": "40.00",
							"fee": "0.01",
							"vol": "1.0",
							"time": 1700000100.0
						}
					},
					"cursor": {}
				}
			}`)
			ledger := NewLedger("real", nil, &pagingHistoryClient{
				pages:   [][]byte{complete},
				balance: []byte(`{"error":[],"result":{"ZEC":"1"}}`),
			}, nil)
			open, closed := ledger.Restore(t.Context())

			Convey("The leftover buy is open", func() {
				So(len(closed), ShouldEqual, 0)
				So(len(open), ShouldEqual, 1)
				So(open[0].Symbol, ShouldEqual, "ZECUSD")
			})
		})

		Convey("When history leftover has no wallet balance", func() {
			complete := []byte(`{
				"error": [],
				"result": {
					"trades": {
						"t1": {
							"ordertxid": "o1",
							"pair": "ZECUSD",
							"type": "buy",
							"ordertype": "market",
							"price": "40.00",
							"cost": "40.00",
							"fee": "0.01",
							"vol": "1.0",
							"time": 1700000100.0
						}
					},
					"cursor": {}
				}
			}`)
			ledger := NewLedger("real", nil, &pagingHistoryClient{
				pages:   [][]byte{complete},
				balance: []byte(`{"error":[],"result":{}}`),
			}, nil)
			open, closed := ledger.Restore(t.Context())

			Convey("Restore keeps closed empty and admits no open", func() {
				So(len(closed), ShouldEqual, 0)
				So(len(open), ShouldEqual, 0)
			})
		})

		Convey("When history residue is tiny but the wallet still holds the base", func() {
			complete := []byte(`{
				"error": [],
				"result": {
					"trades": {
						"t1": {
							"ordertxid": "o1",
							"pair": "DOGEUSD",
							"type": "buy",
							"ordertype": "market",
							"price": "0.1",
							"cost": "0.1",
							"fee": "0.01",
							"vol": "1.0",
							"time": 1700000100.0
						},
						"t2": {
							"ordertxid": "o2",
							"pair": "DOGEUSD",
							"type": "sell",
							"ordertype": "market",
							"price": "0.1",
							"cost": "0.099",
							"fee": "0.01",
							"vol": "0.999",
							"time": 1700000200.0
						}
					},
					"cursor": {}
				}
			}`)
			ledger := NewLedger("real", nil, &pagingHistoryClient{
				pages:   [][]byte{complete},
				balance: []byte(`{"error":[],"result":{"DOGE":"500"}}`),
			}, nil)
			open, _ := ledger.Restore(t.Context())

			Convey("Open qty follows the wallet, not the history dust", func() {
				So(len(open), ShouldEqual, 1)
				So(open[0].Symbol, ShouldEqual, "DOGEUSD")
				So(open[0].Qty.String(), ShouldEqual, "500")
				So(len(open[0].Fills), ShouldEqual, 1)
				So(open[0].Fills[0].ExecID, ShouldEqual, "wallet:DOGEUSD")
				So(open[0].Fills[0].LastQty.String(), ShouldEqual, "500")
			})
		})
	})
}

func BenchmarkLedgerFold(b *testing.B) {
	ledger := NewLedger("real", nil, nil, nil)
	trades := []kraken.HistoryTrade{
		historyTrade("t1", "o1", "XBTUSD", "buy", "1", 1),
		historyTrade("t2", "o2", "XBTUSD", "sell", "1", 2),
		historyTrade("t3", "o3", "XBTUSD", "buy", "2", 3),
		historyTrade("t4", "o4", "XBTUSD", "sell", "2", 4),
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = ledger.Fold(context.Background(), trades)
	}
}
