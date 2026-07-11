package manifold

import (
	"time"

	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

const replayCapacity = 8192

/*
ReplayFrame records one deterministic pipeline observation for audit.
*/
type ReplayFrame struct {
	At            time.Time
	Symbol        string
	FrameType     string
	Checksum      uint32
	Epoch         uint64
	ScaleVersion  uint64
	InvalidReason InvalidReason
	Accounting    PopulationAccounting
	VisibleMass   float64
	CohortCount   int
	OrderCount    int
	DepositCount  int
	Ready         bool
	Conservation  float64
	StressAniso   float64
	DeltaT        float64
	Subdivisions  int
	PriceScale    float64
	SizeScale     float64
	PressureTrace float64
	PushFailed    bool
}

/*
ReplayRecorder appends immutable frames for post-mortem replay identity checks.
The hot Record path is a single-producer lock-free Push into structure.SPSCRing.
*/
type ReplayRecorder struct {
	ring *structure.SPSCRing[ReplayFrame]
}

func NewReplayRecorder() *ReplayRecorder {
	ring := structure.NewSPSCRing[ReplayFrame](replayCapacity, true)

	if ring == nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"logic manifold replay: ring was not created",
			nil,
		))
	}

	return &ReplayRecorder{ring: ring}
}

func (recorder *ReplayRecorder) Record(
	symbol string,
	row kraken.Level3Data,
	state State,
	accounting PopulationAccounting,
	cohortCount int,
	orderCount int,
	depositCount int,
) bool {
	if recorder.ring == nil {
		return false
	}

	at := row.Timestamp

	if at.IsZero() {
		at = state.At
	}

	pushed := recorder.ring.Push(ReplayFrame{
		At:            at,
		Symbol:        symbol,
		FrameType:     row.Type,
		Checksum:      row.Checksum,
		Epoch:         state.Epoch,
		ScaleVersion:  state.ScaleVersion,
		InvalidReason: state.InvalidReason,
		Accounting:    accounting,
		VisibleMass:   state.VisibleMass,
		CohortCount:   cohortCount,
		OrderCount:    orderCount,
		DepositCount:  depositCount,
		Ready:         state.Ready,
		Conservation:  state.ConservationResidual,
		StressAniso:   state.StressAnisotropy,
		DeltaT:        state.DeltaT,
		Subdivisions:  state.Subdivisions,
		PriceScale:    state.PriceScale,
		SizeScale:     state.SizeScale,
		PressureTrace: state.PressureTensor.Trace(),
	})

	if !pushed {
		return false
	}

	return true
}

func (recorder *ReplayRecorder) Frames() []ReplayFrame {
	if recorder.ring == nil {
		return nil
	}

	queued := recorder.ring.Len()
	frames := make([]ReplayFrame, 0, queued)

	recorder.ring.Select(0).Do(func(frame ReplayFrame) {
		frames = append(frames, frame)
	})

	return frames
}

func (recorder *ReplayRecorder) Dropped() uint64 {
	if recorder.ring == nil {
		return 0
	}

	return recorder.ring.Dropped()
}
