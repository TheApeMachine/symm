package logic

import "errors"

const SourceCount = 13

var SpectrumSources = [SourceCount]SourceType{
	SourceCausal,
	SourceCorrelation,
	SourceCVD,
	SourceDepthFlow,
	SourceExhaustion,
	SourceFluid,
	SourceHawkes,
	SourceLeadLag,
	SourceLiquidity,
	SourceManifold,
	SourcePumpDump,
	SourceSentiment,
	SourceToxicity,
}

var sourceIndexByType map[SourceType]int

func init() {
	sourceIndexByType = make(map[SourceType]int, SourceCount)

	for index, source := range SpectrumSources {
		sourceIndexByType[source] = index
	}
}

/*
SourceIndex maps a signal source to its fixed spectrum slot.
*/
func SourceIndex(source SourceType) (int, error) {
	if source == SourceNone {
		return -1, errors.New("logic: source is not in the measurement spectrum")
	}

	index, ok := sourceIndexByType[source]

	if !ok {
		return -1, errors.New("logic: source is not in the measurement spectrum")
	}

	return index, nil
}

/*
SpectrumFilled reports whether every spectrum slot has a measurement.
*/
func SpectrumFilled(filled uint16) bool {
	return filled == (uint16(1)<<SourceCount)-1
}
