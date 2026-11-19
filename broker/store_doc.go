package broker

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
)

/*
positionStoreDoc is the S3 / DuckDB / hub closed-history envelope. Cache must
write this shape — bare *Position JSON leaves hub AttributeOk("position") and
workbench read_json(columns={…,'position':'JSON'}) empty.
*/
type positionStoreDoc struct {
	Epoch     uint64         `json:"epoch"`
	Tick      uint64         `json:"tick"`
	Timestamp uint64         `json:"timestamp"`
	Position  map[string]any `json:"position"`
}

/*
encodePositionDoc wraps a live Position for object storage. Readers expect
position.book (ledger stem); Position still carries tier_key on the wire.
*/
func encodePositionDoc(position *Position, tierKey string) ([]byte, error) {
	if position == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[broker.encodePositionDoc] position is nil",
			nil,
		))
	}

	raw, err := sonic.Marshal(position)
	if err != nil {
		return nil, errnie.Error(err)
	}

	var row map[string]any
	if err := sonic.Unmarshal(raw, &row); err != nil {
		return nil, errnie.Error(err)
	}

	book := tierKey
	if tk, ok := row["tier_key"].(string); ok && tk != "" {
		book = tk
	} else if position.TierKey != "" {
		book = position.TierKey
	}

	if book != "" {
		row["book"] = book
		if _, ok := row["tier_key"]; !ok {
			row["tier_key"] = book
		}
	}

	doc := positionStoreDoc{
		Epoch:     data.Epoch.Load(),
		Tick:      data.Tick.Load(),
		Timestamp: uint64(time.Now().UTC().UnixNano()),
		Position:  row,
	}

	body, err := sonic.Marshal(doc)
	if err != nil {
		return nil, errnie.Error(err)
	}

	return body, nil
}
