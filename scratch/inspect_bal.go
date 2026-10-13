package main

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/apache/iceberg-go"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/system"
)

func main() {
	system.NewConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	catalog := tables.Open(ctx)
	epoch := int64(1791928500389570000)
	filter := iceberg.NewAnd(
		iceberg.EqualTo(iceberg.Reference("source"), "spot:level3"),
		iceberg.EqualTo(iceberg.Reference("label"), "BAL/USD"),
	)
	var list []*data.Measurement
	for m, scanErr := range catalog.Scan(ctx, tables.Measurements, epoch, filter, 0) {
		if scanErr != nil {
			break
		}
		list = append(list, m)
	}
	slices.SortFunc(list, func(a, b *data.Measurement) int {
		return cmp.Compare(a.SeqIdx, b.SeqIdx)
	})
	fmt.Printf("Total BAL/USD level3 measurements: %d\n", len(list))
	for i, m := range list {
		cs := data.Pull(m.Meta("checksum"))
		if cs != nil && cs.Value == "3457586274" {
			start := max(0, i-5)
			end := min(len(list), i+15)
			fmt.Printf("Found 3457586274 at index %d! Showing %d to %d:\n", i, start, end)
			for j := start; j < end; j++ {
				curr := list[j]
				c := data.Pull(curr.Meta("checksum"))
				t := data.Pull(curr.Meta("type"))
				e := data.Pull(curr.Meta("event"))
				s := data.Pull(curr.Meta("side"))
				id := data.Pull(curr.Meta("order_id"))
				fseq := data.Pull(curr.Meta("frame_seq"))
				fmt.Printf("  [%d] seq=%d tick=%d at=%v cs=%v tp=%v ev=%v sd=%v fseq=%v id=%v\n",
					j, curr.SeqIdx, curr.Tick, curr.At.Format(time.RFC3339Nano), c, t, e, s, fseq, id)
			}
			break
		}
	}
}
