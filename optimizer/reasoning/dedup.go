package reasoning

import (
	"bytes"

	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

/*
forestDedup tracks forests already scored without allocating a string key per lookup.
Buckets keyed by FNV-1a 64-bit hash hold encoded identities; byte equality resolves the
rare hash collision.
*/
type forestDedup struct {
	buckets map[uint64][][]byte
}

func newForestDedup() *forestDedup {
	return &forestDedup{
		buckets: make(map[uint64][][]byte),
	}
}

/*
insert reports whether forest was already present.
*/
func (cache *forestDedup) insert(forest []reasoning.Thought) bool {
	key := newThoughtKey()
	key.writeForest(forest)
	hash := key.fnv64()
	encoded := key.buffer

	for _, prior := range cache.buckets[hash] {
		if bytes.Equal(prior, encoded) {
			releaseThoughtKey(key)

			return true
		}
	}

	cache.buckets[hash] = append(cache.buckets[hash], bytes.Clone(encoded))
	releaseThoughtKey(key)

	return false
}
