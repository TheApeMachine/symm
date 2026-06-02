/*
Package activate prints boot and one-shot activation lines.

Once deduplicates by key using a sync.Map that never evicts entries. Keys must be
low-cardinality fixed strings (subsystem, channel, lifecycle phase). Do not embed
counts, IDs, symbols, or other unbounded or high-cardinality data in keys — that
leaks memory. For variable or per-entity keys use a separate bounded helper when
one exists, or add BoundedOnce / OnceWithTTL before using Once that way.
*/
package activate

import (
	"fmt"
	"sync"
)

var seen sync.Map

/*
Once prints a single activation line for key. Safe to call from hot paths.

key must be a low-cardinality label (no formatted counts, symbols, or IDs). The
seen map never evicts; unbounded keys leak memory.
*/
func Once(key string) {
	if _, loaded := seen.LoadOrStore(key, struct{}{}); loaded {
		return
	}

	fmt.Println("[symm] activate:", key)
}

/*
Boot prints a startup milestone (not deduplicated).
*/
func Boot(message string) {
	fmt.Println("[symm] boot:", message)
}
