package tokenizer

/*
SegmentMeta accumulates boundary event tags across a segment (XOR tags).
*/
type SegmentMeta uint64

func (meta SegmentMeta) RollLeft(steps int) SegmentMeta {
	if steps <= 0 {
		return meta
	}

	steps &= 63
	shifted := uint64(meta) << steps

	return SegmentMeta(shifted | (uint64(meta) >> (64 - steps)))
}

func tagMeta(meta SegmentMeta, event int) SegmentMeta {
	switch event {
	case EventLowVarianceFlux:
		return meta ^ SegmentMeta(1<<0)
	case EventDensitySpike:
		return meta ^ SegmentMeta(1<<1)
	case EventDensityTrough:
		return meta ^ SegmentMeta(1<<2)
	case EventPhaseInversion:
		return meta ^ SegmentMeta(1<<3)
	default:
		return meta
	}
}
