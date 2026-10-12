package data

import (
	"hash/crc64"
	"math"
	"strings"
	"sync"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
standardizer owns one metric stream's causal rank: a summary of every
observation the stream has already produced. An observation is placed among
the observations before it and only then folded in, so a z-score never
contains its own value or any later one.

The z-score is the standard normal quantile of the observation's mid-rank
among its priors. Under a stream whose observations are exchangeable it is
standard normal whatever the stream's distribution: quantized prices that sit
on one tick, sizes with no finite variance, and rates over intervals near the
clock's grain all standardize alike, with no scale that rounding residue or a
quiet stretch can shrink. A value beyond every prior is placed at the edge of
the priors, never at a distance divided by a dispersion they never showed.
*/
type standardizer struct {
	mu    sync.Mutex
	ranks core.Ranks
}

/*
standardizerKey identifies one metric stream. The epoch is part of the
identity so a replay of one epoch never inherits ranks from another.
*/
type standardizerKey struct {
	epoch  int64
	source string
	label  string
	metric string
}

/*
seedTable is the ECMA-182 CRC-64 polynomial table the stream seeds are
checksummed with.
*/
var seedTable = crc64.MakeTable(crc64.ECMA)

/*
seed derives the stream's compaction seed from its identity, so the same
stream replayed in the same order reproduces the same ranks.
*/
func (key standardizerKey) seed() uint64 {
	identity := strings.Join([]string{
		key.source, key.label, key.metric,
	}, "\x00")

	return crc64.Checksum([]byte(identity), seedTable)
}

/*
standardizers holds every stream's standardizer. Producers derive their output
from a fresh ingress frame on every event, so state carried on frames dies
with the frame; every producer finalizes through this package, which makes it
the one owner that sees each stream from its first observation. Signals run on
separate goroutines, so lookup goes through sync.Map and each stream locks its
own ranks.
*/
var standardizers sync.Map

/*
standardizerFor returns the stream's standardizer, creating it on first use.
*/
func standardizerFor(key standardizerKey) *standardizer {
	if found, ok := standardizers.Load(key); ok {
		return found.(*standardizer)
	}

	found, _ := standardizers.LoadOrStore(key, &standardizer{ranks: core.NewRanks(key.seed())})

	return found.(*standardizer)
}

/*
observe returns raw's z-score against the observations before it and the
number of them, then incorporates raw. The mid-rank places raw among n priors
and itself, p = (below + (equal + 1) / 2) / (n + 1), which lies strictly
inside (0, 1); the z-score is its standard normal quantile. It is undefined,
with zero support, while n is below core.MinimumRankPrior.
*/
func (state *standardizer) observe(raw float64) (standardized float64, support float64, defined bool) {
	state.mu.Lock()
	defer state.mu.Unlock()

	priors := state.ranks.Count()
	below, equal := state.ranks.Rank(raw)
	state.ranks.Add(raw)

	if priors < core.MinimumRankPrior {
		return 0, 0, false
	}

	probability := (below + (equal+core.Unit)/2) / (priors + core.Unit)

	return math.Sqrt2 * math.Erfinv(2*probability-core.Unit), priors, true
}
