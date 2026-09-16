package runtime

import (
	"github.com/theapemachine/symm/system"
	goruntime "runtime"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestNewWorkspace(t *testing.T) {
	Convey("Given a freshly initialized workspace with stages", t, func() {
		node := &countingNode{}
		workspace := NewWorkspace(
			t.Context(), "test-workspace", [][]Node[*data.Measurement[float64]]{{node}}, nil,
		)
		defer func() { So(workspace.Close(), ShouldBeNil) }()

		So(workspace.Status(), ShouldEqual, INIT)
		// Idle admission must not touch the ring at all.
		channel := workspace.channel
		workspace.channel = nil
		workspace.Step(nil)
		workspace.channel = channel
		So(node.steps, ShouldEqual, 0)
		workspace.Transition(READY)
		So(workspace.Status(), ShouldEqual, READY)

		workspace.Transition(WAITING)
		workspace.channel = nil
		workspace.Step(nil)
		workspace.channel = channel
		So(node.steps, ShouldEqual, 0)
	})

	Convey("Idle source polls do not repeat downstream observations", t, func() {
		source := &countingNode{}
		source.onStep = func(measurement *data.Measurement[float64]) *data.Measurement[float64] {
			if source.steps > 5 {
				return nil
			}

			return measurement
		}
		sink := &workspaceNode{source: "sink", observed: make(chan *data.Measurement[float64], 5)}
		workspace := NewWorkspace(t.Context(), "idle-source", [][]Node[*data.Measurement[float64]]{{source}, {sink}})
		defer func() { So(workspace.Close(), ShouldBeNil) }()
		workspace.Transition(READY)

		for poll := 0; poll < 100; poll++ {
			workspace.Step(nil)
		}

		for sequence := 1; sequence <= 5; sequence++ {
			var measurement *data.Measurement[float64]
			select {
			case measurement = <-sink.observed:
			case <-time.After(time.Second):
				t.Fatal("sink did not receive the observation")
			}
			So(measurement.SeqIdx, ShouldEqual, sequence)
			So(measurement.Peers[0].SeqIdx, ShouldEqual, sequence)
		}
	})

	Convey("Given a workspace with no handlers", t, func() {
		workspace := NewWorkspace[int](t.Context(), "test-workspace", nil, nil)
		So(workspace, ShouldBeNil)
	})

	Convey("Stage dependencies exclude concurrent and downstream registrations", t, func() {
		source := &workspaceNode{source: "source", observed: make(chan *data.Measurement[float64], 1)}
		category := &workspaceNode{source: "category", observed: make(chan *data.Measurement[float64], 1)}
		resonance := &workspaceNode{source: "resonance", observed: make(chan *data.Measurement[float64], 1)}
		learner := &workspaceNode{source: "learner", observed: make(chan *data.Measurement[float64], 1)}
		workspace := NewWorkspace(t.Context(), "stage-boundaries", [][]Node[*data.Measurement[float64]]{
			{source}, {}, {category, resonance}, {learner},
		})
		defer func() { So(workspace.Close(), ShouldBeNil) }()
		workspace.Transition(READY)

		for sequence := 1; sequence <= 3; sequence++ {
			workspace.Step(nil)

			for _, observation := range []struct {
				node     *workspaceNode
				expected []string
			}{
				{source, nil},
				{category, []string{"source"}},
				{resonance, []string{"source"}},
				{learner, []string{"source", "category", "resonance"}},
			} {
				select {
				case measurement := <-observation.node.observed:
					var sources []string

					for _, peer := range measurement.Peers {
						sources = append(sources, peer.Source)
					}

					So(sources, ShouldResemble, observation.expected)
					So(measurement.SeqIdx, ShouldEqual, sequence)
				case <-time.After(time.Second):
					t.Fatal("workspace did not complete the committed observation")
				}
			}
		}
	})
}

type workspaceNode struct {
	source   string
	observed chan *data.Measurement[float64]
}

func (node *workspaceNode) Register() *data.Measurement[float64] {
	measurement := data.NewMeasurement[float64](node.source, nil)
	measurement.Label = node.source
	measurement.Metadata["peer-interest"] = "*"
	return measurement
}

func (node *workspaceNode) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if node.observed != nil {
		node.observed <- measurement.Clone()
	}
	return measurement
}

func BenchmarkNewWorkspace(b *testing.B) {
	source := &workspaceNode{source: "source", observed: make(chan *data.Measurement[float64], 1)}
	category := &workspaceNode{source: "category", observed: make(chan *data.Measurement[float64], 1)}
	resonance := &workspaceNode{source: "resonance", observed: make(chan *data.Measurement[float64], 1)}
	learner := &workspaceNode{source: "learner", observed: make(chan *data.Measurement[float64], 1)}
	workspace := NewWorkspace(b.Context(), "stage-benchmark", [][]Node[*data.Measurement[float64]]{
		{source}, {category, resonance}, {learner},
	})
	workspace.Transition(READY)
	b.Cleanup(func() {
		if err := workspace.Close(); err != nil {
			b.Error(err)
		}
	})
	b.ReportAllocs()
	b.ResetTimer()

	// Measure completed three-stage dispatch, including test snapshot delivery.
	for b.Loop() {
		workspace.Step(nil)

		for _, node := range []*workspaceNode{source, category, resonance, learner} {
			<-node.observed
		}
	}
}

func TestWorkspaceStep(t *testing.T) {
	Convey("Admission does not wait for a stalled consumer while ring space remains", t, func() {
		entered, release := make(chan struct{}), make(chan struct{})
		defer close(release)
		node := &countingNode{}
		node.onStep = func(measurement *data.Measurement[float64]) *data.Measurement[float64] {
			if node.steps == 1 {
				close(entered)
				<-release
			}

			return measurement
		}
		tee := &workspacePublications{rows: make(chan *data.Measurement[float64], 4)}
		workspace := NewWorkspace(t.Context(), "stalled-consumer", [][]Node[*data.Measurement[float64]]{{&workspaceNode{source: "source"}}, {node}}, tee)
		defer func() { So(workspace.Close(), ShouldBeNil) }()
		workspace.Transition(READY)
		admitted := make(chan struct{})
		go func() {
			workspace.Step(nil)
			workspace.Step(nil)
			close(admitted)
		}()

		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("consumer did not start")
		}

		select {
		case measurement := <-tee.rows:
			So(measurement.Source, ShouldEqual, "source")
		case <-time.After(time.Second):
			t.Fatal("Tee publication waited for the downstream stage")
		}

		select {
		case <-admitted:
		case <-time.After(time.Second):
			t.Fatal("admission waited for consumer completion")
		}
	})

	Convey("Each consumer publishes its sequence-owned results across ring wraps", t, func() {
		original := system.Cfg.Runtime.Workspace.Buffer
		system.Cfg.Runtime.Workspace.Buffer = 8
		defer func() { system.Cfg.Runtime.Workspace.Buffer = original }()
		const observations = 128
		tee := &workspacePublications{rows: make(chan *data.Measurement[float64], 4)}
		workspace := NewWorkspace(t.Context(), "wraparound", [][]Node[*data.Measurement[float64]]{
			{&workspaceNode{source: "source"}},
			{&workspaceNode{source: "left"}, &workspaceNode{source: "right"}},
			{&workspaceNode{source: "sink"}},
		}, tee)
		defer func() { So(workspace.Close(), ShouldBeNil) }()
		workspace.Transition(READY)
		finished := make(chan struct{})
		go func() {
			defer close(finished)

			for sequence := 0; sequence < observations; sequence++ {
				workspace.Step(nil)
			}
		}()

		sequences := map[string]int64{}

		for received := 0; received < observations*4; received++ {
			select {
			case measurement := <-tee.rows:
				sequences[measurement.Source]++
				So(measurement.SeqIdx, ShouldEqual, sequences[measurement.Source])

				for _, peer := range measurement.Peers {
					So(peer.SeqIdx, ShouldEqual, measurement.SeqIdx)
				}
			case <-time.After(time.Second):
				t.Fatal("publication stalled or lost a sequence")
			}
		}

		So(sequences, ShouldResemble, map[string]int64{
			"source": observations, "left": observations, "right": observations, "sink": observations,
		})

		<-finished
	})
}

// workspacePublications observes Tee calls directly from each consumer.
type workspacePublications struct {
	rows chan *data.Measurement[float64]
}

func (tee *workspacePublications) Push(measurement *data.Measurement[float64]) {
	tee.rows <- measurement
}
func (tee *workspacePublications) Next() unsafe.Pointer { return nil }
func (tee *workspacePublications) Close() error         { return nil }

func BenchmarkWorkspaceStep(b *testing.B) {
	tee := &workspaceCounter{}
	workspace := NewWorkspace(b.Context(), "pipeline-benchmark", [][]Node[*data.Measurement[float64]]{
		{&workspaceNode{source: "source"}},
		{&workspaceNode{source: "left"}, &workspaceNode{source: "right"}},
		{&workspaceNode{source: "sink"}},
	}, tee)
	workspace.Transition(READY)
	b.Cleanup(func() {
		if err := workspace.Close(); err != nil {
			b.Fatal(err)
		}
	})
	b.ReportAllocs()
	b.ResetTimer()

	for sequence := 0; sequence < b.N; sequence++ {
		workspace.Step(nil)
	}

	for tee.count.Load() != int64(b.N)*4 {
		goruntime.Gosched()
	}
}

type workspaceCounter struct{ count atomic.Int64 }

func (tee *workspaceCounter) Push(*data.Measurement[float64]) { tee.count.Add(1) }
func (tee *workspaceCounter) Next() unsafe.Pointer            { return nil }
func (tee *workspaceCounter) Close() error                    { return nil }
