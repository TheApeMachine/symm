package broker

import (
	"context"
	"fmt"
	"iter"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
)

func newTestCache(tierKey string) *Cache {
	return &Cache{
		System:  runtime.NewSystem(context.Background(), "cache"),
		tierKey: tierKey,
		open:    &sync.Map{},
		closed:  &sync.Map{},
		archive: &sync.Map{},
	}
}

func TestCacheRestore(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("wrapped lotBlob unmarshals into the same ledger as bare JSON", func() {
			cache := newTestCache("virtual")
			inner := replayFills("WRP/USD", []market.ExecutionData{
				tradeFill("b1", "buy", "2", "10"),
			})
			inner.ID = "lot-wrap"

			wrapped, err := sonic.Marshal(&lotBlob{
				Epoch:    1,
				Tick:     2,
				Position: inner,
			})
			So(err, ShouldBeNil)

			got := cache.Restore(wrapped)
			So(got, ShouldNotBeNil)
			So(got.ID, ShouldEqual, "lot-wrap")

			lq, le, _, _ := foldSnapshot(inner)
			gq, ge, _, _ := foldSnapshot(got)
			So(gq, ShouldEqual, lq)
			So(ge, ShouldEqual, le)
		})

		Convey("corrupt bytes return nil", func() {
			cache := newTestCache("virtual")
			So(cache.Restore([]byte(`{not json`)), ShouldBeNil)
			So(cache.Restore(nil), ShouldBeNil)
		})

		Convey("legacy fills without qty replay through Apply to the same ledger", func() {
			cache := newTestCache("virtual")
			seq := []market.ExecutionData{
				tradeFill("b1", "buy", "10", "100"),
				tradeFill("s1", "sell", "4", "105"),
			}
			live := replayFills("ETH/USD", seq)

			raw, err := sonic.Marshal(&Position{
				ID:       "pos-replay",
				Symbol:   "ETH/USD",
				OpenedAt: time.Now().UTC(),
				Fills:    live.Fills,
			})
			So(err, ShouldBeNil)

			got := cache.Restore(raw)
			So(got, ShouldNotBeNil)

			lq, le, ls, lv := foldSnapshot(live)
			gq, ge, gs, gv := foldSnapshot(got)

			So(gq, ShouldEqual, lq)
			So(ge, ShouldEqual, le)
			So(gs, ShouldEqual, ls)
			So(gv, ShouldEqual, lv)
		})

		Convey("fill replay during Restore does not invoke onSettled", func() {
			called := false
			cache := &Cache{
				System:    runtime.NewSystem(context.Background(), "cache"),
				tierKey:   "virtual",
				onSettled: func(*Position) { called = true },
			}

			raw := []byte(`{
				"id":"pos-replay",
				"symbol":"BTC/USD",
				"fills":[
					{"exec_id":"b1","side":"buy","last_qty":"1","last_price":"10","exec_type":"trade"},
					{"exec_id":"s1","side":"sell","last_qty":"1","last_price":"10","exec_type":"trade"}
				]
			}`)

			got := cache.Restore(raw)

			So(got, ShouldNotBeNil)
			So(got.Flat(), ShouldBeTrue)
			So(called, ShouldBeFalse)
		})

		Convey("flat orphan on open prefix keeps first exit vwap and drops later sells", func() {
			cache := newTestCache("virtual")
			raw := []byte(`{
				"id":"pos-flat",
				"symbol":"INIT/USD",
				"closed_at":"2026-11-05T13:19:50Z",
				"fills":[
					{"exec_id":"b1","side":"buy","last_qty":"10","last_price":"1","exec_type":"trade"},
					{"exec_id":"s1","side":"sell","last_qty":"10","last_price":"1.1","exec_type":"trade"},
					{"exec_id":"s2","side":"sell","last_qty":"10","last_price":"0.9","exec_type":"trade"}
				]
			}`)

			got := cache.Restore(raw)

			So(got.Flat(), ShouldBeTrue)
			So(got.ExitVWAP.String(), ShouldEqual, "1.1")
			So(len(got.Fills), ShouldEqual, 2)
		})
	})
}

func TestCacheAdoptOpenLot(t *testing.T) {
	Convey("Setup", t, func() {
		cases := []struct {
			name         string
			raw          []byte
			stopOnWire   bool
			wantArmPct   float64
			wantStopLoss StoplossState
		}{
			{
				name:         "missing stoploss and pnl_pct seeds zero",
				raw:          []byte(`{"id":"pos-3","symbol":"ADA/USD","entry_order":{"cl_ord_id":"x","type":"buy","volume":"1"}}`),
				wantArmPct:   0,
				wantStopLoss: StoplossHealthy,
			},
			{
				name: "missing stoploss uses wire pnl_pct",
				raw: []byte(`{
					"id":"pos-open",
					"symbol":"ETH/USD",
					"pnl_pct":"2.5",
					"fills":[{"exec_id":"b1","side":"buy","last_qty":"1","last_price":"10","exec_type":"trade"}]
				}`),
				wantArmPct:   2.5,
				wantStopLoss: StoplossHealthy,
			},
			{
				name:       "existing stoploss on wire is not replaced",
				stopOnWire: true,
				raw: []byte(`{
					"id":"pos-armed",
					"symbol":"SOL/USD",
					"stoploss":{"State":1,"InitialPnLPct":2.5,"CurrentPnLPct":2.5,"PeakPnLPct":2.5,"FloorPnLPct":2.5,"ArmPnlPct":1,"GivebackPnlPct":0.5,"FloorMinPnlPct":0.5,"TightenPerPct":0.05},
					"fills":[{"exec_id":"b1","side":"buy","last_qty":"1","last_price":"1","exec_type":"trade"}]
				}`),
				wantArmPct:   2.5,
				wantStopLoss: StoplossArmed,
			},
		}

		for _, tc := range cases {
			Convey("When "+tc.name, func() {
				cache := newTestCache("virtual")
				got := cache.Restore(tc.raw)
				So(got, ShouldNotBeNil)

				if tc.stopOnWire {
					So(got.Stoploss, ShouldNotBeNil)
				} else {
					So(got.Stoploss, ShouldBeNil)
				}

				cache.adoptOpenLot(got)

				So(got.Stoploss, ShouldNotBeNil)
				So(got.Stoploss.InitialPnLPct, ShouldEqual, tc.wantArmPct)
				So(got.Stoploss.State, ShouldEqual, tc.wantStopLoss)
				So(cache.Has(got.Symbol), ShouldBeTrue)
				So(got.ClosedAt, ShouldBeNil)
			})
		}
	})
}

func TestCacheLanes(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("Submitted includes awaiting fill but Open lists filled lots only", func() {
			cache := newTestCache("virtual")

			awaiting := &Position{
				ID:         "pending",
				Symbol:     "AAA/USD",
				EntryOrder: &spot.OrderRequest{ClOrdId: "cl-1", Type: "buy", Volume: "1"},
			}
			filled := NewPosition(context.Background(), "BBB/USD", "virtual", "path", nil, nil, nil)
			filled.Apply(tradeFill("b1", "buy", "1", "10"))

			cache.open.Store("AAA/USD", awaiting)
			cache.open.Store("BBB/USD", filled)

			So(len(cache.Submitted()), ShouldEqual, 2)
			So(len(cache.Open()), ShouldEqual, 1)
			So(cache.Open("BBB/USD")[0].Qty.String(), ShouldEqual, "1")
		})

		Convey("Has stays true while ClosedAt is set until Remove", func() {
			now := time.Now().UTC()
			cache := newTestCache("virtual")
			cache.open.Store("XRP/USD", &Position{Symbol: "XRP/USD", ClosedAt: &now})

			So(cache.Has("XRP/USD"), ShouldBeTrue)
			_, inClosed := cache.closed.Load("XRP/USD")
			So(inClosed, ShouldBeFalse)
		})

		Convey("RememberClosed keeps the latest ClosedAt per symbol", func() {
			cache := newTestCache("virtual")
			older := time.Now().UTC().Add(-time.Hour)
			newer := time.Now().UTC()

			cache.RememberClosed(&Position{ID: "old", Symbol: "SOL/USD", ClosedAt: &older})
			cache.RememberClosed(&Position{ID: "new", Symbol: "SOL/USD", ClosedAt: &newer})

			raw, _ := cache.closed.Load("SOL/USD")
			So(raw.(*Position).ID, ShouldEqual, "new")
		})

		Convey("Add refuses a second open lot for the same symbol", func() {
			cache := newTestCache("virtual")
			first := NewPosition(context.Background(), "ONE/USD", "virtual", "path", nil, nil, nil)
			second := NewPosition(context.Background(), "ONE/USD", "virtual", "path", nil, nil, nil)

			So(cache.Add("ONE/USD", first, true), ShouldBeFalse)
			So(cache.Add("ONE/USD", second, true), ShouldBeTrue)

			slots := cache.Submitted("ONE/USD")
			So(slots, ShouldHaveLength, 1)
			So(slots[0].ID, ShouldEqual, first.ID)
		})

		Convey("Remove moves a flat lot from open to closed lane", func() {
			cache := newTestCache("virtual")
			now := time.Now().UTC()
			pos := &Position{ID: "done", Symbol: "DOT/USD", ClosedAt: &now}

			cache.open.Store("DOT/USD", pos)
			So(cache.Remove("DOT/USD"), ShouldBeTrue)
			So(cache.Has("DOT/USD"), ShouldBeFalse)

			stored, ok := cache.closed.Load("DOT/USD")
			So(ok, ShouldBeTrue)
			So(stored.(*Position).ID, ShouldEqual, "done")
		})
	})
}

func TestEncodeLot(t *testing.T) {
	Convey("encodeLot stamps exit tick and closed timestamp", t, func() {
		closed := time.Now().UTC()
		pos := &Position{
			ID: "lot-1", Symbol: "ENC/USD",
			PolicyEpoch: 7, EntryTick: 10, ExitTick: 20,
			ClosedAt: &closed,
		}

		raw, err := encodeLot(pos)
		So(err, ShouldBeNil)

		var blob lotBlob
		So(sonic.Unmarshal(raw, &blob), ShouldBeNil)
		So(blob.Epoch, ShouldEqual, 7)
		So(blob.Tick, ShouldEqual, 20)
		So(blob.Timestamp, ShouldEqual, uint64(closed.UnixNano()))
		So(blob.Position.ID, ShouldEqual, "lot-1")
	})
}

func TestCacheArchiveAndAll(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("All dedupes open and closed by id", func() {
			cache := newTestCache("virtual")
			open := NewPosition(context.Background(), "A/USD", "virtual", "path", nil, nil, nil)
			open.ID = "same"
			open.Apply(tradeFill("b1", "buy", "1", "1"))

			closed := NewPosition(context.Background(), "B/USD", "virtual", "path", nil, nil, nil)
			closed.ID = "other"
			now := time.Now().UTC()
			closed.ClosedAt = &now

			cache.open.Store("A/USD", open)
			cache.closed.Store("B/USD", closed)

			all := cache.All()
			So(len(all), ShouldEqual, 2)
		})

		Convey("Archived returns virtual round trips by id", func() {
			cache := newTestCache("virtual")
			now := time.Now().UTC()
			pos := &Position{ID: "arc-1", Symbol: "ARC/USD", ClosedAt: &now}

			cache.RememberClosed(pos)

			So(cache.Archived("arc-1"), ShouldEqual, pos)
			So(cache.Archived("missing"), ShouldBeNil)
		})

		Convey("Drop evicts the open lane", func() {
			cache := newTestCache("virtual")
			cache.open.Store("DROP/USD", &Position{Symbol: "DROP/USD"})
			cache.Drop("DROP/USD")
			So(cache.Has("DROP/USD"), ShouldBeFalse)
		})

		Convey("SetOpen replaces the live key", func() {
			cache := newTestCache("virtual")
			first := &Position{ID: "a", Symbol: "Z/USD"}
			second := &Position{ID: "b", Symbol: "Z/USD"}

			cache.SetOpen("Z/USD", first)
			cache.SetOpen("Z/USD", second)

			raw, _ := cache.open.Load("Z/USD")
			So(raw.(*Position).ID, ShouldEqual, "b")
		})
	})
}

type memCacheStore struct {
	mu        sync.Mutex
	blob      map[string][]byte
	writeErr  error
	deleteErr error
}

func (store *memCacheStore) WriteAtPrefix(key string, value []byte) (int, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	if store.writeErr != nil {
		return 0, store.writeErr
	}

	if store.blob == nil {
		store.blob = make(map[string][]byte)
	}

	store.blob[key] = append([]byte(nil), value...)

	return len(value), nil
}

func (store *memCacheStore) DeletePrefix(prefix string) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	if store.deleteErr != nil {
		return store.deleteErr
	}

	for key := range store.blob {
		if strings.HasPrefix(key, prefix) {
			delete(store.blob, key)
		}
	}

	return nil
}

func (store *memCacheStore) Stream(ctx context.Context, prefix string) iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		store.mu.Lock()
		keys := make([]string, 0, len(store.blob))

		for key := range store.blob {
			if strings.HasPrefix(key, prefix) {
				keys = append(keys, key)
			}
		}

		store.mu.Unlock()

		for _, key := range keys {
			store.mu.Lock()
			buf := append([]byte(nil), store.blob[key]...)
			store.mu.Unlock()

			if !yield(buf) {
				return
			}
		}
	}
}

func TestCacheSaveWriteLot(t *testing.T) {
	Convey("Save routes blobs to open or closed prefixes", t, func() {
		backing := &memCacheStore{}
		cache := &Cache{
			System:  runtime.NewSystem(context.Background(), "cache"),
			tierKey: "virtual",
			store:   backing,
			open:    &sync.Map{},
			closed:  &sync.Map{},
			archive: &sync.Map{},
		}

		pos := NewPosition(context.Background(), "SAV/USD", "virtual", "path", nil, nil, nil)
		pos.ID = "lot-save"
		cache.open.Store("SAV/USD", pos)

		So(cache.Save(pos), ShouldBeNil)

		openKey := "virtual/open/SAV/USD/lot-save"
		_, hasOpen := backing.blob[openKey]
		So(hasOpen, ShouldBeTrue)

		closed := time.Now().UTC()
		pos.ClosedAt = &closed
		cache.closed.Store("SAV/USD", pos)

		So(cache.Save(pos), ShouldBeNil)

		closedKey := "virtual/closed/SAV/USD/lot-save"
		_, hasClosed := backing.blob[closedKey]
		So(hasClosed, ShouldBeTrue)
		_, stillOpen := backing.blob[openKey]
		So(stillOpen, ShouldBeFalse)
	})

	Convey("SaveAsync eventually persists", t, func() {
		backing := &memCacheStore{}
		cache := &Cache{
			System:  runtime.NewSystem(context.Background(), "cache"),
			tierKey: "virtual",
			store:   backing,
			open:    &sync.Map{},
			closed:  &sync.Map{},
			archive: &sync.Map{},
		}

		pos := &Position{ID: "async-1", Symbol: "ASY/USD"}
		cache.open.Store("ASY/USD", pos)

		cache.SaveAsync(pos)

		deadline := time.Now().Add(2 * time.Second)
		var key string

		for time.Now().Before(deadline) {
			backing.mu.Lock()
			key = fmt.Sprintf("virtual/open/ASY/USD/%s", pos.ID)
			_, ok := backing.blob[key]
			backing.mu.Unlock()

			if ok {
				break
			}

			goruntime.Gosched()
		}

		backing.mu.Lock()
		_, ok := backing.blob[key]
		backing.mu.Unlock()

		So(ok, ShouldBeTrue)
	})
}

func TestCacheMoveDeleteFailure(t *testing.T) {
	Convey("move surfaces DeletePrefix failures after closing write", t, func() {
		backing := &memCacheStore{
			blob: map[string][]byte{
				"virtual/open/MOV/USD/lot-1": []byte("open"),
			},
			deleteErr: fmt.Errorf("delete failed"),
		}

		cache := &Cache{
			System:  runtime.NewSystem(context.Background(), "cache"),
			tierKey: "virtual",
			store:   backing,
			open:    &sync.Map{},
			closed:  &sync.Map{},
			archive: &sync.Map{},
		}

		pos := &Position{ID: "lot-1", Symbol: "MOV/USD"}

		err := cache.move(pos)
		So(err, ShouldNotBeNil)

		closedKey := "virtual/closed/MOV/USD/lot-1"
		_, hasClosed := backing.blob[closedKey]
		So(hasClosed, ShouldBeTrue)
		_, stillOpen := backing.blob["virtual/open/MOV/USD/lot-1"]
		So(stillOpen, ShouldBeTrue)
	})
}

func TestCacheDropDeleteFailure(t *testing.T) {
	Convey("Drop evicts memory even when store delete fails", t, func() {
		openKey := "virtual/open/DROP/USD/lot-x"
		backing := &memCacheStore{
			blob: map[string][]byte{
				openKey: []byte("blob"),
			},
			deleteErr: fmt.Errorf("delete failed"),
		}

		cache := &Cache{
			System:  runtime.NewSystem(context.Background(), "cache"),
			tierKey: "virtual",
			store:   backing,
			open:    &sync.Map{},
			closed:  &sync.Map{},
			archive: &sync.Map{},
		}

		pos := &Position{ID: "lot-x", Symbol: "DROP/USD"}
		cache.open.Store("DROP/USD", pos)

		cache.Drop("DROP/USD")

		_, inMem := cache.open.Load("DROP/USD")
		So(inMem, ShouldBeFalse)
		_, blobLeft := backing.blob[openKey]
		So(blobLeft, ShouldBeTrue)
	})
}

func TestCacheSaveWriteFailure(t *testing.T) {
	Convey("Save surfaces store write failures", t, func() {
		backing := &memCacheStore{writeErr: fmt.Errorf("disk full")}
		cache := &Cache{
			System:  runtime.NewSystem(context.Background(), "cache"),
			tierKey: "virtual",
			store:   backing,
			open:    &sync.Map{},
			closed:  &sync.Map{},
			archive: &sync.Map{},
		}

		pos := NewPosition(context.Background(), "ERR/USD", "virtual", "path", nil, nil, nil)
		pos.ID = "write-fail"
		cache.open.Store("ERR/USD", pos)

		err := cache.Save(pos)
		So(err, ShouldNotBeNil)
		_, saved := backing.blob["virtual/open/ERR/USD/write-fail"]
		So(saved, ShouldBeFalse)
	})
}

func TestCacheBootstrapFromStore(t *testing.T) {
	Convey("bootstrapFromStore adopts open lots and relocates flat orphans", t, func() {
		backing := &memCacheStore{}

		open := NewPosition(context.Background(), "OPN/USD", "virtual", "path", nil, nil, nil)
		open.ID = "open-1"
		open.Apply(tradeFill("b1", "buy", "1", "10"))
		openBody, err := encodeLot(open)
		So(err, ShouldBeNil)
		_, err = backing.WriteAtPrefix("virtual/open/OPN/USD/open-1", openBody)
		So(err, ShouldBeNil)

		pnl := decimal.RequireFromString("1")
		pct := decimal.RequireFromString("2")
		closed := time.Now().UTC()
		zero := decimal.Zero
		sold := decimal.RequireFromString("1")
		flat := &Position{
			ID: "flat-1", Symbol: "FLT/USD",
			Qty: &zero, Sold: &sold,
			ClosedAt: &closed, PnL: &pnl, PnLPct: &pct,
		}
		flatBody, err := encodeLot(flat)
		So(err, ShouldBeNil)
		_, err = backing.WriteAtPrefix("virtual/open/FLT/USD/flat-1", flatBody)
		So(err, ShouldBeNil)

		cache := &Cache{
			System:  runtime.NewSystem(context.Background(), "cache"),
			tierKey: "virtual",
			store:   backing,
			open:    &sync.Map{},
			closed:  &sync.Map{},
			archive: &sync.Map{},
		}

		cache.bootstrapFromStore()

		So(cache.Has("OPN/USD"), ShouldBeTrue)
		_, flatClosed := cache.closed.Load("FLT/USD")
		So(flatClosed, ShouldBeTrue)
		_, closedKey := backing.blob["virtual/closed/FLT/USD/flat-1"]
		So(closedKey, ShouldBeTrue)
	})
}

func TestCacheRemovePersistsMove(t *testing.T) {
	Convey("Remove with a backing store writes the closed prefix", t, func() {
		backing := &memCacheStore{}
		cache := &Cache{
			System:  runtime.NewSystem(context.Background(), "cache"),
			tierKey: "virtual",
			store:   backing,
			open:    &sync.Map{},
			closed:  &sync.Map{},
			archive: &sync.Map{},
		}

		closedAt := time.Now().UTC()
		pos := NewPosition(context.Background(), "MOV/USD", "virtual", "path", nil, nil, nil)
		pos.ID = "move-1"
		pos.ClosedAt = &closedAt
		pnl := decimal.RequireFromString("5")
		pos.PnL = &pnl

		cache.open.Store("MOV/USD", pos)
		So(cache.Remove("MOV/USD"), ShouldBeTrue)
		_, ok := backing.blob["virtual/closed/MOV/USD/move-1"]
		So(ok, ShouldBeTrue)
	})
}
