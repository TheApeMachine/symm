package nomagique

import "fmt"

const MaxSamples = 128

var (
	SampleValue = MustIntern("sample")
	SampleCount = MustIntern("count")
	SampleHead  = MustIntern("head")
	SampleReady = MustIntern("ready")

	sampleSymbols [MaxSamples]Symbol
)

func init() {
	for index := range MaxSamples {
		sampleSymbols[index] = MustIntern(fmt.Sprintf("sample/%d", index))
	}
}

/*
SampleSymbol returns the interned slot for one generic sample position.
*/
func SampleSymbol(index int) (Symbol, bool) {
	if index < 0 || index >= MaxSamples {
		return 0, false
	}

	return sampleSymbols[index], true
}

/*
MustSampleSymbol returns one generic sample slot or panics.
*/
func MustSampleSymbol(index int) Symbol {
	symbol, found := SampleSymbol(index)

	if !found {
		panic(fmt.Sprintf("nomagique: sample index %d is outside capacity", index))
	}

	return symbol
}
