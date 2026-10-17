package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
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
		if err := managed.Create("BTC/USD", 32); err != nil {
			t.Fatal(err)
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

			So(managed.Update(payload), ShouldBeNil)
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
		if err := managed.Create("BTC/USD", 10); err != nil {
			t.Fatal(err)
		}
		rawSnapshot := []byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"BTC/USD","checksum":1063832831,"bids":[{"order_id":"OTCFZG-YOE2Q-LQKNM3","limit_price":"44939.4","order_qty":"0.88968699","timestamp":"2024-01-08T12:26:39.526146327Z"},{"order_id":"OFGP5R-B3E7G-54EZD6","limit_price":"44939.4","order_qty":"0.45210000","timestamp":"2024-01-08T12:26:39.530287934Z"},{"order_id":"OMPHVY-IZPJ4-KOKA3P","limit_price":"44939.4","order_qty":"0.10000000","timestamp":"2024-01-08T12:26:39.576380340Z"},{"order_id":"OAI5QZ-AMPLW-NBNO72","limit_price":"44939.4","order_qty":"0.14296323","timestamp":"2024-01-08T12:26:39.602118534Z"},{"order_id":"O7VFZI-CTFWH-FF6EIR","limit_price":"44939.4","order_qty":"0.25000000","timestamp":"2024-01-08T12:26:41.780601700Z"},{"order_id":"O472V3-ZG4EZ-OLD66C","limit_price":"44939.4","order_qty":"0.10292988","timestamp":"2024-01-08T12:26:43.087136366Z"},{"order_id":"OEK26P-BGPUK-LDHMD2","limit_price":"44939.4","order_qty":"0.33880000","timestamp":"2024-01-08T12:26:43.822433365Z"},{"order_id":"OSMYPE-S5VOC-YSS3WM","limit_price":"44939.4","order_qty":"1.28140860","timestamp":"2024-01-08T12:26:45.066096694Z"},{"order_id":"OJPMIN-NXZL5-SOWP6V","limit_price":"44937.1","order_qty":"0.03346877","timestamp":"2024-01-08T12:26:39.691304329Z"},{"order_id":"O6PUGE-SQWYQ-TRJEEE","limit_price":"44934.7","order_qty":"0.35630000","timestamp":"2024-01-08T12:26:44.129718463Z"},{"order_id":"OPUOGC-Q532V-3OKLPM","limit_price":"44930.2","order_qty":"0.22734299","timestamp":"2024-01-08T12:26:30.769031831Z"},{"order_id":"OCIU7J-VB3CI-HPULSF","limit_price":"44930.2","order_qty":"0.01000000","timestamp":"2024-01-08T12:26:36.054352106Z"},{"order_id":"ORWVAF-LJFLY-ZWEHDQ","limit_price":"44930.2","order_qty":"0.05550000","timestamp":"2024-01-08T12:26:36.635882793Z"},{"order_id":"OYRAHE-PI5AN-7KOQ4E","limit_price":"44930.2","order_qty":"0.70000000","timestamp":"2024-01-08T12:26:37.296554518Z"},{"order_id":"OGBHYU-UILDD-6DLLYJ","limit_price":"44930.2","order_qty":"0.15000000","timestamp":"2024-01-08T12:26:41.222733191Z"},{"order_id":"O74ZBU-K2TKC-R76XSW","limit_price":"44928.0","order_qty":"0.00105240","timestamp":"2024-01-08T12:26:23.542563322Z"},{"order_id":"OQVTQF-Y56MR-BM6LWL","limit_price":"44919.6","order_qty":"0.33870000","timestamp":"2024-01-08T12:26:42.808132842Z"},{"order_id":"OYEH6U-ZCHA2-3HFR3W","limit_price":"44919.5","order_qty":"0.07610000","timestamp":"2024-01-08T12:26:34.269600037Z"},{"order_id":"OLGPG7-HVKXU-J6SANK","limit_price":"44912.0","order_qty":"0.35630000","timestamp":"2024-01-08T12:26:34.961292766Z"},{"order_id":"OHGC3L-FRZQ3-UIVZRU","limit_price":"44909.7","order_qty":"0.06690000","timestamp":"2024-01-08T12:26:31.912880024Z"},{"order_id":"O73C6Y-VZXYA-H4LDFY","limit_price":"44901.9","order_qty":"0.00088982","timestamp":"2024-01-08T12:26:42.883315043Z"}],"asks":[{"order_id":"OFVLAA-HRSSP-BK75KB","limit_price":"44939.5","order_qty":"4.52308393","timestamp":"2024-01-08T12:18:05.770906486Z"},{"order_id":"OYBAMK-O5DKX-WMPUTM","limit_price":"44939.5","order_qty":"0.00111261","timestamp":"2024-01-08T12:18:12.847426441Z"},{"order_id":"O3DRCT-J5M2S-KYV526","limit_price":"44939.5","order_qty":"0.00100000","timestamp":"2024-01-08T12:26:42.108176464Z"},{"order_id":"OF3X3A-72WZY-6EKA5F","limit_price":"44939.5","order_qty":"0.01000000","timestamp":"2024-01-08T12:26:43.955098263Z"},{"order_id":"OF5UA6-6IIZ2-YGQTSJ","limit_price":"44950.0","order_qty":"0.10334926","timestamp":"2024-01-08T12:25:52.800473795Z"},{"order_id":"OSDOZX-7UZ6Y-QDNPVI","limit_price":"44953.0","order_qty":"0.00064537","timestamp":"2024-01-08T12:24:58.086806970Z"},{"order_id":"OV7KTS-A2TWV-3XKRIA","limit_price":"44955.0","order_qty":"0.00250000","timestamp":"2024-01-08T12:21:52.257936228Z"},{"order_id":"OOF2V5-RYOHC-GLRNPM","limit_price":"44959.6","order_qty":"0.35630000","timestamp":"2024-01-08T12:26:44.202823127Z"},{"order_id":"OTVOVS-QLST3-3JG7JI","limit_price":"44959.6","order_qty":"0.35630000","timestamp":"2024-01-08T12:26:44.203383999Z"},{"order_id":"OGZCIU-RDQ77-DAAL3P","limit_price":"44960.1","order_qty":"0.00338072","timestamp":"2024-01-08T12:26:42.724829715Z"},{"order_id":"OVLG3E-HYBQM-CWNGCY","limit_price":"44960.2","order_qty":"0.88967575","timestamp":"2024-01-08T12:26:12.935924248Z"},{"order_id":"OWEOFO-HUCJC-T37MVO","limit_price":"44967.0","order_qty":"3.14392283","timestamp":"2024-01-08T12:26:39.474431925Z"},{"order_id":"OVYTHY-D2N76-5QYREQ","limit_price":"44978.5","order_qty":"0.06778960","timestamp":"2024-01-08T12:26:41.229379178Z"},{"order_id":"OFO525-PHRVS-236RMN","limit_price":"44979.2","order_qty":"0.35630000","timestamp":"2024-01-08T12:26:20.271584488Z"}]}]}`)
		payload := kraken.NewLevel3(rawSnapshot)

		Convey("It should validate the official Kraken Level 3 CRC32 checksum", func() {
			So(managed.Update(payload), ShouldBeNil)
			managed.Book("BTC/USD", func(book *spotbook.Book) {
				So(book.BestAsk().Price.String(), ShouldEqual, "44939.5")
				So(book.BestBid().Price.String(), ShouldEqual, "44939.4")
			})
		})
	})

	Convey("Given a Level 3 cache connected to its owning transport", t, func() {
		managed := newBookFixture(t, "BTC/USD", 0, 0)
		if err := managed.Create("BTC/USD", 32); err != nil {
			t.Fatal(err)
		}
		updates := make(chan string, 1)
		managed.SetNotify(func(symbol string, _ time.Time) { updates <- symbol })

		payload := &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{{
				Event: "add", OrderID: "bid", LimitPrice: decimal.NewFromInt64(100),
				OrderQty: decimal.NewFromInt64(1), Timestamp: time.Unix(1, 0).UTC(),
			}},
		}}}

		err := managed.Update(payload)

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
		if err := managed.Create("BTC/USD", 32); err != nil {
			t.Fatal(err)
		}
		notifyEntered := make(chan struct{})
		notifyBlock := make(chan struct{})
		managed.SetNotify(func(symbol string, _ time.Time) {
			close(notifyEntered)
			<-notifyBlock
		})

		payload := &kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{{
				Event: "add", OrderID: "bid", LimitPrice: decimal.NewFromInt64(100),
				OrderQty: decimal.NewFromInt64(1), Timestamp: time.Unix(1, 0).UTC(),
			}},
		}}}
		updateDone := make(chan error, 1)

		go func() {
			updateDone <- managed.Update(payload)
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
		if err := managed.Create("BTC/USD", 32); err != nil {
			t.Fatal(err)
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
		So(managed.Update(payload), ShouldBeNil)

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

			So(managed.Update(deletion("bid-one")), ShouldBeNil)
			managed.Book("BTC/USD", func(book *spotbook.Book) {
				So(book.Bids.Levels, ShouldHaveLength, 1)
				So(book.BestBid().Quantity.Float64(), ShouldEqual, 2.0)
			})

			So(managed.Update(deletion("bid-two")), ShouldBeNil)
			managed.Book("BTC/USD", func(book *spotbook.Book) {
				So(book.Bids.Levels, ShouldBeEmpty)
			})
		})

		Convey("Deleting orders without LimitPrice should locate and remove them", func() {
			delManaged := newBookFixture(t, "BTC/USD", 2, 0)
			if err := delManaged.Create("BTC/USD", 32); err != nil {
				t.Fatal(err)
			}
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
			So(delManaged.Update(payload), ShouldBeNil)

			deletePayload := &kraken.Level3{Data: []kraken.Level3Data{{
				Symbol: "BTC/USD",
				Bids: []kraken.Level3Order{{
					Event: "delete", OrderID: "b2",
				}},
			}}}
			So(delManaged.Update(deletePayload), ShouldBeNil)
			delManaged.Book("BTC/USD", func(book *spotbook.Book) {
				So(book.BestBid().Price.String(), ShouldEqual, "2.50")
			})
		})
	})

	Convey("Given a snapshot and updates whose checksums disagree with local state", t, func() {
		managed := newBookFixture(t, "CELR/USD", 7, 5)
		if err := managed.Create("CELR/USD", 10); err != nil {
			t.Fatal(err)
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

		Convey("It should raise the mismatch, dump a diagnostic, and mark the book invalid until a snapshot", func() {
			t.Chdir(t.TempDir())
			So(managed.Update(knownGood), ShouldBeNil)

			// A wrong server checksum for state that is known good.
			diverged := &kraken.Level3{Raw: []byte(`{"channel":"level3"}`), Data: []kraken.Level3Data{{
				Symbol:   "CELR/USD",
				Checksum: 1,
				Bids: []kraken.Level3Order{{
					Event: "add", OrderID: "late-bid",
					LimitPrice: lateBidPrice,
					OrderQty:   lateBidQuantity, Timestamp: time.Unix(2, 0).UTC(),
				}},
			}}}

			err := managed.Update(diverged)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "checksum_mismatch for CELR/USD")
			So(err.Error(), ShouldContainSubstring, "server 1")
			So(err.Error(), ShouldContainSubstring, DiagnosticDir)

			raw, readErr := os.ReadFile(filepath.Join(DiagnosticDir, fmt.Sprintf("%d-CELR_USD-1.json", data.Epoch.Load())))
			So(readErr, ShouldBeNil)

			var diagnostic BookDiagnostic
			So(json.Unmarshal(raw, &diagnostic), ShouldBeNil)
			So(diagnostic.ServerChecksum, ShouldEqual, "1")
			So(diagnostic.ChecksumInput, ShouldNotBeEmpty)
			So(string(diagnostic.Frame), ShouldEqual, `{"channel":"level3"}`)
			So(diagnostic.Bids[0].Orders, ShouldNotBeEmpty)

			// The invalid book is never served and the tick says why.
			managed.Book("CELR/USD", func(book *spotbook.Book) {
				t.Fatal("invalid book was exposed to a reader")
			})

			// Deltas for an invalid book are counted, not applied, and the
			// count is on the tick's book_error layer.
			So(managed.Update(diverged), ShouldBeNil)
			index, layer := managed.Layer("CELR/USD")
			So(index, ShouldEqual, "book_error/CELR/USD")
			So(string(layer), ShouldContainSubstring, `"kind":"checksum_mismatch"`)
			So(string(layer), ShouldContainSubstring, `"dropped_deltas":1`)

			// Only a fresh snapshot makes the book valid again.
			So(managed.Update(knownGood), ShouldBeNil)
			index, _ = managed.Layer("CELR/USD")
			So(index, ShouldEqual, "book/CELR/USD")
		})

		Convey("The same price in different decimal text is one level, keyed at the pair's precision", func() {
			add := kraken.NewLevel3([]byte(`{"channel":"level3","type":"update","data":[{"symbol":"CELR/USD","bids":[{"event":"add","order_id":"short","limit_price":0.564,"order_qty":1,"timestamp":"2026-08-13T11:41:05Z"},{"event":"add","order_id":"long","limit_price":"0.5640000","order_qty":"2.00000","timestamp":"2026-08-13T11:41:06Z"}],"asks":[]}]}`))
			So(add, ShouldNotBeNil)
			So(managed.Update(add), ShouldBeNil)

			managed.Book("CELR/USD", func(book *spotbook.Book) {
				level := book.Bids.Levels["0.5640000"]
				So(level, ShouldNotBeNil)
				So(level.Queue(), ShouldHaveLength, 2)
				So(book.Bids.Levels["0.564"], ShouldBeNil)
				So(level.Quantity.String(), ShouldEqual, "3.00000")
			})
		})

		Convey("A value finer than the pair's precision is an error, never rounded", func() {
			fine := kraken.NewLevel3([]byte(`{"channel":"level3","type":"update","data":[{"symbol":"CELR/USD","bids":[{"event":"add","order_id":"fine","limit_price":0.56400001,"order_qty":1,"timestamp":"2026-08-13T11:41:05Z"}],"asks":[]}]}`))
			err := managed.Update(fine)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "limit_price 0.56400001 has more than the pair's 7 decimals")
		})

		Convey("A stale book is hidden from readers until a fresh snapshot", func() {
			So(managed.Update(knownGood), ShouldBeNil)
			managed.Stale([]string{"CELR/USD"})
			managed.Book("CELR/USD", func(book *spotbook.Book) {
				t.Fatal("stale book was exposed to a reader")
			})

			So(managed.Update(knownGood), ShouldBeNil)
			read := false
			managed.Book("CELR/USD", func(book *spotbook.Book) { read = true })
			So(read, ShouldBeTrue)
		})
	})

	Convey("Given a venue frame whose intermediate state is crossed", t, func() {
		managed := newBookFixture(t, "APR/USD", 0, 0)
		if err := managed.Create("APR/USD", 10); err != nil {
			t.Fatal(err)
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
		So(managed.Update(initial), ShouldBeNil)
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

		err := managed.Update(update)

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
		if err := managed.Create("BTC/USD", 2); err != nil {
			t.Fatal(err)
		}
		order := func(event, id string, price, quantity int64) kraken.Level3Order {
			return kraken.Level3Order{Event: event, OrderID: id, LimitPrice: decimal.NewFromInt64(price), OrderQty: decimal.NewFromInt64(quantity), Timestamp: time.Unix(price, 0)}
		}
		frame := func(orders ...kraken.Level3Order) *kraken.Level3 {
			return &kraken.Level3{Type: "update", Data: []kraken.Level3Data{{Symbol: "BTC/USD", Bids: orders}}}
		}
		_, err, _ := managed.apply(frame(order("add", "best", 100, 1), order("add", "worst", 99, 2)))
		So(err, ShouldBeNil)

		Convey("Truncation occurs after the whole frame, so later deletion can still address the old worst level", func() {
			accepted, err, _ := managed.apply(frame(order("add", "new", 101, 3), order("delete", "worst", 99, 0)))
			So(err, ShouldBeNil)
			So(accepted, ShouldHaveLength, 1)
			managed.Book("BTC/USD", func(current *spotbook.Book) {
				So(current.Bids.Levels, ShouldHaveLength, 2)
				So(current.Bids.Low.Price.Float64(), ShouldEqual, 100)
			})
		})

		Convey("Every entry in a venue frame is applied in order", func() {
			payload := &kraken.Level3{Type: "update", Data: []kraken.Level3Data{
				{Symbol: "BTC/USD", Bids: []kraken.Level3Order{order("add", "first", 101, 3)}},
				{Symbol: "BTC/USD", Bids: []kraken.Level3Order{order("add", "second", 102, 4)}},
			}}
			accepted, err, _ := managed.apply(payload)
			So(err, ShouldBeNil)
			So(accepted, ShouldHaveLength, 2)
			managed.Book("BTC/USD", func(current *spotbook.Book) {
				So(current.BestBid().Price.Float64(), ShouldEqual, 102)
			})
		})

		Convey("An unknown deleted level is raised with a diagnostic and invalidates the book", func() {
			t.Chdir(t.TempDir())
			bad := frame(order("delete", "missing", 98, 0))
			accepted, err, _ := managed.apply(bad)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "references absent level 98")
			So(err.Error(), ShouldContainSubstring, DiagnosticDir)
			So(accepted, ShouldBeEmpty)
			index, layer := managed.Layer("BTC/USD")
			So(index, ShouldEqual, "book_error/BTC/USD")
			So(string(layer), ShouldContainSubstring, `"kind":"absent_level"`)
		})

		Convey("A modify to the minimum lot keeps the order resting", func() {
			managed := newBookFixture(t, "BTC/USD", 1, 5)
			snapshot := kraken.NewLevel3([]byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"BTC/USD","bids":[{"order_id":"lot","limit_price":100.5,"order_qty":2.00000,"timestamp":"2026-08-13T11:41:05Z"}],"asks":[]}]}`))
			So(snapshot, ShouldNotBeNil)
			_, err, _ := managed.apply(snapshot)
			So(err, ShouldBeNil)

			modify := kraken.NewLevel3([]byte(`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD","bids":[{"event":"modify","order_id":"lot","limit_price":100.5,"order_qty":1e-05,"timestamp":"2026-08-13T11:41:06Z"}],"asks":[]}]}`))
			So(modify, ShouldNotBeNil)
			_, err, _ = managed.apply(modify)
			So(err, ShouldBeNil)

			managed.Book("BTC/USD", func(current *spotbook.Book) {
				best := current.BestBid()
				So(best, ShouldNotBeNil)
				So(best.Price.String(), ShouldEqual, "100.5")
				So(best.Quantity.String(), ShouldEqual, "0.00001")
			})
		})

	})
}

func BenchmarkBookAll(b *testing.B) {
	managed := newBookFixture(b, "BTC/USD", 0, 0)
	if err := managed.Create("BTC/USD", 256); err != nil {
		b.Fatal(err)
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

		if err := managed.Update(payload); err != nil {
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
	if err := managed.Create("BTC/USD", 32); err != nil {
		b.Fatal(err)
	}
	read := func(*spotbook.Book) {}
	b.ReportAllocs()

	for b.Loop() {
		managed.Book("BTC/USD", read)
	}
}

func BenchmarkBookUpdate(b *testing.B) {
	managed := newBookFixture(b, "BTC/USD", 0, 0)
	if err := managed.Create("BTC/USD", 32); err != nil {
		b.Fatal(err)
	}
	price := decimal.NewFromInt64(100)
	quantity := decimal.NewFromInt64(1)
	at := time.Unix(1_700_008_000, 0).UTC()

	b.ReportAllocs()

	for b.Loop() {
		if err := managed.Update(&kraken.Level3{Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{{
				Event: "add", OrderID: "bid-one", LimitPrice: price,
				OrderQty: quantity, Timestamp: at,
			}},
		}}}); err != nil {
			b.Fatal(err)
		}
		if err := managed.Update(&kraken.Level3{Data: []kraken.Level3Data{{
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

func TestBookWait(t *testing.T) {
	Convey("Given a boot that requires two symbol snapshots", t, func() {
		managed := newBookFixture(t, "BTC/USD", 0, 0)
		managed.Expect([]string{"BTC/USD", "ETH/USD"})
		apply := func(kind, symbol string) {
			So(managed.Update(&kraken.Level3{Type: kind, Data: []kraken.Level3Data{{Symbol: symbol}}}), ShouldBeNil)
		}
		Convey("Creating a book and receiving a delta do not count as seeding", func() {
			if err := managed.Create("BTC/USD", 10); err != nil {
				t.Fatal(err)
			}
			apply("update", "BTC/USD")
			So(managed.Status(), ShouldEqual, runtime.BUSY)
		})
		Convey("All snapshots and their consumers must finish before READY", func() {
			observed := 0
			managed.SetNotify(func(symbol string, _ time.Time) {
				So(managed.Status(), ShouldEqual, runtime.BUSY)
				readable := false
				managed.Book(symbol, func(book *spotbook.Book) { readable = true })
				So(readable, ShouldBeTrue)
				observed++
			})
			apply("snapshot", "BTC/USD")
			So(managed.Status(), ShouldEqual, runtime.BUSY)
			apply("snapshot", "ETH/USD")
			So(observed, ShouldEqual, 2)
			So(managed.Wait(time.Second), ShouldBeNil)
			So(managed.Status(), ShouldEqual, runtime.READY)
		})
		Convey("Interrupted seeding is an error and never READY", func() {
			ctx, cancel := context.WithCancel(t.Context())
			managed.ctx = ctx
			cancel()
			So(managed.Wait(time.Second), ShouldNotBeNil)
			So(managed.Status(), ShouldEqual, runtime.BUSY)
		})
		Convey("A symbol that never seeds is an error naming it once the bound passes", func() {
			apply("snapshot", "BTC/USD")
			err := managed.Wait(10 * time.Millisecond)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "1 level3 books not seeded within 10ms: ETH/USD")
			So(managed.Pending(), ShouldResemble, []string{"ETH/USD"})
			So(managed.Status(), ShouldEqual, runtime.BUSY)
		})
	})
}

func TestAppendDecimalDigits(t *testing.T) {
	Convey("Kraken L3 CRC32 checksum decimal formatting", t, func() {
		Convey("Strips decimal points and trims leading zeros", func() {
			cases := []struct {
				val      string
				expected string
			}{
				{"0.00100000", "100000"},
				{"0.00316", "316"},
				{"0.1", "1"},
				{"1.0", "10"},
				{"100.50", "10050"},
				{"1000", "1000"},
				{"0.0", ""},
			}

			for _, c := range cases {
				dec, err := decimal.NewFromString(c.val)
				So(err, ShouldBeNil)
				res := string(appendDecimalDigits(nil, dec))
				So(res, ShouldEqual, c.expected)
			}
		})
	})
}

func BenchmarkAppendDecimalDigits(b *testing.B) {
	dec, _ := decimal.NewFromString("100.50")
	var buf [64]byte

	b.ReportAllocs()

	for b.Loop() {
		_ = appendDecimalDigits(buf[:0], dec)
	}
}
