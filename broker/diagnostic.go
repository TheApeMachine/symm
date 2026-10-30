package broker

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/kraken"
)

/*
DiagnosticDir is where every book-maintenance failure is dumped, one JSON file
per failure, so the failure can be replayed and the bug fixed.
*/
const DiagnosticDir = "diagnostics/checksum"

/*
BookDiagnostic is the full record of one book-maintenance failure: the frame
exactly as Kraken sent it, the entry being applied, and the local top ten
levels per side with every order's exact price/quantity strings and the
digits Kraken's level3 checksum hashes for it.
*/
type BookDiagnostic struct {
	Kind           string            `json:"kind"`
	Symbol         string            `json:"symbol"`
	Epoch          uint64            `json:"epoch"`
	Seq            int64             `json:"seq"`
	At             time.Time         `json:"at"`
	Detail         string            `json:"detail"`
	FrameType      string            `json:"frame_type"`
	ServerChecksum string            `json:"server_checksum,omitempty"`
	LocalChecksum  string            `json:"local_checksum"`
	ChecksumInput  string            `json:"checksum_input"`
	Frame          string            `json:"frame"`
	Entry          kraken.Level3Data `json:"entry"`
	Asks           []DiagnosticLevel `json:"asks"`
	Bids           []DiagnosticLevel `json:"bids"`
	Path           string            `json:"path"`
}

type DiagnosticLevel struct {
	Price  string            `json:"price"`
	Orders []DiagnosticOrder `json:"orders"`
}

type DiagnosticOrder struct {
	ID       string `json:"id"`
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
	Checksum string `json:"checksum"`
}

/*
invalidBook is a symbol whose local book failed maintenance. It stays invalid,
and its deltas are counted instead of applied, until a fresh snapshot.
*/
type invalidBook struct {
	diagnostic *BookDiagnostic
	dropped    atomic.Int64
}

/*
invalidate marks symbol's book invalid, dumps the full diagnostic to
DiagnosticDir, and returns the error to raise, naming the dump.
*/
func (book *Book) invalidate(
	kind, detail string, payload *kraken.Level3,
	entry kraken.Level3Data, serverChecksum string,
) error {
	diagnostic := &BookDiagnostic{
		Kind:           kind,
		Symbol:         entry.Symbol,
		Epoch:          data.Epoch.Load(),
		Seq:            book.failures.Add(1),
		At:             time.Now().UTC(),
		Detail:         detail,
		FrameType:      payload.Type,
		ServerChecksum: serverChecksum,
		Frame:          string(payload.Raw),
		Entry:          entry,
	}

	diagnostic.LocalChecksum = fmt.Sprint(crc32.ChecksumIEEE([]byte(diagnostic.ChecksumInput)))
	diagnostic.Path = filepath.Join(DiagnosticDir, fmt.Sprintf(
		"%d-%s-%d.json", diagnostic.Epoch, strings.ReplaceAll(entry.Symbol, "/", "_"), diagnostic.Seq,
	))

	book.invalid.Store(entry.Symbol, &invalidBook{diagnostic: diagnostic})
	book.requestLevel3Resubscribe(entry.Symbol)

	message := fmt.Sprintf(
		"level3 %s for %s: %s (local checksum %s, server %s, frame %s); book invalid until a fresh snapshot; diagnostic %s",
		kind, entry.Symbol, detail, diagnostic.LocalChecksum, serverChecksum, payload.Type, diagnostic.Path,
	)

	if err := writeDiagnostic(diagnostic); err != nil {
		return errnie.Err(errnie.IO, message+" (diagnostic write failed)", err)
	}

	return errnie.Err(errnie.Validation, message, nil)
}

func writeDiagnostic(diagnostic *BookDiagnostic) error {
	raw, err := json.MarshalIndent(diagnostic, "", "  ")

	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(diagnostic.Path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(diagnostic.Path, raw, 0o644)
}

/*
Layer returns the layer that represents symbol's book on a tick: "book/<sym>"
with the book itself when it is valid, otherwise "book_error/<sym>" saying
why it is not (invalid, stale, or never seeded by a snapshot). A tick never
silently lacks a book.
*/
func (book *Book) Layer(symbol string) (string, []byte) {
	if raw := book.JSON(symbol); raw != nil {
		return "book/" + symbol, raw
	}

	state := map[string]any{"symbol": symbol, "kind": "unseeded"}

	if _, stale := book.stale.Load(symbol); stale {
		state["kind"] = "stale"
	}

	if value, invalid := book.invalid.Load(symbol); invalid {
		entry := value.(*invalidBook)
		state["kind"] = entry.diagnostic.Kind
		state["detail"] = entry.diagnostic.Detail
		state["server_checksum"] = entry.diagnostic.ServerChecksum
		state["local_checksum"] = entry.diagnostic.LocalChecksum
		state["seq"] = entry.diagnostic.Seq
		state["diagnostic"] = entry.diagnostic.Path
		state["dropped_deltas"] = entry.dropped.Load()
	}

	raw, err := sonic.Marshal(state)

	if err != nil {
		errnie.Error(errnie.Err(errnie.UnprocessableContent, "[book] "+symbol+" error layer marshal failed", err))
		return "book_error/" + symbol, nil
	}

	return "book_error/" + symbol, raw
}
