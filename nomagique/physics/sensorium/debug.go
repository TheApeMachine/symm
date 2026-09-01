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
const (
	// dbgTagBaseline is the first-particle sample the gather kernel emits on
	// every step, and dbgTagVacuum marks a cell that simply holds nothing.
	// Both are routine observations rather than rejected states.
	dbgTagBaseline = 0x01
	dbgTagVacuum   = 0x02
)

var dbgTagNames = map[uint32]string{
	0x02: "exact vacuum gather",
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
func (fluid *workspace) drainDebug() {
	if fluid.dbgHead == nil || fluid.dbgWords == nil {
		return
	}

	head := fluid.dbgHead.UInt32Slice()

	if len(head) == 0 || head[0] == 0 {
		return
	}

	recorded := head[0]
	head[0] = 0
	words := fluid.dbgWords.UInt32Slice()
	stored := min(recorded, uint32(dbgCapacity))
	reported := 0

	for event := uint32(0); event < stored; event++ {
		base := event * dbgWordsPerEvent

		if int(base)+dbgWordsPerEvent > len(words) {
			break
		}

		tag := words[base]

		// A cell holding no particles is an ordinary state of a sparse domain,
		// and the baseline sample is instrumentation the kernel emits every
		// step. Neither is a rejection, so neither is reported as one; only the
		// contracts the kernel actually refused reach the log.
		if tag == dbgTagBaseline || tag == dbgTagVacuum {
			continue
		}

		reported++

		name, known := dbgTagNames[tag]

		if !known {
			name = "unknown kernel tag"
		}

		errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"sensorium: %s (tag 0x%02x) at cell/particle %d | rho=%s e_int=%s mom_x=%s mom_y=%s",
				name,
				tag,
				words[base+1],
				dbgFloat(words[base+2]),
				dbgFloat(words[base+3]),
				dbgFloat(words[base+4]),
				dbgFloat(words[base+5]),
			),
			nil,
		))
	}

	// Only say events were lost when something worth reporting was actually
	// reported: a sparse domain fills the buffer with routine vacuum samples
	// every step, and announcing that overflow each time would bury the
	// rejections this exists to surface.
	if recorded > stored && reported > 0 {
		errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"sensorium: %d further kernel events exceeded the %d-event debug buffer",
				recorded-stored, dbgCapacity,
			),
			nil,
		))
	}
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
