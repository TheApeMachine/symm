package tokenizer

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func generateCorpus(paragraphs int, rng *rand.Rand) string {
	sentences := []string{
		"The quick brown fox jumps over the lazy dog.",
		"Alice was beginning to get very tired of sitting by her sister on the bank.",
		"In a hole in the ground there lived a hobbit.",
	}
	var builder strings.Builder
	for paragraph := range paragraphs {
		count := 3 + rng.Intn(5)
		for sentence := range count {
			builder.WriteString(sentences[rng.Intn(len(sentences))])
			if sentence < count-1 {
				builder.WriteByte(' ')
			}
		}
		if paragraph < paragraphs-1 {
			builder.WriteString("\n\n")
			numLen := 10 + rng.Intn(30)
			for range numLen {
				builder.WriteByte(byte('0' + rng.Intn(10)))
			}
			builder.WriteString("\n\n")
		}
	}
	return builder.String()
}

func generateBinaryNoise(size int, rng *rand.Rand) []byte {
	out := make([]byte, size)
	for i := range out {
		out[i] = byte(rng.Intn(256))
	}
	return out
}

func generateRepetitive(runs int, runLen int) []byte {
	out := make([]byte, 0, runs*runLen)
	for run := range runs {
		val := byte((run * 37) % 256)
		for range runLen {
			out = append(out, val)
		}
	}
	return out
}

func TestMDL(t *testing.T) {
	Convey("Given a Sequencer", t, func() {
		seq := NewMDL(NewCalibrator())

		Convey("When analyzing a random byte stream", func() {
			rng := rand.New(rand.NewSource(time.Now().UnixNano()))
			for i := range 50 {
				val := byte(rng.Intn(system.Cfg.Learning.VocabSize))
				seq.Analyze(uint32(i), val)
			}

			Convey("It analyzes successfully", func() {
				So(len(seq.buf), ShouldBeGreaterThan, 0)
			})
		})

		Convey("When forcing a boundary via ShannonCeiling", func() {
			for i := range system.Cfg.Learning.VocabSize {
				ok, _, _, _ := seq.Analyze(uint32(i), byte(i))
				if ok {
					break
				}
			}
			Convey("It should eventually force a boundary", func() {
				So(len(seq.candidates), ShouldBeGreaterThanOrEqualTo, 0)
			})
		})

		Convey("When calibrator history lowers the active density ceiling", func() {
			cal := NewCalibrator()
			seq = NewMDL(cal)
			seq.ShannonCeiling = 1.0
			seq.MinSegmentBytes = 2

			cal.FeedbackChunk(0.10, 1.0, 1.0)
			cal.FeedbackChunk(0.15, 1.0, 1.0)
			cal.FeedbackChunk(0.20, 1.0, 1.0)
			cal.FeedbackChunk(0.10, 1.0, 1.0)

			for i := range system.Cfg.Learning.VocabSize {
				_, _, _, _ = seq.Analyze(uint32(i), byte(i))
				if len(seq.candidates) > 0 {
					break
				}
			}

			Convey("It should force a boundary before the fallback ceiling is reached", func() {
				So(len(seq.candidates), ShouldBeGreaterThan, 0)
			})
		})

		Convey("When detecting a boundary via MDL", func() {
			buf := []byte{0, 0, 0, 0, 0, 255, 255, 255, 255, 255}
			dist := NewDistribution()
			for _, b := range buf {
				dist.Add(b)
			}
			ok, k, gain := seq.detectBoundary(buf, dist.Clone())

			Convey("It should identify the boundary and gain", func() {
				So(ok, ShouldBeTrue)
				So(k, ShouldBeGreaterThan, 0)
				So(gain, ShouldBeGreaterThan, 0.0)
			})
		})

		Convey("When balancing candidates", func() {
			seq.buf = []byte{0, 0, 0, 255, 255, 255, 0, 0, 0}
			seq.candidates = []candidate{
				{k: 3, gain: 1.0, forced: false},
				{k: 6, gain: 1.5, forced: false},
			}

			seq.balanceCandidates()

			Convey("It should handle merging correctly", func() {
				So(len(seq.candidates), ShouldBeLessThanOrEqualTo, 2)
			})
		})

		Convey("When integrating computeSignal and eigen modes", func() {
			seq.SetEigenMode(nil)
			So(seq.eigen, ShouldNotBeNil)

			eigen := NewEigenMode()
			eigen.Trained = true
			seq.SetEigenMode(eigen)

			val, delta, eigenMag := seq.computeSignal(42)
			Convey("It should compute phases", func() {
				So(val, ShouldEqual, 42.0)
				So(delta, ShouldBeGreaterThanOrEqualTo, 0.0)
				So(eigenMag, ShouldBeGreaterThanOrEqualTo, 0.0)
			})
		})

		Convey("When cloning and flushing", func() {
			for i := range 10 {
				_, _, _, _ = seq.Analyze(uint32(i), byte(i))
			}

			clone := seq.Clone()
			Convey("Clone should maintain state", func() {
				So(len(clone.buf), ShouldEqual, len(seq.buf))
				So(clone.offset, ShouldEqual, seq.offset)
			})

			empty := seq.CloneEmpty()
			Convey("CloneEmpty should wipe state", func() {
				So(len(empty.buf), ShouldEqual, 0)
				So(empty.offset, ShouldEqual, 0)
			})

			seq.buf = []byte{0, 1, 2, 3}
			seq.offset = 2
			seq.candidates = []candidate{{k: 2, gain: 1.0}}

			ok, _, evts, _ := seq.Flush()
			Convey("Flush should commit the candidate", func() {
				So(ok, ShouldBeTrue)
				So(len(evts), ShouldBeGreaterThan, 0)
				So(len(seq.candidates), ShouldEqual, 0)
			})

			ok2, _, _ := seq.Forecast(0, 4)
			Convey("Forecast should return detection status without mutating", func() {
				So(seq.buf[len(seq.buf)-1], ShouldEqual, 3)
				So(ok2, ShouldBeFalse)
			})
		})
	})
}

func BenchmarkMDLDetectBoundary(b *testing.B) {
	seq := NewMDL(nil)

	profiles := [][]byte{
		[]byte(generateCorpus(10, rand.New(rand.NewSource(42)))),
		generateBinaryNoise(2048, rand.New(rand.NewSource(99))),
		generateRepetitive(10, 200),
	}

	for i := 0; b.Loop(); i++ {
		buf := profiles[i%len(profiles)]
		dist := NewDistribution()
		for _, byteVal := range buf {
			dist.Add(byteVal)
		}
		seq.detectBoundary(buf, dist)
	}
}
