package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRunID(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a RunID is constructed", func() {
			id := RunID("run-abc")

			Convey("it retains the opaque string", func() {
				So(string(id), ShouldEqual, "run-abc")
			})
		})
	})
}

func TestStream(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a Stream is constructed", func() {
			s := Stream("spot.public")

			Convey("it retains the logical name", func() {
				So(string(s), ShouldEqual, "spot.public")
			})
		})
	})
}

func TestStreamEpoch(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a StreamEpoch is constructed", func() {
			epoch := StreamEpoch(3)

			Convey("it retains the connection span", func() {
				So(uint64(epoch), ShouldEqual, uint64(3))
			})
		})
	})
}

func TestStreamRef(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a StreamRef is constructed", func() {
			ref := StreamRef{
				Stream:   Stream("spot.public"),
				Epoch:    StreamEpoch(2),
				Sequence: 81,
			}

			Convey("stream, epoch, and sequence are exact", func() {
				So(string(ref.Stream), ShouldEqual, "spot.public")
				So(uint64(ref.Epoch), ShouldEqual, uint64(2))
				So(ref.Sequence, ShouldEqual, uint64(81))
			})
		})
	})
}

func TestCaptureSequence(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a CaptureSequence is constructed", func() {
			seq := CaptureSequence(42)

			Convey("it retains the run-local order", func() {
				So(uint64(seq), ShouldEqual, uint64(42))
			})
		})
	})
}

func TestCaptureIdentity(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a CaptureIdentity is constructed", func() {
			id := CaptureIdentity{
				Run:            RunID("run-1"),
				Sequence:       CaptureSequence(7),
				Stream:         Stream("spot.private"),
				StreamEpoch:    StreamEpoch(1),
				StreamSequence: 99,
			}

			Convey("run, sequence, and stream identity are exact", func() {
				So(string(id.Run), ShouldEqual, "run-1")
				So(uint64(id.Sequence), ShouldEqual, uint64(7))
				So(string(id.Stream), ShouldEqual, "spot.private")
				So(uint64(id.StreamEpoch), ShouldEqual, uint64(1))
				So(id.StreamSequence, ShouldEqual, uint64(99))
			})
		})
	})
}

func TestEnvelopeRef(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When an EnvelopeRef is constructed", func() {
			ref := EnvelopeRef{
				Origin: CaptureIdentity{
					Run:            RunID("run-2"),
					Sequence:       CaptureSequence(1),
					Stream:         Stream("level3"),
					StreamEpoch:    StreamEpoch(0),
					StreamSequence: 5,
				},
				Ordinal: 3,
			}

			Convey("origin and ordinal are exact", func() {
				So(string(ref.Origin.Run), ShouldEqual, "run-2")
				So(uint64(ref.Origin.Sequence), ShouldEqual, uint64(1))
				So(string(ref.Origin.Stream), ShouldEqual, "level3")
				So(uint64(ref.Origin.StreamEpoch), ShouldEqual, uint64(0))
				So(ref.Origin.StreamSequence, ShouldEqual, uint64(5))
				So(ref.Ordinal, ShouldEqual, uint64(3))
			})
		})
	})
}

func TestArtifactID(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When an ArtifactID is constructed", func() {
			id := ArtifactID{
				Kind:     "fingerprint",
				Identity: "fp-001",
			}

			Convey("kind and identity are exact", func() {
				So(id.Kind, ShouldEqual, "fingerprint")
				So(id.Identity, ShouldEqual, "fp-001")
			})
		})
	})
}

func TestStateVersion(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a StateVersion is constructed", func() {
			v := StateVersion{
				Component: "resonance",
				Key:       "BTC/USD",
				Version:   12,
			}

			Convey("component, key, and version are exact", func() {
				So(v.Component, ShouldEqual, "resonance")
				So(v.Key, ShouldEqual, "BTC/USD")
				So(v.Version, ShouldEqual, uint64(12))
			})
		})
	})
}

func TestComponentState(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a ComponentState is constructed", func() {
			s := ComponentState{
				Component: "manifold",
				Key:       "ETH/USD",
				Version:   4,
			}

			Convey("component, key, and version are exact", func() {
				So(s.Component, ShouldEqual, "manifold")
				So(s.Key, ShouldEqual, "ETH/USD")
				So(s.Version, ShouldEqual, uint64(4))
			})
		})
	})
}

func BenchmarkRunID(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = string(RunID("run-abc"))
	}
}

func BenchmarkStream(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = string(Stream("spot.public"))
	}
}

func BenchmarkStreamEpoch(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = uint64(StreamEpoch(3))
	}
}

func BenchmarkStreamRef(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		ref := StreamRef{Stream: Stream("spot.public"), Epoch: StreamEpoch(2), Sequence: 81}
		_ = ref.Sequence
	}
}

func BenchmarkCaptureSequence(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = uint64(CaptureSequence(42))
	}
}

func BenchmarkCaptureIdentity(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		id := CaptureIdentity{
			Run: RunID("run-1"), Sequence: CaptureSequence(7),
			Stream: Stream("spot.private"), StreamEpoch: StreamEpoch(1), StreamSequence: 99,
		}
		_ = id.StreamSequence
	}
}

func BenchmarkEnvelopeRef(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		ref := EnvelopeRef{
			Origin: CaptureIdentity{
				Run: RunID("run-2"), Sequence: CaptureSequence(1),
				Stream: Stream("level3"), StreamEpoch: StreamEpoch(0), StreamSequence: 5,
			},
			Ordinal: 3,
		}
		_ = ref.Ordinal
	}
}

func BenchmarkArtifactID(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		id := ArtifactID{Kind: "fingerprint", Identity: "fp-001"}
		_ = id.Identity
	}
}

func BenchmarkStateVersion(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		v := StateVersion{Component: "resonance", Key: "BTC/USD", Version: 12}
		_ = v.Version
	}
}

func BenchmarkComponentState(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		s := ComponentState{Component: "manifold", Key: "ETH/USD", Version: 4}
		_ = s.Version
	}
}
