package broker

import (
	"math"
	"time"

	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
)

/*
path composes nomagique stream memory for one armed climb:
CUSUM (up/down drift), MoveBaseline (n1/n2 envelopes), Momentum (ascending),
TimeElastic (down vs its own time-decayed scale), Hysteresis (debounce).
Exit candidates are derived from those baselines — never fixed exhaust cutoffs.
*/
type path struct {
	cusum     *probability.CUSUM
	ups       *algorithm.MoveBaseline
	downs     *algorithm.MoveBaseline
	momentum  *adaptive.Momentum
	gate      *adaptive.Hysteresis
	scale     *adaptive.TimeElastic
	recovered *adaptive.EMA

	peak     float64
	last     float64
	hasLast  bool
	openPull float64
	samples  int

	meanUp    float64
	meanDown  float64
	upReady   bool
	downReady bool
	cusumPos  float64
	cusumNeg  float64
	recov     float64
}

func newPath() *path {
	recovered, _ := adaptive.NewEMA(adaptive.EMAConfig{Period: 5, Smoothing: 2})

	return &path{
		cusum: probability.NewCUSUM(),
		ups: algorithm.NewMoveBaseline(algorithm.MoveBaselineConfig{
			MinObs:  3,
			PathCap: 64,
		}),
		downs: algorithm.NewMoveBaseline(algorithm.MoveBaselineConfig{
			MinObs:  3,
			PathCap: 64,
		}),
		momentum:  adaptive.NewMomentum(),
		gate:      adaptive.NewHysteresis(adaptive.HysteresisConfig{Window: 2, Threshold: 0.5}),
		scale:     adaptive.NewTimeElastic(adaptive.TimeElasticConfig{Halflife: 30 * time.Second, Epsilon: 1e-6}),
		recovered: recovered,
	}
}

/*
pathReading is one mark's derived view. armedExit means this print is outside
the learned envelope (or ignition stalled near peak with no down memory yet).
*/
type pathReading struct {
	meanUp     float64
	meanDown   float64
	recovered  float64
	openPull   float64
	peak       float64
	last       float64
	samples    int
	cusumPos   float64
	cusumNeg   float64
	momentum   float64
	momReady   bool
	upReady    bool
	downReady  bool
	ascending  bool
	ignition   bool
	armedExit  bool
	gateArmed  bool
}

/*
note folds one PnL print into path memory. Unchanged PnL is ignored.
*/
func (path *path) note(pnlPct float64, at time.Time) pathReading {
	if at.IsZero() {
		at = time.Now()
	}

	if !path.hasLast {
		path.last = pnlPct
		path.peak = pnlPct
		path.hasLast = true
		path.samples = 1
		cusumOut, _ := path.cusum.Measure(pnlPct)
		path.cusumPos = cusumOut.Positive
		path.cusumNeg = cusumOut.Negative
		_, _ = path.momentum.Measure(pnlPct)
		return path.reading(false, 0, false)
	}

	if pnlPct == path.last {
		return path.reading(false, 0, false)
	}

	delta := pnlPct - path.last
	path.last = pnlPct
	path.samples++

	cusumOut, _ := path.cusum.Measure(pnlPct)
	path.cusumPos = cusumOut.Positive
	path.cusumNeg = cusumOut.Negative

	momOut, _ := path.momentum.Measure(pnlPct)

	downMoved := false
	aboveScale := false

	if delta > 0 {
		upOut, _ := path.ups.Measure(delta)
		path.meanUp = upOut.Mean
		path.upReady = upOut.Ready == 1
	} else {
		down := -delta
		downOut, _ := path.downs.Measure(down)
		path.meanDown = downOut.Mean
		path.downReady = downOut.Ready == 1
		downMoved = downOut.Ready == 1 && downOut.Moved == 1

		scaleOut, err := path.scale.Measure(adaptive.TimedValue{Value: down, At: at})
		aboveScale = err == nil && scaleOut.Ready && scaleOut.Value > 1
	}

	if pnlPct > path.peak {
		if path.openPull > 0 && path.recovered != nil {
			path.recov, _ = path.recovered.Measure(path.openPull)
		}
		path.peak = pnlPct
		path.openPull = 0
	} else if pull := path.peak - pnlPct; pull > path.openPull {
		path.openPull = pull
	}

	abnormal := path.downReady && (downMoved || aboveScale)
	return path.reading(momOut.Ready, momOut.Value, abnormal)
}

func (path *path) reading(momReady bool, momentum float64, abnormal bool) pathReading {
	ascending := momReady && momentum > 0
	if !momReady {
		ascending = path.openPull <= path.meanUp
	}

	// No ready down-envelope yet while ups have printed ⇒ ignition.
	ignition := path.samples >= 2 && !path.downReady && path.meanUp > 0

	nearPeak := path.meanUp <= 0 || path.openPull <= path.meanUp
	ignitionExit := ignition && momReady && momentum <= 0 && nearPeak

	armedExit := (abnormal && !ascending) || ignitionExit

	gateIn := 0.0
	if armedExit {
		gateIn = 1
	}

	// Trail uses Hysteresis window 2; ignition arms on the stall mark itself.
	gateOut, _ := path.gate.Measure(gateIn)
	gateArmed := ignitionExit || gateOut.Value == 1

	return pathReading{
		meanUp:    path.meanUp,
		meanDown:  math.Max(path.meanDown, path.recov),
		recovered: path.recov,
		openPull:  path.openPull,
		peak:      path.peak,
		last:      path.last,
		samples:   path.samples,
		cusumPos:  path.cusumPos,
		cusumNeg:  path.cusumNeg,
		momentum:  momentum,
		momReady:  momReady,
		upReady:   path.upReady,
		downReady: path.downReady,
		ascending: ascending,
		ignition:  ignition,
		armedExit: armedExit,
		gateArmed: gateArmed,
	}
}

func (path *path) reset() {
	*path = *newPath()
}
