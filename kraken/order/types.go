package order

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
)

const (
	MethodAddOrder   = "add_order"
	MethodAmendOrder = "amend_order"
)

// clOrdIDMaxLen is Kraken's cap on cl_ord_id free-text: 18 ASCII chars
// (see docs.kraken.com/api/docs/websocket-v2/add_order, "Client Order Id"
// field). The generator below produces ids ≤ 18 chars without relying on
// post-truncation, so the random suffix entropy is preserved.
const clOrdIDMaxLen = 18

// clOrdRandHexChars is the width of the random hex suffix. 8 hex chars =
// 32 bits = ~1 in 4 billion collisions for a single process restart on
// the same account, which is more than enough alongside the per-process
// sequence number.
const clOrdRandHexChars = 8

// clOrdSeqWidth is the maximum length of the base62 sequence segment.
// With base62 a uint64 fits in at most 11 chars, but we cap the segment
// at 8 chars (62^8 ≈ 2.2e14 unique seqs per process) and reset at the
// cap. This keeps the total id width at 1 ("s") + 8 (seq) + 1 ("-") + 8
// (rand) = 18 chars, never tripping clOrdIDMaxLen truncation.
const clOrdSeqWidth = 8

var clOrdSeq atomic.Uint64

/*
NextClOrdID returns a process-unique client order id within Kraken's
18-character cap. Format: "s<seqBase62>-<randHex>" where seqBase62 is
the process-monotonic counter encoded in base-62 (uppercase + lowercase +
digits) capped at clOrdSeqWidth chars, and randHex is clOrdRandHexChars
of crypto/rand entropy so the same sequence number colliding across two
process restarts within the same account is still distinguishable.

The function returns an error when the system entropy source fails;
callers must treat that as a hard failure rather than fall back to a
predictable id, since cl_ord_id is the dedupe key the wallet uses to
reject reconnect-replays.
*/
func NextClOrdID() (string, error) {
	seq := clOrdSeq.Add(1)
	seqStr := encodeBase62(seq, clOrdSeqWidth)

	var randBytes [clOrdRandHexChars / 2]byte

	if _, err := rand.Read(randBytes[:]); err != nil {
		return "", fmt.Errorf("cl_ord_id entropy: %w", err)
	}

	return "s" + seqStr + "-" + hex.EncodeToString(randBytes[:]), nil
}

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func encodeBase62(value uint64, maxWidth int) string {
	if value == 0 {
		return "0"
	}

	buf := make([]byte, 0, maxWidth)

	for value > 0 && len(buf) < maxWidth {
		buf = append(buf, base62Alphabet[value%62])
		value /= 62
	}

	// Reverse in place.
	for left, right := 0, len(buf)-1; left < right; left, right = left+1, right-1 {
		buf[left], buf[right] = buf[right], buf[left]
	}

	return string(buf)
}

/*
OrderType is the Kraken WebSocket v2 order execution model.
*/
type OrderType string

const (
	Limit             OrderType = "limit"
	Market            OrderType = "market"
	Iceberg           OrderType = "iceberg"
	StopLoss          OrderType = "stop-loss"
	StopLossLimit     OrderType = "stop-loss-limit"
	TakeProfit        OrderType = "take-profit"
	TakeProfitLimit   OrderType = "take-profit-limit"
	TrailingStop      OrderType = "trailing-stop"
	TrailingStopLimit OrderType = "trailing-stop-limit"
)

/*
Side is the order book side.
*/
type Side string

const (
	Buy  Side = "buy"
	Sell Side = "sell"
)
