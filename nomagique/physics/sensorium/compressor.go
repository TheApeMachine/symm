package sensorium

import "sort"

/*
Compressor implements collision-is-compression: duplicate (byte, seq) pairs
inside one batch become one particle whose mass is their multiplicity.
*/
type Compressor struct {
	segmentLen int
	seen       map[[2]int64]int
	edges      map[[4]int64]int
	lastByte   int64
	lastSeq    int64
	hasLast    bool
	SeenCount  int
}

func NewCompressor(segmentLen int) *Compressor {
	if segmentLen < 1 {
		segmentLen = 1
	}

	return &Compressor{
		segmentLen: segmentLen,
		seen:       make(map[[2]int64]int),
		edges:      make(map[[4]int64]int),
	}
}

func (compressor *Compressor) Filter(
	bytes []int64,
	seqs []int64,
) (filteredBytes []int64, filteredSeqs []int64, counts []float32) {
	if len(bytes) == 0 {
		return nil, nil, nil
	}

	type pair struct {
		byteValue int64
		seq       int64
	}

	tallies := make(map[pair]float32, len(bytes))
	order := make([]pair, 0, len(bytes))

	for index, byteValue := range bytes {
		key := pair{byteValue: byteValue, seq: seqs[index]}

		if _, seen := tallies[key]; !seen {
			order = append(order, key)
		}

		tallies[key]++
	}

	sort.Slice(order, func(left, right int) bool {
		if order[left].byteValue != order[right].byteValue {
			return order[left].byteValue < order[right].byteValue
		}

		return order[left].seq < order[right].seq
	})

	filteredBytes = make([]int64, len(order))
	filteredSeqs = make([]int64, len(order))
	counts = make([]float32, len(order))

	for index, key := range order {
		filteredBytes[index] = key.byteValue
		filteredSeqs[index] = key.seq
		counts[index] = tallies[key]
		rel := key.seq % int64(compressor.segmentLen)

		if rel < 0 {
			rel += int64(compressor.segmentLen)
		}

		token := [2]int64{key.byteValue, rel}
		compressor.seen[token] += int(counts[index])
	}

	compressor.SeenCount += len(order)
	compressor.recordEdges(bytes, seqs)

	return filteredBytes, filteredSeqs, counts
}

func (compressor *Compressor) recordEdges(bytes, seqs []int64) {
	if len(bytes) == 0 {
		return
	}

	segment := int64(compressor.segmentLen)

	if compressor.hasLast && seqs[0] == compressor.lastSeq+1 {
		from := [2]int64{compressor.lastByte, modSeg(compressor.lastSeq, segment)}
		to := [2]int64{bytes[0], modSeg(seqs[0], segment)}
		compressor.edges[[4]int64{from[0], from[1], to[0], to[1]}]++
	}

	for index := 0; index+1 < len(bytes); index++ {
		if seqs[index+1] != seqs[index]+1 {
			continue
		}

		from := [2]int64{bytes[index], modSeg(seqs[index], segment)}
		to := [2]int64{bytes[index+1], modSeg(seqs[index+1], segment)}
		compressor.edges[[4]int64{from[0], from[1], to[0], to[1]}]++
	}

	compressor.lastByte = bytes[len(bytes)-1]
	compressor.lastSeq = seqs[len(seqs)-1]
	compressor.hasLast = true
}

func modSeg(seq, segment int64) int64 {
	rel := seq % segment

	if rel < 0 {
		rel += segment
	}

	return rel
}

func (compressor *Compressor) Seen() map[[2]int64]int {
	return compressor.seen
}

func (compressor *Compressor) Edges() map[[4]int64]int {
	return compressor.edges
}
