package store

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
)

/*
	ManifoldServer owns the resident physical domain and its immutable published

frame. Ingress retains the latest full queue per market while the GPU advances;
it never waits for the physical integrator. Coalescing is valid because each
queue describes all currently resting orders, including explicit departures.
*/
type ManifoldServer struct {
	mu              sync.Mutex
	markets         map[string]manifoldInput
	pending         map[string]manifoldInput
	wake            chan struct{}
	stop            chan struct{}
	finished        chan struct{}
	started         bool
	closed          bool
	grid            [3]uint32
	epoch, sequence int64
	frame           ManifoldFrame
	delivered       uint64
	err             error
	physics         *sensorium.Manifold
	resident        map[string]*manifoldMarket
	nextID          int64
}

/* NewManifold constructs an idle native owner; Write activates its GPU worker. */
func NewManifold() *ManifoldServer {
	return &ManifoldServer{markets: map[string]manifoldInput{}, pending: map[string]manifoldInput{},
		resident: map[string]*manifoldMarket{}, wake: make(chan struct{}, 1),
		stop: make(chan struct{}), finished: make(chan struct{})}
}

/* Write records current native inputs, preserving absent excitation values. */
func (server *ManifoldServer) Write(ctx context.Context, call Manifold_write) error {
	args := call.Args()
	symbol, err := args.Symbol()

	if err != nil {
		return errnie.Error(err)
	}
	if symbol == "" || args.Epoch() < 0 || args.Sequence() < 0 || args.GridX() == 0 || args.GridY() == 0 || args.GridZ() == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "manifold: symbol, causal stamp and positive grid dimensions required", nil))
	}
	server.mu.Lock()
	defer server.mu.Unlock()

	if server.err != nil {
		return errnie.Error(server.err)
	}
	if server.closed {
		return errnie.Error(errnie.Err(errnie.Validation, "manifold: owner is closed", nil))
	}
	grid := [3]uint32{args.GridX(), args.GridY(), args.GridZ()}

	if server.started && (grid != server.grid || args.Epoch() != server.epoch) {
		return errnie.Error(errnie.Err(errnie.Validation, "manifold: a physical domain cannot change grid or epoch during its lifetime", nil))
	}
	if server.started && args.Sequence() < server.sequence {
		return errnie.Error(errnie.Err(errnie.Validation, "manifold: observation sequence regressed", nil))
	}
	input := server.markets[symbol]
	input.symbol, input.epoch, input.sequence = symbol, args.Epoch(), args.Sequence()
	changed, err := input.read(args)

	if err != nil {
		return errnie.Error(err)
	}
	server.markets[symbol] = input
	server.epoch, server.sequence = args.Epoch(), args.Sequence()

	if !server.started {
		server.grid = grid
	}

	if !changed || !input.book || !input.known[0] || !input.known[1] {
		return nil
	}
	server.pending[symbol] = input

	if !server.started {
		server.started = true
		go server.run()
	}
	select {
	case server.wake <- struct{}{}:
	default:
	}
	return nil
}

/*
	Done publishes actual completed physics, retaining its own causal stamp. The

large immutable frame is emitted once per version; scalar cuts remain available.
*/
func (server *ManifoldServer) Done(ctx context.Context, call Manifold_done) error {
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	server.mu.Lock()
	defer server.mu.Unlock()

	if server.err != nil {
		return errnie.Error(server.err)
	}
	values, err := result.NewValues(6)

	if err != nil {
		return errnie.Error(err)
	}
	present, err := result.NewPresent(6)

	if err != nil {
		return errnie.Error(err)
	}
	if !server.frame.IsValid() {
		return nil
	}
	frame := server.frame
	for index, value := range []float64{frame.Divergence(), frame.GuidanceSpeed(), frame.Coherence(), frame.PressureGradient(), frame.Viscosity(), frame.Synchronization()} {
		values.Set(index, value)
		present.Set(index, frame.Population() > 0)
	}
	result.SetEpoch(frame.Epoch())
	result.SetSequence(frame.Sequence())

	if frame.Version() == server.delivered {
		return nil
	}
	if err := result.SetFrame(frame); err != nil {
		return errnie.Error(err)
	}
	server.delivered = frame.Version()
	return nil
}

/* Shutdown joins the sole GPU owner before releasing its retained frame. */
func (server *ManifoldServer) Shutdown() {
	server.mu.Lock()

	if server.closed {
		server.mu.Unlock()
		return
	}
	server.closed = true
	started := server.started
	close(server.stop)
	server.mu.Unlock()

	if started {
		<-server.finished
	}
	if server.frame.IsValid() {
		server.frame.Message().Release()
	}
}

/* run processes pending snapshots in causal order across markets. */
func (server *ManifoldServer) run() {
	defer close(server.finished)
	server.physics = sensorium.NewManifold(int(server.grid[0]), int(server.grid[1]), int(server.grid[2]))

	if server.physics == nil {
		server.fail(errnie.Err(errnie.Internal, "manifold: initialize Sensorium", nil))
		return
	}
	defer func() {
		if err := server.physics.Close(); err != nil {
			server.fail(err)
		}
	}()
	for {
		select {
		case <-server.stop:
			return
		case <-server.wake:
		}
		for {
			input, available := server.next()

			if !available {
				break
			}
			if err := server.advance(input); err != nil {
				server.fail(err)
				return
			}
			select {
			case <-server.stop:
				return
			default:
			}
		}
	}
}

func (server *ManifoldServer) next() (manifoldInput, bool) {
	server.mu.Lock()
	defer server.mu.Unlock()
	var selected manifoldInput
	for _, input := range server.pending {
		if selected.symbol == "" || input.sequence < selected.sequence {
			selected = input
		}
	}
	delete(server.pending, selected.symbol)
	return selected, selected.symbol != ""
}

func (server *ManifoldServer) fail(err error) {
	server.mu.Lock()
	defer server.mu.Unlock()
	server.err = errnie.Error(errnie.Err(errnie.Internal, "manifold: physical integration failed", err))
}

/* publish transfers the immutable frame to the native retained owner. */
func (server *ManifoldServer) publish(frame ManifoldFrame) {
	server.mu.Lock()
	defer server.mu.Unlock()

	if server.frame.IsValid() {
		frame.SetVersion(server.frame.Version() + 1)
		server.frame.Message().Release()
	}
	if frame.Version() == 0 {
		frame.SetVersion(1)
	}
	server.frame = frame
}
