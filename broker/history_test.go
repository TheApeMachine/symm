package broker

import (
	"context"
	"io"
	"testing"
)

func TestHistoryColdReadEnds(t *testing.T) {
	history := NewHistory(context.Background(), nil)

	n, err := history.Read(make([]byte, 64))

	if n != 0 || err != io.EOF {
		t.Fatalf("cold read n=%d err=%v", n, err)
	}
}
