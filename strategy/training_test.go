package strategy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/tests/market"
)

func TestTrainingRegister(t *testing.T) {
	Convey("Training owns one registered publication", t, func() {
		training := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)
		So(training.Register(), ShouldEqual, training.Register())
	})
}

func TestTrainingStep(t *testing.T) {
	Convey("Given a Training instance in INIT stage", t, func() {
		training := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)
		So(training.Status(), ShouldEqual, runtime.INIT)

		Convey("When observations arrive the existing grid assigns coordinates", func() {
			frame := data.NewMeasurement[float64]("fixture", nil)
			frame.Label = "BTC/USD"
			frame.Metrics = map[string]data.Metric[float64]{
				"price":  {Label: "price", Raw: 50000},
				"volume": {Label: "volume", Raw: 12.5},
			}
			output := training.Step(frame)
			So(output, ShouldNotBeNil)
			So(output.Metrics["price"].Region, ShouldBeGreaterThan, 0)
			So(output.Metrics["volume"].Region, ShouldBeGreaterThan, 0)

			// Observing a market state without an outcome must not teach WAIT.
			So(training.trie.Tree().Len(), ShouldEqual, 0)
			So(output.Provenance["prediction_status"], ShouldEqual, "unseen")

			Convey("Repeating unlabelled observations still creates no action evidence", func() {
				for tick := 0; tick < 15; tick++ {
					repeated := data.NewMeasurement[float64]("fixture", nil)
					repeated.Label = "BTC/USD"
					repeated.Metrics = map[string]data.Metric[float64]{
						"price":  {Label: "price", Raw: 50000},
						"volume": {Label: "volume", Raw: 12.5},
					}
					training.Step(repeated)
				}

				So(training.trie.Tree().Len(), ShouldEqual, 0)
			})
		})

		Convey("An explicitly stored association is read without creating another", func() {
			training.Transition(runtime.READY)
			training.trie.Insert([]byte{1}, []byte(ActionEnter))
			frame := data.NewMeasurement[float64]("fixture", nil)
			frame.Label = "BTC/USD"
			frame.Metrics["price"] = data.Metric[float64]{Label: "price", Raw: 50000, Region: 1}
			output := training.Step(frame)
			So(output.Metrics["action"].Raw, ShouldEqual, 1)
			So(output.Provenance["prediction_status"], ShouldEqual, "stored association")
			So(training.trie.Tree().Len(), ShouldEqual, 1)
		})
	})
}

func checkpointDirectory(t *testing.T) {
	t.Helper()
	original, err := os.Getwd()

	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Error(err)
		}
	})
}

func TestTrainingCheckpoint(t *testing.T) {
	checkpointDirectory(t)
	Convey("The grid and exact binary trie restore as one complete model", t, func() {
		training := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)
		training.grid.Add(&data.Metric[float64]{Label: "fixture", Raw: 12.5, X: -4, Y: 7, Region: 3})
		training.grid.Settled = true

		for index := 0; index < 256; index++ {
			training.trie.Insert([]byte{byte(index), 0, 255}, []byte(fmt.Sprint(index)))
		}

		So(training.SaveCheckpoint(), ShouldBeNil)
		before, err := os.ReadFile("grid_checkpoint.json")
		So(err, ShouldBeNil)
		restored := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)
		So(restored.LoadCheckpoint(), ShouldBeNil)
		So(restored.grid.Metrics, ShouldResemble, training.grid.Metrics)
		So(restored.grid.Settled, ShouldBeTrue)
		So(restored.trie.Tree().Len(), ShouldEqual, 256)
		So(restored.Status(), ShouldEqual, runtime.INIT)

		for index := 0; index < 256; index++ {
			value, found := restored.trie.Get([]byte{byte(index), 0, 255})
			So(found, ShouldBeTrue)
			So(string(value), ShouldEqual, fmt.Sprint(index))
		}

		So(restored.SaveCheckpoint(), ShouldBeNil)
		after, err := os.ReadFile("grid_checkpoint.json")
		So(err, ShouldBeNil)
		So(bytes.Equal(before, after), ShouldBeTrue)
		temporaryFiles, err := filepath.Glob(".symm-checkpoint-*")
		So(err, ShouldBeNil)
		So(temporaryFiles, ShouldBeEmpty)

		Convey("A corrupt trie cannot replace the grid or partially restore the model", func() {
			priorGrid, priorTrie := restored.grid, restored.trie
			var document map[string]json.RawMessage
			So(json.Unmarshal(before, &document), ShouldBeNil)
			document["grid"] = json.RawMessage(`{"metrics":[],"settled":false}`)
			document["trie"] = json.RawMessage(`{"\ufffd":"d2FpdA=="}`)
			broken, err := json.Marshal(document)
			So(err, ShouldBeNil)
			So(os.WriteFile("grid_checkpoint.json", broken, 0600), ShouldBeNil)
			So(restored.LoadCheckpoint(), ShouldNotBeNil)
			So(restored.grid, ShouldEqual, priorGrid)
			So(restored.trie, ShouldEqual, priorTrie)
			So(restored.trie.Tree().Len(), ShouldEqual, 256)
		})

		Convey("A missing grid is rejected rather than silently restored as an empty model", func() {
			priorGrid, priorTrie := restored.grid, restored.trie
			var document map[string]json.RawMessage
			So(json.Unmarshal(before, &document), ShouldBeNil)
			delete(document, "grid")
			broken, err := json.Marshal(document)
			So(err, ShouldBeNil)
			So(os.WriteFile("grid_checkpoint.json", broken, 0600), ShouldBeNil)
			So(restored.LoadCheckpoint(), ShouldNotBeNil)
			So(restored.grid, ShouldEqual, priorGrid)
			So(restored.trie, ShouldEqual, priorTrie)
		})
	})
}

func TestTrainingCheckpointConcurrent(t *testing.T) {
	checkpointDirectory(t)
	Convey("Live model updates and complete checkpoint snapshots share one model lock", t, func() {
		training := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)
		var workers sync.WaitGroup
		failures := make(chan error, 1)
		workers.Add(1)
		go func() {
			defer workers.Done()

			for index := 0; index < 20; index++ {
				if err := training.SaveCheckpoint(); err != nil {
					failures <- err
					return
				}
			}
		}()

		for index := 0; index < 100; index++ {
			frame := data.NewMeasurement[float64]("fixture", nil)
			frame.Label = "BTC/USD"
			frame.Metrics["value"] = data.Metric[float64]{Label: "value", Raw: float64(index)}
			training.Step(frame)
			training.CognitionTree()
		}

		workers.Wait()
		close(failures)

		for err := range failures {
			So(err, ShouldBeNil)
		}

		restored := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)
		So(restored.LoadCheckpoint(), ShouldBeNil)
		So(restored.trie.Tree().Len(), ShouldEqual, 0)
	})
}
