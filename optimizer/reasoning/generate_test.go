package reasoning

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func forestDepth(forest []perspectives.Thought) int {
	deepest := 0

	var walk func(thought perspectives.Thought, depth int)
	walk = func(thought perspectives.Thought, depth int) {
		if depth > deepest {
			deepest = depth
		}

		for _, child := range thought.Then {
			walk(child, depth+1)
		}
	}

	for _, thought := range forest {
		walk(thought, 1)
	}

	return deepest
}

func ignitionRows() []perspectives.Measurement {
	base := time.Unix(1_700_000_000, 0)

	return []perspectives.Measurement{
		{Symbol: "BTC/EUR", Category: perspectives.CategoryVerticalIgnition, SNR: 1.5, Last: 100, At: base},
		{Symbol: "BTC/EUR", Category: perspectives.CategoryVerticalIgnition, SNR: 1.5, Last: 101, At: base.Add(time.Second)},
		{Symbol: "BTC/EUR", Last: 102, At: base.Add(2 * time.Second)},
	}
}

func TestDeriveVocabularyAndSeeds(t *testing.T) {
	Convey("Given rows carrying one signal category", t, func() {
		vocab := DeriveVocabulary(ignitionRows())

		Convey("The vocabulary derives the category from the data", func() {
			So(vocab.Categories, ShouldContain, perspectives.CategoryVerticalIgnition)
		})

		Convey("Seeds are one minimal, valid strategy per category", func() {
			seeds := Seeds(vocab)
			So(len(seeds), ShouldEqual, len(vocab.Categories))

			seed := seeds[0]

			// A coherent seed: an entry branch and a protective management leg.
			_, hasEntry := entryNode(seed)
			_, hasManagement := managementNode(seed)
			So(hasEntry, ShouldBeTrue)
			So(hasManagement, ShouldBeTrue)

			// It round-trips through the playbook serializer (a real, writable tree).
			encoded, err := perspectives.MarshalThoughts(seed, 2)
			So(err, ShouldBeNil)
			reparsed, err := perspectives.ParseThoughts(encoded)
			So(err, ShouldBeNil)
			So(reparsed, ShouldResemble, seed)
		})
	})
}

func TestTemporalizeDeepensTheEntry(t *testing.T) {
	Convey("Given a flat seed strategy", t, func() {
		vocab := DeriveVocabulary(ignitionRows())
		seed := Seeds(vocab)[0]

		So(forestDepth(seed), ShouldEqual, 1) // entry and management are both roots

		Convey("temporalize pushes the entry behind a price follow-through", func() {
			neighbors := temporalizeEntry(seed, vocab)
			So(len(neighbors), ShouldBeGreaterThan, 0)

			deepened := neighbors[0]
			So(forestDepth(deepened), ShouldEqual, 2) // entry is now a Then child

			// The entry action survived the move into the chain.
			_, hasEntry := entryNode(deepened)
			So(hasEntry, ShouldBeTrue)

			// The original seed is untouched (clone isolation).
			So(forestDepth(seed), ShouldEqual, 1)
		})

		Convey("Re-applying temporalize grows an ordered multi-tick chain", func() {
			once := temporalizeEntry(seed, vocab)[0]
			twice := temporalizeEntry(once, vocab)[0]
			So(forestDepth(twice), ShouldEqual, 3)
		})
	})
}

func TestNeighborsAreDistinctAndValid(t *testing.T) {
	Convey("Given a seed and its neighbours", t, func() {
		vocab := DeriveVocabulary(ignitionRows())
		seed := Seeds(vocab)[0]

		neighbors := Neighbors(seed, vocab)

		Convey("There are several, all parse, and none equals the seed", func() {
			So(len(neighbors), ShouldBeGreaterThan, 3)

			seedKey := keyOf(seed)
			keys := make(map[string]bool)

			for _, neighbor := range neighbors {
				key := keyOf(neighbor)
				So(key, ShouldNotEqual, seedKey)
				So(key, ShouldNotBeEmpty) // marshalled cleanly
				keys[key] = true
			}

			So(len(keys), ShouldEqual, len(neighbors)) // all distinct
		})
	})
}

func BenchmarkDeriveVocabulary(b *testing.B) {
	rows := ignitionRows()

	for b.Loop() {
		_ = DeriveVocabulary(rows)
	}
}

func BenchmarkSeeds(b *testing.B) {
	vocab := DeriveVocabulary(ignitionRows())

	for b.Loop() {
		_ = Seeds(vocab)
	}
}

func BenchmarkKeyOf(b *testing.B) {
	forest := Seeds(DeriveVocabulary(ignitionRows()))[0]

	for b.Loop() {
		_ = keyOf(forest)
	}
}

func BenchmarkCloneForest(b *testing.B) {
	forest := Seeds(DeriveVocabulary(ignitionRows()))[0]

	for b.Loop() {
		_ = cloneForest(forest)
	}
}
