package correlation

import (
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"math"
	"testing"
	"time"
)

func TestRelationsAll(t *testing.T) {
	Convey("Pair evidence retains sign and support independently of the cohort", t, func() {
		entity := NewTicker()
		for index := 0; index < 30; index++ {
			at := timestamp(int64(index + 1))
			excursion := 0.01 * math.Sin(float64(index))
			for symbol, price := range map[string]float64{"A": 100 * math.Exp(excursion), "B": 200 * math.Exp(excursion), "C": 300 * math.Exp(-excursion)} {
				So(entity.Step(ticker(symbol, price, at)).Err, ShouldBeNil)
			}
		}
		pairs := make(map[[2]string]Relation)
		for relation := range entity.Relations.All() {
			pairs[[2]string{relation.Left, relation.Right}] = relation
		}
		So(len(pairs), ShouldEqual, 3)
		So(pairs[[2]string{"A", "B"}].Signed, ShouldAlmostEqual, 1)
		So(pairs[[2]string{"A", "C"}].Signed, ShouldAlmostEqual, -1)
		So(pairs[[2]string{"A", "C"}].Absolute, ShouldAlmostEqual, 1)
		So(pairs[[2]string{"A", "C"}].Support, ShouldBeGreaterThan, 3)
		So(pairs[[2]string{"A", "C"}].Defined, ShouldBeTrue)
		stale := pairs[[2]string{"A", "B"}].At
		entity.Step(ticker("A", 101, timestamp(40)))
		for relation := range entity.Relations.All() {
			if relation.Right == "B" {
				So(relation.At.Equal(stale), ShouldBeTrue)
			}
		}
		count := 0
		for range entity.Relations.All() {
			count++
			break
		}
		So(count, ShouldEqual, 1)
	})
}

func BenchmarkRelationsAll(b *testing.B) {
	relations := &Relations{pairs: make(map[[2]string]Relation)}
	// The production universe contains roughly 640 symbols: 204480 unordered pairs.
	for left := 0; left < 640; left++ {
		for right := left + 1; right < 640; right++ {
			key := [2]string{fmt.Sprint(left), fmt.Sprint(right)}
			relations.pairs[key] = Relation{Left: key[0], Right: key[1], At: time.Unix(1, 0)}
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		for range relations.All() {
		}
	}
}
