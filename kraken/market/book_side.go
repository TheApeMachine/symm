package market

import (
	"github.com/google/btree"
)

const bookSideDegree = 32

/*
bookSide stores one L2 side in a btree keyed by price.
*/
type bookSide struct {
	tree      *btree.BTreeG[BookLevel]
	ascending bool
}

func newAskSide() *bookSide {
	return &bookSide{
		tree:      btree.NewG(bookSideDegree, lessAskLevel),
		ascending: true,
	}
}

func newBidSide() *bookSide {
	return &bookSide{
		tree:      btree.NewG(bookSideDegree, lessBidLevel),
		ascending: false,
	}
}

func lessAskLevel(left, right BookLevel) bool {
	return left.Price < right.Price
}

func lessBidLevel(left, right BookLevel) bool {
	return left.Price > right.Price
}

func (side *bookSide) reset(levels []BookLevel) {
	side.tree.Clear(false)

	for _, level := range levels {
		if level.Qty <= 0 {
			continue
		}

		side.tree.ReplaceOrInsert(level)
	}
}

func (side *bookSide) apply(change BookLevel) {
	if change.Qty <= 0 {
		side.tree.Delete(BookLevel{Price: change.Price})

		return
	}

	side.tree.ReplaceOrInsert(change)
}

func (side *bookSide) levels(depth int) []BookLevel {
	if side.tree.Len() == 0 {
		return nil
	}

	out := make([]BookLevel, 0, side.tree.Len())

	side.tree.Ascend(func(level BookLevel) bool {
		out = append(out, level)

		if depth > 0 && len(out) >= depth {
			return false
		}

		return true
	})

	return out
}

func (side *bookSide) pruneBeyond(depth int) {
	if depth <= 0 {
		return
	}

	remove := make([]BookLevel, 0)
	seen := 0

	side.tree.Ascend(func(level BookLevel) bool {
		seen++

		if seen > depth {
			remove = append(remove, level)
		}

		return true
	})

	for _, level := range remove {
		side.tree.Delete(level)
	}
}
