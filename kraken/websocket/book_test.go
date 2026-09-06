package websocket

import (
	"fmt"
	"strings"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	sdk "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func newBookFixture(
	testingTB testing.TB,
	symbol string,
	pricePrecision int,
	quantityPrecision int,
) *Book {
	testingTB.Helper()
	base, quote, found := strings.Cut(symbol, "/")

	if !found {
		testingTB.Fatalf("book fixture pair required: %s", symbol)
	}

	normalizer := spot.NewNormalizer()
	tickSizeText := "1"

	if pricePrecision > 0 {
		tickSizeText = "0." + strings.Repeat("0", pricePrecision-1) + "1"
	}

	tickSize, err := decimal.NewFromString(tickSizeText)

	if err != nil {
		testingTB.Fatalf("book fixture tick size: %v", err)
	}

	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			base:  {AltName: base},
			quote: {AltName: quote},
		},
		NewPairs: map[string]spot.AssetPair{
			base + quote: {
				WSName: symbol, Base: base, Quote: quote,
				PairDecimals: pricePrecision, LotDecimals: quantityPrecision,
				LotMultiplier: 1, TickSize: tickSize,
			},
		},
	})

	return NewBook(testingTB.Context(), normalizer)
}

func TestBookAll(t *testing.T) {
	Convey("Given a reconstructed Level3 book", t, func() {
		managed := newBookFixture(t, "BTC/USD", 0, 0)
		managed.Create("BTC/USD", 32)
		event := &callback.Event[*sdk.WebSocketMessage]{
			Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
		}

		for index := range 32 {
			price := decimal.NewFromInt64(int64(100 + index))
			payload := &kraken.Level3{Data: []kraken.Level3Data{{
				Symbol: "BTC/USD",
				Bids: []kraken.Level3Order{{
					OrderID:    fmt.Sprintf("bid-%d", index),
					LimitPrice: price,
					OrderQty:   decimal.NewFromInt64(1),
					Timestamp:  time.Now().UTC(),
				}},
			}}}

			So(managed.Update(event, payload), ShouldBeNil)
		}

		Convey("All should expose the managed book", func() {
			value, found := managed.All().Load("BTC/USD")
			So(found, ShouldBeTrue)
			managed.Book("BTC/USD", func(current *spotbook.Book) {
				So(value.(*spotbook.Book), ShouldEqual, current)
			})
		})
	})
}

func TestBookUpdate(t *testing.T) {
	Convey("Given authoritative fixed-point Level 3 decimals", t, func() {
		managed := newBookFixture(t, "BTC/USD", 1, 8)
		managed.Create("BTC/USD", 10)
		event := &callback.Event[*sdk.WebSocketMessage]{
			Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
		}
		rawSnapshot := []byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"BTC/USD","checksum":1063832831,"bids":[{"order_id":"OTCFZG-YOE2Q-LQKNM3","limit_price":"44939.4","order_qty":"0.88968699","timestamp":"2024-01-08T12:26:39.526146327Z"},{"order_id":"OFGP5R-B3E7G-54EZD6","limit_price":"44939.4","order_qty":"0.45210000","timestamp":"2024-01-08T12:26:39.530287934Z"},{"order_id":"OMPHVY-IZPJ4-KOKA3P","limit_price":"44939.4","order_qty":"0.10000000","timestamp":"2024-01-08T12:26:39.576380340Z"},{"order_id":"OAI5QZ-AMPLW-NBNO72","limit_price":"44939.4","order_qty":"0.14296323","timestamp":"2024-01-08T12:26:39.602118534Z"},{"order_id":"O7VFZI-CTFWH-FF6EIR","limit_price":"44939.4","order_qty":"0.25000000","timestamp":"2024-01-08T12:26:41.780601700Z"},{"order_id":"O472V3-ZG4EZ-OLD66C","limit_price":"44939.4","order_qty":"0.10292988","timestamp":"2024-01-08T12:26:43.087136366Z"},{"order_id":"OEK26P-BGPUK-LDHMD2","limit_price":"44939.4","order_qty":"0.33880000","timestamp":"2024-01-08T12:26:43.822433365Z"},{"order_id":"OSMYPE-S5VOC-YSS3WM","limit_price":"44939.4","order_qty":"1.28140860","timestamp":"2024-01-08T12:26:45.066096694Z"},{"order_id":"OJPMIN-NXZL5-SOWP6V","limit_price":"44937.1","order_qty":"0.03346877","timestamp":"2024-01-08T12:26:39.691304329Z"},{"order_id":"O6PUGE-SQWYQ-TRJEEE","limit_price":"44934.7","order_qty":"0.35630000","timestamp":"2024-01-08T12:26:44.129718463Z"},{"order_id":"OPUOGC-Q532V-3OKLPM","limit_price":"44930.2","order_qty":"0.22734299","timestamp":"2024-01-08T12:26:30.769031831Z"},{"order_id":"OCIU7J-VB3CI-HPULSF","limit_price":"44930.2","order_qty":"0.01000000","timestamp":"2024-01-08T12:26:36.054352106Z"},{"order_id":"ORWVAF-LJFLY-ZWEHDQ","limit_price":"44930.2","order_qty":"0.05550000","timestamp":"2024-01-08T12:26:36.635882793Z"},{"order_id":"OYRAHE-PI5AN-7KOQ4E","limit_price":"44930.2","order_qty":"0.70000000","timestamp":"2024-01-08T12:26:37.296554518Z"},{"order_id":"OGBHYU-UILDD-6DLLYJ","limit_price":"44930.2","order_qty":"0.15000000","timestamp":"2024-01-08T12:26:41.222733191Z"},{"order_id":"O74ZBU-K2TKC-R76XSW","limit_price":"44928.0","order_qty":"0.00105240","timestamp":"2024-01-08T12:26:23.542563322Z"},{"order_id":"OQVTQF-Y56MR-BM6LWL","limit_price":"44919.6","order_qty":"0.33870000","timestamp":"2024-01-08T12:26:42.808132842Z"},{"order_id":"OYEH6U-ZCHA2-3HFR3W","limit_price":"44919.5","order_qty":"0.07610000","timestamp":"2024-01-08T12:26:34.269600037Z"},{"order_id":"OLGPG7-HVKXU-J6SANK","limit_price":"44912.0","order_qty":"0.35630000","timestamp":"2024-01-08T12:26:34.961292766Z"},{"order_id":"OHGC3L-FRZQ3-UIVZRU","limit_price":"44909.7","order_qty":"0.06690000","timestamp":"2024-01-08T12:26:31.912880024Z"},{"order_id":"O73C6Y-VZXYA-H4LDFY","limit_price":"44901.9","order_qty":"0.00088982","timestamp":"2024-01-08T12:26:42.883315043Z"}],"asks":[{"order_id":"OFVLAA-HRSSP-BK75KB","limit_price":"44939.5","order_qty":"4.52308393","timestamp":"2024-01-08T12:18:05.770906486Z"},{"order_id":"OYBAMK-O5DKX-WMPUTM","limit_price":"44939.5","order_qty":"0.00111261","timestamp":"2024-01-08T12:18:12.847426441Z"},{"order_id":"O3DRCT-J5M2S-KYV526","limit_price":"44939.5","order_qty":"0.00100000","timestamp":"2024-01-08T12:26:42.108176464Z"},{"order_id":"OF3X3A-72WZY-6EKA5F","limit_price":"44939.5","order_qty":"0.01000000","timestamp":"2024-01-08T12:26:43.955098263Z"},{"order_id":"OF5UA6-6IIZ2-YGQTSJ","limit_price":"44950.0","order_qty":"0.10334926","timestamp":"2024-01-08T12:25:52.800473795Z"},{"order_id":"OSDOZX-7UZ6Y-QDNPVI","limit_price":"44953.0","order_qty":"0.00064537","timestamp":"2024-01-08T12:24:58.086806970Z"},{"order_id":"OV7KTS-A2TWV-3XKRIA","limit_price":"44955.0","order_qty":"0.00250000","timestamp":"2024-01-08T12:21:52.257936228Z"},{"order_id":"OOF2V5-RYOHC-GLRNPM","limit_price":"44959.6","order_qty":"0.35630000","timestamp":"2024-01-08T12:26:44.202823127Z"},{"order_id":"OTVOVS-QLST3-3JG7JI","limit_price":"44959.6","order_qty":"0.35630000","timestamp":"2024-01-08T12:26:44.203383999Z"},{"order_id":"OGZCIU-RDQ77-DAAL3P","limit_price":"44960.1","order_qty":"0.00338072","timestamp":"2024-01-08T12:26:42.724829715Z"},{"order_id":"OVLG3E-HYBQM-CWNGCY","limit_price":"44960.2","order_qty":"0.88967575","timestamp":"2024-01-08T12:26:12.935924248Z"},{"order_id":"OWEOFO-HUCJC-T37MVO","limit_price":"44967.0","order_qty":"3.14392283","timestamp":"2024-01-08T12:26:39.474431925Z"},{"order_id":"OVYTHY-D2N76-5QYREQ","limit_price":"44978.5","order_qty":"0.06778960","timestamp":"2024-01-08T12:26:41.229379178Z"},{"order_id":"OFO525-PHRVS-236RMN","limit_price":"44979.2","order_qty":"0.35630000","timestamp":"2024-01-08T12:26:20.271584488Z"}]}]}`)
		payload := kraken.NewLevel3(rawSnapshot)

		Convey("It should validate the official Kraken Level 3 CRC32 checksum", func() {
			So(managed.Update(event, payload), ShouldBeNil)
			managed.Book("BTC/USD", func(book *spotbook.Book) {
				So(book.BestAsk().Price.String(), ShouldEqual, "44939.5")
				So(book.BestBid().Price.String(), ShouldEqual, "44939.4")
			})
		})
	})

	Convey("Given a Level 3 cache connected to its owning transport", t, func() {
		managed := newBookFixture(t, "BTC/USD", 0, 0)
		managed.Create("BTC/USD", 32)
		updates := make(chan string, 1)
		managed.SetNotify(func(symbol string, _ time.Time) { updates <- symbol })
		event := &callback.Event[*sdk.WebSocketMessage]{
			Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
		}
		payload := &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{{
				Event: "add", OrderID: "bid", LimitPrice: decimal.NewFromInt64(100),
				OrderQty: decimal.NewFromInt64(1), Timestamp: time.Unix(1, 0).UTC(),
			}},
		}}}

		err := managed.Update(event, payload)

		Convey("It should publish the symbol only after applying the update", func() {
			So(err, ShouldBeNil)
			So(<-updates, ShouldEqual, "BTC/USD")
			managed.Book("BTC/USD", func(book *spotbook.Book) {
				So(book.BestBid(), ShouldNotBeNil)
			})
		})
	})

	Convey("Given backpressure from the Level3 event consumer", t, func() {
		managed := newBookFixture(t, "BTC/USD", 0, 0)
		managed.Create("BTC/USD", 32)
		notifyEntered := make(chan struct{})
		notifyBlock := make(chan struct{})
		managed.SetNotify(func(symbol string, _ time.Time) {
			close(notifyEntered)
			<-notifyBlock
		})
		event := &callback.Event[*sdk.WebSocketMessage]{
			Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
		}
		payload := &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{{
				Event: "add", OrderID: "bid", LimitPrice: decimal.NewFromInt64(100),
				OrderQty: decimal.NewFromInt64(1), Timestamp: time.Unix(1, 0).UTC(),
			}},
		}}}
		updateDone := make(chan error, 1)

		go func() {
			updateDone <- managed.Update(event, payload)
		}()

		<-notifyEntered
		readDone := make(chan struct{})
		bestBidFound := false

		go func() {
			managed.Book("BTC/USD", func(book *spotbook.Book) {
				bestBidFound = book.BestBid() != nil
			})
			close(readDone)
		}()

		readBeforeDrain := false

		select {
		case <-readDone:
			readBeforeDrain = true
		case <-time.After(100 * time.Millisecond):
		}

		close(notifyBlock)

		if !readBeforeDrain {
			<-readDone
		}

		Convey("It should release the book before waiting for the consumer", func() {
			So(readBeforeDrain, ShouldBeTrue)
			So(bestBidFound, ShouldBeTrue)
			So(<-updateDone, ShouldBeNil)
		})
	})

	Convey("Given two Level3 orders at one price", t, func() {
		managed := newBookFixture(t, "BTC/USD", 0, 0)
		managed.Create("BTC/USD", 32)
		event := &callback.Event[*sdk.WebSocketMessage]{
			Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
		}
		price := decimal.NewFromInt64(100)
		at := time.Unix(1_700_008_000, 0).UTC()
		payload := &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{
				{
					Event: "add", OrderID: "bid-one", LimitPrice: price,
					OrderQty: decimal.NewFromInt64(3), Timestamp: at,
				},
				{
					Event: "add", OrderID: "bid-two", LimitPrice: price,
					OrderQty: decimal.NewFromInt64(2), Timestamp: at,
				},
			},
		}}}
		So(managed.Update(event, payload), ShouldBeNil)

		Convey("Deleting orders should remove their actual queued quantity", func() {
			deletion := func(orderID string) *kraken.Level3 {
				return &kraken.Level3{Data: []kraken.Level3Data{{
					Symbol: "BTC/USD",
					Bids: []kraken.Level3Order{{
						Event: "delete", OrderID: orderID,
						LimitPrice: price, Timestamp: at.Add(time.Second),
					}},
				}}}
			}

			So(managed.Update(event, deletion("bid-one")), ShouldBeNil)
			managed.Book("BTC/USD", func(book *spotbook.Book) {
				So(book.Bids.Levels, ShouldHaveLength, 1)
				So(book.BestBid().Quantity.Float64(), ShouldEqual, 2.0)
			})

			So(managed.Update(event, deletion("bid-two")), ShouldBeNil)
			managed.Book("BTC/USD", func(book *spotbook.Book) {
				So(book.Bids.Levels, ShouldBeEmpty)
			})
		})

		Convey("Deleting orders without LimitPrice should locate and remove them", func() {
			delManaged := newBookFixture(t, "BTC/USD", 2, 0)
			delManaged.Create("BTC/USD", 32)
			at := time.Unix(1_700_008_000, 0).UTC()
			payload := &kraken.Level3{Data: []kraken.Level3Data{{
				Symbol: "BTC/USD",
				Bids: []kraken.Level3Order{
					{
						OrderID: "b1", LimitPrice: decimal.NewFromFloat64(2.50),
						OrderQty: decimal.NewFromInt64(100000), Timestamp: at,
					},
					{
						OrderID: "b2", LimitPrice: decimal.NewFromFloat64(2.60),
						OrderQty: decimal.NewFromInt64(100000), Timestamp: at,
					},
				},
			}}}
			So(delManaged.Update(event, payload), ShouldBeNil)

			deletePayload := &kraken.Level3{Data: []kraken.Level3Data{{
				Symbol: "BTC/USD",
				Bids: []kraken.Level3Order{{
					Event: "delete", OrderID: "b2",
				}},
			}}}
			So(delManaged.Update(event, deletePayload), ShouldBeNil)
			delManaged.Get("BTC/USD", func(book *spotbook.Book) {
				So(book.BestBid().Price.String(), ShouldEqual, "2.5")
			})
		})
	})

	Convey("Given a snapshot and updates whose checksums disagree with local state", t, func() {
		managed := newBookFixture(t, "CELR/USD", 7, 5)
		managed.Create("CELR/USD", 10)
		resynced := make(chan string, 4)
		managed.SetResync(func(symbol string) { resynced <- symbol })
		event := &callback.Event[*sdk.WebSocketMessage]{
			Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
		}
		bidPrice, err := decimal.NewFromString("0.0003400")
		So(err, ShouldBeNil)
		bidQuantity, err := decimal.NewFromString("2000.00000")
		So(err, ShouldBeNil)
		askPrice, err := decimal.NewFromString("0.0003500")
		So(err, ShouldBeNil)
		askQuantity, err := decimal.NewFromString("1234.50000")
		So(err, ShouldBeNil)
		lateBidPrice, err := decimal.NewFromString("0.0003300")
		So(err, ShouldBeNil)
		lateBidQuantity, err := decimal.NewFromString("5.00000")
		So(err, ShouldBeNil)

		knownGood := &kraken.Level3{
			Type: "snapshot",
			Data: []kraken.Level3Data{{
				Symbol:   "CELR/USD",
				Checksum: 3152022922,
				Bids: []kraken.Level3Order{{
					OrderID: "bid", LimitPrice: bidPrice,
					OrderQty: bidQuantity, Timestamp: time.Unix(1, 0).UTC(),
				}},
				Asks: []kraken.Level3Order{{
					OrderID: "ask", LimitPrice: askPrice,
					OrderQty: askQuantity, Timestamp: time.Unix(1, 0).UTC(),
				}},
			}},
		}

		Convey("It should diverge once, drop further deltas, and recover on the resubscription snapshot", func() {
			So(managed.Update(event, knownGood), ShouldBeNil)

			// A wrong server checksum for state that is known good: local
			// state is untrustworthy from here on.
			diverged := &kraken.Level3{Data: []kraken.Level3Data{{
				Symbol:   "CELR/USD",
				Checksum: 1,
				Bids: []kraken.Level3Order{{
					Event: "add", OrderID: "late-bid",
					LimitPrice: lateBidPrice,
					OrderQty:   lateBidQuantity, Timestamp: time.Unix(2, 0).UTC(),
				}},
			}}}

			So(managed.Update(event, diverged), ShouldNotBeNil)
			So(<-resynced, ShouldEqual, "CELR/USD")
			managed.Book("CELR/USD", func(book *spotbook.Book) {
				So(book.Bids.Levels, ShouldBeEmpty)
			})

			// Deltas for a diverged symbol are dropped without touching the
			// empty book and without asking for a second resubscription.
			So(managed.Update(event, diverged), ShouldBeNil)

			select {
			case symbol := <-resynced:
				So(symbol, ShouldBeNil)
			default:
			}

			managed.Book("CELR/USD", func(book *spotbook.Book) {
				So(book.Bids.Levels, ShouldBeEmpty)
			})

			// The resubscription snapshot is authoritative again.
			So(managed.Update(event, knownGood), ShouldBeNil)
			managed.Book("CELR/USD", func(book *spotbook.Book) {
				So(book.BestBid(), ShouldNotBeNil)
				So(book.BestAsk(), ShouldNotBeNil)
			})

			accepted := &kraken.Level3{Data: []kraken.Level3Data{{
				Symbol: "CELR/USD",
				Bids: []kraken.Level3Order{{
					Event: "add", OrderID: "fresh-bid",
					LimitPrice: lateBidPrice,
					OrderQty:   lateBidQuantity, Timestamp: time.Unix(3, 0).UTC(),
				}},
			}}}

			So(managed.Update(event, accepted), ShouldBeNil)
			managed.Book("CELR/USD", func(book *spotbook.Book) {
				So(book.BestBid(), ShouldNotBeNil)
			})
		})
	})

	Convey("Given a venue frame whose intermediate state is crossed", t, func() {
		managed := newBookFixture(t, "APR/USD", 0, 0)
		managed.Create("APR/USD", 10)
		event := &callback.Event[*sdk.WebSocketMessage]{
			Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
		}
		initial := &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "APR/USD",
			Asks: []kraken.Level3Order{
				{
					Event: "add", OrderID: "consumed", LimitPrice: decimal.NewFromInt64(99),
					OrderQty: decimal.NewFromInt64(1), Timestamp: time.Unix(1, 0).UTC(),
				},
				{
					Event: "add", OrderID: "resting", LimitPrice: decimal.NewFromInt64(100),
					OrderQty: decimal.NewFromInt64(1), Timestamp: time.Unix(3, 0).UTC(),
				},
			},
		}}}
		So(managed.Update(event, initial), ShouldBeNil)
		update := &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "APR/USD",
			Bids: []kraken.Level3Order{{
				Event: "add", OrderID: "new-bid", LimitPrice: decimal.NewFromInt64(100),
				OrderQty: decimal.NewFromInt64(1), Timestamp: time.Unix(2, 0).UTC(),
			}},
			Asks: []kraken.Level3Order{{
				Event: "delete", OrderID: "consumed", LimitPrice: decimal.NewFromInt64(99),
				Timestamp: time.Unix(2, 1).UTC(),
			}},
		}}}

		err := managed.Update(event, update)

		Convey("It should retain the venue's final locked book for checksum validation", func() {
			So(err, ShouldBeNil)
			managed.Book("APR/USD", func(book *spotbook.Book) {
				So(book.NoBookCrossing, ShouldBeFalse)
				So(book.BestBid(), ShouldNotBeNil)
				So(book.BestAsk(), ShouldNotBeNil)
				So(book.BestBid().Price.Cmp(book.BestAsk().Price), ShouldEqual, 0)
			})
		})
	})
}

func TestBookApply(t *testing.T) {
	Convey("Given a depth-limited book and a multi-order venue update", t, func() {
		managed := newBookFixture(t, "BTC/USD", 0, 0)
		managed.Create("BTC/USD", 2)
		order := func(event, id string, price, quantity int64) kraken.Level3Order {
			return kraken.Level3Order{Event: event, OrderID: id, LimitPrice: decimal.NewFromInt64(price), OrderQty: decimal.NewFromInt64(quantity), Timestamp: time.Unix(price, 0)}
		}
		frame := func(orders ...kraken.Level3Order) *kraken.Level3 {
			return &kraken.Level3{Type: "update", Data: []kraken.Level3Data{{Symbol: "BTC/USD", Bids: orders}}}
		}
		_, _, err := managed.apply(frame(order("add", "best", 100, 1), order("add", "worst", 99, 2)))
		So(err, ShouldBeNil)

		Convey("Truncation occurs after the whole frame, so later deletion can still address the old worst level", func() {
			accepted, resynced, err := managed.apply(frame(order("add", "new", 101, 3), order("delete", "worst", 99, 0)))
			So(err, ShouldBeNil)
			So(resynced, ShouldBeEmpty)
			So(accepted, ShouldHaveLength, 1)
			managed.Book("BTC/USD", func(current *spotbook.Book) {
				So(current.Bids.Levels, ShouldHaveLength, 2)
				So(current.Bids.Low.Price.Float64(), ShouldEqual, 100)
			})
		})

		Convey("An unknown deleted level fails explicitly and requests exactly one fresh snapshot", func() {
			bad := frame(order("delete", "missing", 98, 0))
			accepted, resynced, err := managed.apply(bad)
			So(err, ShouldNotBeNil)
			So(accepted, ShouldBeEmpty)
			So(resynced, ShouldResemble, []string{"BTC/USD"})
			accepted, resynced, err = managed.apply(bad)
			So(err, ShouldBeNil)
			So(accepted, ShouldBeEmpty)
			So(resynced, ShouldBeEmpty)
			snapshot := frame(order("add", "restored", 100, 1))
			snapshot.Type = "snapshot"
			_, _, err = managed.apply(snapshot)
			So(err, ShouldBeNil)
			_, _, err = managed.apply(frame(order("delete", "restored", 100, 0)))
			So(err, ShouldBeNil)
		})
	})
}

func BenchmarkBookAll(b *testing.B) {
	managed := newBookFixture(b, "BTC/USD", 0, 0)
	managed.Create("BTC/USD", 256)
	event := &callback.Event[*sdk.WebSocketMessage]{
		Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
	}

	for index := range 256 {
		price := decimal.NewFromInt64(int64(100 + index))
		payload := &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{{
				OrderID:    fmt.Sprintf("bid-%d", index),
				LimitPrice: price,
				OrderQty:   decimal.NewFromInt64(1),
				Timestamp:  time.Unix(int64(index), 0).UTC(),
			}},
		}}}

		if err := managed.Update(event, payload); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		managed.All()
	}
}

func BenchmarkBookGet(b *testing.B) {
	managed := newBookFixture(b, "BTC/USD", 0, 0)
	managed.Create("BTC/USD", 32)
	read := func(*spotbook.Book) {}
	b.ReportAllocs()

	for b.Loop() {
		managed.Book("BTC/USD", read)
	}
}

func BenchmarkBookUpdate(b *testing.B) {
	managed := newBookFixture(b, "BTC/USD", 0, 0)
	managed.Create("BTC/USD", 32)
	event := &callback.Event[*sdk.WebSocketMessage]{
		Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
	}
	price := decimal.NewFromInt64(100)
	quantity := decimal.NewFromInt64(1)
	at := time.Unix(1_700_008_000, 0).UTC()

	b.ReportAllocs()

	for b.Loop() {
		if err := managed.Update(event, &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{{
				Event: "add", OrderID: "bid-one", LimitPrice: price,
				OrderQty: quantity, Timestamp: at,
			}},
		}}}); err != nil {
			b.Fatal(err)
		}
		if err := managed.Update(event, &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{{
				Event: "delete", OrderID: "bid-one", LimitPrice: price,
				Timestamp: at.Add(time.Second),
			}},
		}}}); err != nil {
			b.Fatal(err)
		}
	}
}
