package keyindex

import (
	"fmt"
	"strconv"
	"strings"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
)

/*
Entry is one stored object key in canonical S3 layout:

	origin/role/scope/epoch%019d/tick%019d.bin
*/
type Entry struct {
	Canonical string
	Origin    string
	Role      string
	Scope     string
	Epoch     uint64
	Tick      uint64
}

/*
Segment names the five path components of a storage key.
*/
type Segment int

const (
	SegOrigin Segment = iota
	SegRole
	SegScope
	SegEpoch
	SegTick
)

/*
ViewOrder lists path segments for an alternate radix view. Every permutation
of the five segments is valid; terminal nodes still point at the canonical key.
*/
type ViewOrder [5]Segment

var (
	CanonicalView = ViewOrder{SegOrigin, SegRole, SegScope, SegEpoch, SegTick}
	ScopeFirstView = ViewOrder{SegScope, SegOrigin, SegRole, SegEpoch, SegTick}
)

/*
Index maps object keys under several prefix views and caches downloaded bytes
by canonical key. Trees are immutable: Insert returns a new Index.
*/
type Index struct {
	canonical *iradix.Tree[*Entry]
	scopeView *iradix.Tree[*Entry]
	objects   *iradix.Tree[[]byte]
}

func New() *Index {
	return &Index{
		canonical: iradix.New[*Entry](),
		scopeView: iradix.New[*Entry](),
		objects:   iradix.New[[]byte](),
	}
}

func (idx *Index) Clone() *Index {
	if idx == nil {
		return New()
	}

	return &Index{
		canonical: idx.canonical,
		scopeView: idx.scopeView,
		objects:   idx.objects,
	}
}

/*
ParseObjectKey splits a listed storage key into its segments.
*/
func ParseObjectKey(key string) (*Entry, error) {
	key = strings.TrimPrefix(key, "/")
	parts := strings.Split(key, "/")

	if len(parts) < 5 {
		return nil, fmt.Errorf("keyindex: %q has fewer than five segments", key)
	}

	tickName := parts[len(parts)-1]
	tickStr, ok := strings.CutSuffix(tickName, ".bin")

	if !ok || len(tickStr) != 19 {
		return nil, fmt.Errorf("keyindex: %q has no %%019d.bin tick segment", key)
	}

	tick, err := strconv.ParseUint(tickStr, 10, 64)

	if err != nil {
		return nil, fmt.Errorf("keyindex: tick in %q: %w", key, err)
	}

	epochStr := parts[len(parts)-2]

	if len(epochStr) != 19 {
		return nil, fmt.Errorf("keyindex: %q has no %%019d epoch segment", key)
	}

	epoch, err := strconv.ParseUint(epochStr, 10, 64)

	if err != nil {
		return nil, fmt.Errorf("keyindex: epoch in %q: %w", key, err)
	}

	origin := parts[0]
	role := parts[1]
	scope := strings.Join(parts[2:len(parts)-2], "/")

	return &Entry{
		Canonical: key,
		Origin:    origin,
		Role:      role,
		Scope:     scope,
		Epoch:     epoch,
		Tick:      tick,
	}, nil
}

func segmentOf(entry *Entry, seg Segment) string {
	switch seg {
	case SegOrigin:
		return entry.Origin
	case SegRole:
		return entry.Role
	case SegScope:
		return entry.Scope
	case SegEpoch:
		return fmt.Sprintf("%019d", entry.Epoch)
	case SegTick:
		return fmt.Sprintf("%019d.bin", entry.Tick)
	default:
		return ""
	}
}

func viewKey(entry *Entry, order ViewOrder) []byte {
	parts := make([]string, 0, 5)

	for _, seg := range order {
		parts = append(parts, segmentOf(entry, seg))
	}

	return []byte(strings.Join(parts, "/"))
}

/*
InsertObjectKey registers key in the canonical and scope-first views.
*/
func (idx *Index) InsertObjectKey(key string) (*Index, error) {
	entry, err := ParseObjectKey(key)

	if err != nil {
		return idx, err
	}

	next := idx.Clone()
	next.canonical, _, _ = next.canonical.Insert([]byte(entry.Canonical), entry)
	next.scopeView, _, _ = next.scopeView.Insert(viewKey(entry, ScopeFirstView), entry)

	return next, nil
}

/*
Object returns a cached copy of the object body, if present.
*/
func (idx *Index) Object(key string) ([]byte, bool) {
	if idx == nil {
		return nil, false
	}

	buf, ok := idx.objects.Get([]byte(key))

	if !ok || len(buf) == 0 {
		return nil, false
	}

	out := make([]byte, len(buf))
	copy(out, buf)

	return out, true
}

/*
StoreObject caches body under the canonical key.
*/
func (idx *Index) StoreObject(key string, body []byte) *Index {
	if idx == nil {
		idx = New()
	}

	next := idx.Clone()
	stored := make([]byte, len(body))
	copy(stored, body)
	next.objects, _, _ = next.objects.Insert([]byte(key), stored)

	return next
}

/*
WalkScopePrefix visits every entry whose scope-first view key has prefix
scope/ (for example every draft object for symbol BTC/USD).
*/
func (idx *Index) WalkScopePrefix(scope string, yield func(*Entry) bool) {
	if idx == nil {
		return
	}

	prefix := []byte(scope + "/")
	it := idx.scopeView.Root().Iterator()
	it.SeekLowerBound(prefix)

	for {
		key, entry, ok := it.Next()

		if !ok || entry == nil {
			return
		}

		if !strings.HasPrefix(string(key), string(prefix)) {
			return
		}

		if !yield(entry) {
			return
		}
	}
}
