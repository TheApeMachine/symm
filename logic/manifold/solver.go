package manifold

import (
	"fmt"
	"math"
	"maps"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/book"
	mgrbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/signal/compute"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Solver owns one resident Sensorium domain for the complete market universe.
Symbols contribute observations to the same gas and wave fields; they are not
split into independent simulations that cannot interfere.
*/
type Solver struct {
	mu        sync.Mutex
	api       *websocket.API
	config    pfluid.Config
	domain    *pfluid.Domain
	recorder  *audit.Recorder
	tokenizer *Tokenizer
	residency int
	turnover  int
	ui        chan []byte
	binui     chan []byte
}

/*
NewSolver creates the single shared Metal domain and a spectral corpus bounded
by the same explicit event-history capacity as the live market feed.
*/
func NewSolver(
	api *websocket.API,
	ui, binui chan []byte,
	recorder *audit.Recorder,
) *Solver {
	config := pfluid.DefaultConfig()
	configuredDelta := viper.GetDuration("market.manifold.integration_interval")

	if configuredDelta > 0 && configuredDelta.Seconds() < float64(config.MaxDelta) {
		config.MaxDelta = float32(configuredDelta.Seconds())
	}

	symbols := make([]string, 0)

	if api != nil {
		api.Books().Range(func(key, _ any) bool {
			name, ok := key.(string)

			if ok {
				symbols = append(symbols, name)
			}

			return true
		})
	}

	domain, err := newDomain(config)
	errnie.Error(err)

	/*
		Residency must stay well below the lattice size in practice. The shared
		thermo-wave solve destabilizes long before one quarter-cell occupancy once
		hundreds of symbols have contributed sequentially, so the resident history
		is bounded to one thirty-second of the cells unless config overrides it.
		That keeps the newest cross-market carriers while forcing eviction to act
		before the pilot-wave gather reaches the observed non-finite regime.
	*/
	cells := config.Grid.X * config.Grid.Y * config.Grid.Z
	residency := max(cells/32, 1)

	if configuredResidency := viper.GetInt("market.manifold.residency"); configuredResidency > 0 {
		residency = configuredResidency
	}

	return &Solver{
		api:       api,
		config:    config,
		domain:    domain,
		recorder:  recorder,
		tokenizer: NewTokenizer(config, symbols),
		residency: residency,
		ui:        ui,
		binui:     binui,
	}
}

func newDomain(config pfluid.Config) (*pfluid.Domain, error) {
	var domain *pfluid.Domain

	err := compute.WithMetalInit(func() error {
		created, err := pfluid.NewDomain(config)

		if err != nil {
			return err
		}

		domain = created
		return nil
	})

	if err != nil {
		return nil, err
	}

	return domain, nil
}

/*
Update appends tokenized book samples for every changed Hawkes epoch, then
always advances the shared domain once for this tick and publishes symbol views
of that physical state. Inject is Hawkes-gated; the step is not.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	solver.mu.Lock()
	defer solver.mu.Unlock()

	solver.maybeRebase(thesis.At)

	stepped := false
	var updateErr error

	solver.api.Books().Range(func(key, value any) bool {
		book, ok := value.(*book.Book)

		if !ok {
			return true
		}

		name, ok := key.(string)

		if !ok {
			return true
		}

		hawkes := utils.ForSymbol(utils.Measurements(thesis, types.SourceHawkes), name)

		if len(hawkes) == 0 {
			return true
		}

		bidOrders := make([]*mgrbook.Order, 0)
		askOrders := make([]*mgrbook.Order, 0)

		for _, level := range book.Bids.Levels {
			bidOrders = append(bidOrders, level.Queue()...)
		}

		for _, level := range book.Asks.Levels {
			askOrders = append(askOrders, level.Queue()...)
		}

		buyIntensity := hawkes[len(hawkes)-1].Sample(
			types.MetricConditionalIntensity, types.SideBuy,
		).Raw
		sellIntensity := hawkes[len(hawkes)-1].Sample(
			types.MetricConditionalIntensity, types.SideSell,
		).Raw

		particles, contentIDs, err := solver.tokenizer.NewBatch(
			bidOrders,
			askOrders,
			book.Midpoint().Float64(),
			buyIntensity,
			sellIntensity,
			name,
		)

		if err != nil {
			updateErr = errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("failed to tokenize manifold particles for %s", name),
				err,
			))

			return false
		}

		if len(particles) == 0 || len(contentIDs) == 0 {
			return true
		}

		originalBatchParticles := len(particles)
		particles, contentIDs, droppedParticles := solver.filterBatch(particles, contentIDs)

		if droppedParticles > 0 && solver.recorder != nil {
			errnie.Error(audit.Record(recorderOrNil(solver.recorder), "predictive", map[string]any{
				"stage":                      "manifold",
				"symbol":                     name,
				"at":                         thesis.At,
				"status":                     "filtered",
				"dropped_particles":          droppedParticles,
				"original_batch_particles":   originalBatchParticles,
				"retained_batch_particles":   len(particles),
				"buy_conditional_intensity":  buyIntensity,
				"sell_conditional_intensity": sellIntensity,
			}))
		}

		if len(particles) == 0 || len(contentIDs) == 0 {
			return true
		}

		batchStart, err := solver.domain.Append(particles, contentIDs)

		if err != nil {
			updateErr = errnie.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf(
					"failed to append %d manifold particles for %s: %v",
					len(particles), name, err,
				),
				err,
			))

			return false
		}

		batchEnd := batchStart + len(particles) - 1
		maxBatchMass, maxBatchHeat, maxBatchEnergy := batchEnvelope(particles)
		solver.turnover += len(particles)

		solver.evict()

		/*
			A step that fails is one symbol's reading lost, not the tick. The
			domain is shared by the whole universe, so aborting here would let
			a single numerically awkward book silence every other symbol and
			stop the desk from deciding at all.
		*/
		if err = solver.Step(name, thesis.At); err != nil {
			if solver.recorder != nil {
				errnie.Error(audit.Record(recorderOrNil(solver.recorder), "predictive", map[string]any{
					"stage":                     "manifold",
					"symbol":                    name,
					"at":                        thesis.At,
					"status":                    "failed",
					"error":                     err.Error(),
					"resident_particles":        solver.domain.ParticleCount(),
					"batch_particles":           len(particles),
					"batch_start":               batchStart,
					"batch_end":                 batchEnd,
					"max_batch_mass":            maxBatchMass,
					"max_batch_heat":            maxBatchHeat,
					"max_batch_energy":          maxBatchEnergy,
					"buy_conditional_intensity": buyIntensity,
					"sell_conditional_intensity": sellIntensity,
				}))
			}

			solver.resetDomain(name, err)
			return true
		}

		stepped = true

		return true
	})

	if updateErr != nil {
		return updateErr
	}

	/*
		Stamp only once the field has actually advanced, so the stages that
		wait on the manifold can tell a completed step from a tick that had
		nothing to advance.
	*/
	if stepped {
		thesis.StampSource(types.SourceManifold, types.MarketDerived)
	}

	return nil
}

func (solver *Solver) maybeRebase(at time.Time) {
	if solver == nil || solver.residency <= 0 || solver.turnover < solver.residency {
		return
	}

	solver.recreateDomain("turnover", at, nil, map[string]any{
		"resident_particles": solver.domain.ParticleCount(),
		"turnover_particles": solver.turnover,
		"residency":         solver.residency,
	})
}

/*
evict bounds how much of the market stays resident in the shared field.

Every symbol appends into one domain and nothing ever leaves, so residency
grows without limit until the pilot-wave transport goes non-finite and the
manifold stops reading at all. Keeping the most recent particles holds the
field at a size it can integrate while preserving the newest state, which is
the part the strategy reads.
*/
func (solver *Solver) evict() {
	if solver.domain == nil {
		return
	}

	resident := solver.domain.ParticleCount()

	if resident <= solver.residency {
		return
	}

	/*
		Retain takes the indices to keep, and particles are appended in
		arrival order, so the tail is the newest of them.
	*/
	keep := make([]uint32, 0, solver.residency)

	for index := resident - solver.residency; index < resident; index++ {
		keep = append(keep, uint32(index))
	}

	if err := solver.domain.Retain(keep); err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"failed to bound manifold residency at %d particles",
				solver.residency,
			),
			err,
		))
	}
}

func (solver *Solver) Step(symbol string, at time.Time) error {
	diagnostics, err := solver.domain.Advance()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"failed to advance manifold for %s with %d resident particles: %v",
				symbol, solver.domain.ParticleCount(), err,
			),
			err,
		))
	}

	frame, stats, err := solver.domain.Display()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"failed to display manifold for %s with %d resident particles: %v",
				symbol, solver.domain.ParticleCount(), err,
			),
			err,
		))
	}

	if solver.binui != nil {
		payload, encodeErr := EncodeDisplay(
			symbol,
			at,
			int(stats.Width),
			int(stats.Height),
			frame,
		)

		if encodeErr != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf("failed to encode manifold display for %s", symbol),
				encodeErr,
			))
		}

		select {
		case solver.binui <- payload:
		default:
		}
	}

	if solver.ui != nil {
		row := datura.NewMap(
			"source", "manifold",
			"symbol", symbol,
			"at", at.Format(time.RFC3339),
		)

		if statsBytes, err := sonic.Marshal(stats); err == nil {
			var statsMap map[string]any
			if err := sonic.Unmarshal(statsBytes, &statsMap); err == nil {
				maps.Copy(row, statsMap)
			}
		}

		select {
		case solver.ui <- datura.NewMap(
			"manifold", []datura.Map[any]{row},
		).MarshalAndFree():
		default:
		}
	}

	if solver.recorder != nil {
		errnie.Error(audit.Record(recorderOrNil(solver.recorder), "predictive", map[string]any{
			"stage":  "manifold",
			"symbol": symbol,
			"at":     at,
			"value":  diagnostics,
		}))
	}

	return nil
}

func (solver *Solver) filterBatch(
	particles []pfluid.Particle,
	contentIDs []uint32,
) ([]pfluid.Particle, []uint32, int) {
	if len(particles) == 0 || len(contentIDs) == 0 {
		return nil, nil, 0
	}

	keptParticles := make([]pfluid.Particle, 0, len(particles))
	keptContentIDs := make([]uint32, 0, len(contentIDs))
	dropped := 0

	for index, particle := range particles {
		if !admissibleParticle(particle, solver.config) {
			dropped++
			continue
		}

		keptParticles = append(keptParticles, particle)
		keptContentIDs = append(keptContentIDs, contentIDs[index])
	}

	return keptParticles, keptContentIDs, dropped
}

func admissibleParticle(particle pfluid.Particle, config pfluid.Config) bool {
	values := []float32{
		particle.Position.X,
		particle.Position.Y,
		particle.Position.Z,
		particle.Velocity.X,
		particle.Velocity.Y,
		particle.Velocity.Z,
		particle.Mass,
		particle.Heat,
		particle.Energy,
		particle.Phase,
		particle.Omega,
	}

	for _, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}

	if particle.Mass <= 0 || particle.Heat < 0 || particle.Energy < 0 {
		return false
	}

	return particle.Omega >= config.OmegaMin && particle.Omega <= config.OmegaMax
}

func batchEnvelope(particles []pfluid.Particle) (float32, float32, float32) {
	maxBatchMass := float32(0)
	maxBatchHeat := float32(0)
	maxBatchEnergy := float32(0)

	for _, particle := range particles {
		if particle.Mass > maxBatchMass {
			maxBatchMass = particle.Mass
		}

		if particle.Heat > maxBatchHeat {
			maxBatchHeat = particle.Heat
		}

		if particle.Energy > maxBatchEnergy {
			maxBatchEnergy = particle.Energy
		}
	}

	return maxBatchMass, maxBatchHeat, maxBatchEnergy
}


func (solver *Solver) resetDomain(symbol string, cause error) {
	solver.recreateDomain("failed_step", time.Time{}, cause, map[string]any{
		"symbol": symbol,
	})
}

func (solver *Solver) recreateDomain(
	reason string,
	at time.Time,
	cause error,
	extra map[string]any,
) {
	if solver == nil {
		return
	}

	if solver.domain != nil {
		errnie.Error(solver.domain.Close())
	}

	domain, err := newDomain(solver.config)

	if err != nil {
		solver.domain = nil
		solver.turnover = 0

		message := "failed to recreate manifold domain"

		if reason == "failed_step" {
			message = fmt.Sprintf("failed to reset manifold domain after %s destabilized it", extra["symbol"])
		}

		errnie.Error(errnie.Err(
			errnie.Internal,
			message,
			err,
		))

		return
	}

	solver.domain = domain
	solver.turnover = 0

	payload := map[string]any{
		"stage":  "manifold",
		"status": "recreated",
		"reason": reason,
	}

	if !at.IsZero() {
		payload["at"] = at
	}

	for key, value := range extra {
		payload[key] = value
	}

	if cause != nil {
		payload["error"] = cause.Error()
		errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("reset manifold domain after failed step for %v", extra["symbol"]),
			cause,
		))
	}

	if solver.recorder != nil {
		errnie.Error(audit.Record(recorderOrNil(solver.recorder), "predictive", payload))
	}
}

func recorderOrNil(recorder *audit.Recorder) *audit.Recorder {
	return recorder
}

/*
Close releases the one resident domain and all accumulated observations.
*/
func (solver *Solver) Close() error {
	if solver == nil {
		return nil
	}

	solver.mu.Lock()
	defer solver.mu.Unlock()

	if solver.domain == nil {
		return nil
	}

	errnie.Error(solver.domain.Close())
	solver.domain = nil

	return nil
}
