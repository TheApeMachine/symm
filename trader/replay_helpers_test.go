package trader

import (
	"fmt"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

var replaySequence = time.Now().UnixNano()

func krakenTickerReplayFrame(
	volume, vwap, last, bid, ask, changePct float64,
	symbol string,
) []byte {
	return fmt.Appendf(nil,
		`{"channel":"ticker","type":"update","data":[{"symbol":%q,"bid":%g,"bid_qty":740.0,"ask":%g,"ask_qty":740.0,"last":%g,"volume":%g,"vwap":%g,"change_pct":%g}]}`,
		symbol, bid, ask, last, volume, vwap, changePct,
	)
}

func insertTickerReplay(
	tree *dmt.Tree,
	symbol string,
	tickCount int,
	volumeStep, vwap, last, bid, ask, changePct float64,
) {
	for tick := range tickCount {
		volume := volumeStep * float64(tick+1)
		stored := datura.Acquire("kraken:public", datura.APPJSON)
		stored.WithRole("ticker")
		stored.WithScope("update")
		stored.WithPayload(krakenTickerReplayFrame(volume, vwap, last, bid, ask, changePct, symbol))
		replaySequence++
		stored.SetTimestamp(replaySequence)
		tree.Insert(stored.Prefix(), stored.Pack())
		stored.Release()
	}
}

func ingestProgressiveTicker(
	tree *dmt.Tree,
	tickCount int,
	volumeStep float64,
	last float64,
	replayAt *int64,
) {
	_ = replayAt

	insertTickerReplay(tree, "REPLAY/USD", tickCount, volumeStep, 10000, last, 9990, 10010, 0)
}

func ingestVerticalTicker(tree *dmt.Tree, replayAt *int64) {
	_ = replayAt

	insertTickerReplay(tree, "REPLAY/USD", 1, 11000, 10000, 41000, 40990, 41010, 3.1)
}
