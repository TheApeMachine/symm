package broker

import (
	"context"
	"testing"
)

func TestHistoryRestoreWithoutStoreEnds(t *testing.T) {
	history := NewHistory(context.Background(), nil)

	for position, err := range history.Restore() {
		if err != nil {
			t.Fatalf("restore: %v", err)
		}

		if position != nil {
			t.Fatalf("storeless restore yielded %v", position)
		}
	}
}

func TestHistoryStorelessWritesAreNoops(t *testing.T) {
	history := NewHistory(context.Background(), nil)

	if err := history.Record(NewPosition()); err != nil {
		t.Fatalf("record: %v", err)
	}

	if err := history.Forget("BTC/USD"); err != nil {
		t.Fatalf("forget: %v", err)
	}

	if err := history.Drop(); err != nil {
		t.Fatalf("drop: %v", err)
	}
}
