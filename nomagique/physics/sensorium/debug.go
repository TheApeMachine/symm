package sensorium

import (
	"fmt"
	"math"
	"strconv"

	"github.com/theapemachine/errnie"
)

/*
dbgTagNames maps a kernel debug tag to what the kernel was rejecting when it
raised it. The kernels cannot print, so they append compact events into a
device buffer and the host decodes them; without this the buffer is a wall of
hex that says nothing about which admissibility contract failed.

The tags are defined at their dbg_log call sites in manifold.metal.
*/
var dbgTagNames = map[uint32]string{
	0x03: "invalid heat capacity",
	0x04: "negative internal energy gathered",
	0x05: "non-finite temperature, heat or velocity",
	0x06: "non-finite advection state",
	0x07: "invalid particle mass or negative density",
	0x08: "low-density envelope violated",
	0x12: "gas RK2 stage 1 produced inadmissible state",
	0x13: "gas RK2 stage 2 produced inadmissible state",
	0x20: "gas RHS produced non-finite in stage 1",
	0x21: "gas RHS produced non-finite in stage 2",
}

/*
drainDebug reports and clears the kernel event buffer.

The physics kernels are fail-fast: an inadmissible cell or particle is written
as qNaN rather than being silently floored, and the reason is logged here first.
That NaN then travels the whole pipeline and surfaces far downstream — as a
rejected planner feature, say — with nothing left to say which contract broke.
Draining the buffer each step is what turns that into the cell index and the
conserved quantities the kernel actually refused.
*/
func (fluid *workspace) drainDebug() error {
	if fluid.dbgHead == nil || fluid.dbgWords == nil {
		return nil
	}

	head := fluid.dbgHead.UInt32Slice()

	if len(head) == 0 || head[0] == 0 {
		return nil
	}

	recorded := head[0]
	head[0] = 0
	words := fluid.dbgWords.UInt32Slice()
	stored := min(recorded, uint32(dbgCapacity))
	rejected := 0
	firstRejection := ""

	for event := uint32(0); event < stored; event++ {
		base := event * dbgWordsPerEvent

		if int(base)+dbgWordsPerEvent > len(words) {
			break
		}

		tag := words[base]

		rejected++

		name, known := dbgTagNames[tag]

		if !known {
			name = "unknown kernel tag"
		}

		if firstRejection == "" {
			firstRejection = debugRejection(
				name,
				tag,
				words[base+1],
				words[base+2:base+dbgWordsPerEvent],
			)
		}
	}

	if rejected == 0 {
		return nil
	}

	return errnie.Error(errnie.Err(
		errnie.Internal,
		fmt.Sprintf(
			"%s; rejected_events=%d, recorded_events=%d, stored_events=%d",
			firstRejection,
			rejected,
			recorded,
			stored,
		),
		nil,
	))
}

func debugRejection(
	name string,
	tag uint32,
	index uint32,
	values []uint32,
) string {
	fields := fmt.Sprintf(
		"rho=%s, e_int=%s, mom_x=%s, mom_y=%s",
		dbgFloat(values[0]),
		dbgFloat(values[1]),
		dbgFloat(values[2]),
		dbgFloat(values[3]),
	)

	if tag == 0x12 || tag == 0x13 {
		fields = fmt.Sprintf(
			"input_rho=%s, input_e_int=%s, candidate_rho=%s, candidate_e_int=%s",
			dbgFloat(values[0]),
			dbgFloat(values[1]),
			dbgFloat(values[2]),
			dbgFloat(values[3]),
		)
	}

	return fmt.Sprintf(
		"sensorium: %s (tag 0x%02x) at cell/particle %d: %s",
		name,
		tag,
		index,
		fields,
	)
}

/*
dbgFloat renders one packed debug word. The kernel logs the very values that
failed its admissibility contract, so NaN and Inf are the expected content here
rather than an anomaly, and they are spelled out instead of being formatted
away.
*/
func dbgFloat(bits uint32) string {
	value := float64(math.Float32frombits(bits))

	if math.IsNaN(value) {
		return "NaN"
	}

	if math.IsInf(value, 1) {
		return "+Inf"
	}

	if math.IsInf(value, -1) {
		return "-Inf"
	}

	return strconv.FormatFloat(value, 'g', 6, 64)
}
