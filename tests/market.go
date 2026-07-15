package tests

import "iter"

/*
Market composes fixture streams into one replayable timeline.
Prefix frames emit in full before the round-robin rotation begins.
*/
type Market struct {
	prefix         []iter.Seq[Frame]
	prefixPayloads []iter.Seq[[]byte]
	streams        []iter.Seq[Frame]
}

/*
NewMarket creates an empty market timeline.
*/
func NewMarket() *Market {
	return &Market{}
}

/*
Prefix appends a stream that fully emits before round-robin begins.
*/
func (market *Market) Prefix(fixture Fixture) *Market {
	market.prefix = append(market.prefix, fixture.Frames())

	return market
}

/*
Feed joins a fixture stream to the round-robin rotation.
*/
func (market *Market) Feed(fixture Fixture) *Market {
	market.streams = append(market.streams, fixture.Frames())

	return market
}

/*
PrefixPayload appends one ordered payload stream before channel round-robin.
Use this for private RPC acknowledgements that do not carry a channel envelope.
*/
func (market *Market) PrefixPayload(fixture PayloadFixture) *Market {
	market.prefixPayloads = append(market.prefixPayloads, fixture.Generate())

	return market
}

/*
Frames yields prefix streams first, then round-robin updates.
*/
func (market *Market) Frames() iter.Seq[Frame] {
	return func(yield func(Frame) bool) {
		for _, prefix := range market.prefix {
			for frame := range prefix {
				if !yield(frame) {
					return
				}
			}
		}

		for _, prefix := range market.prefixPayloads {
			for payload := range prefix {
				frame, ok := frameFromPayload(payload)

				if !ok {
					continue
				}

				if !yield(frame) {
					return
				}
			}
		}

		for frame := range RoundRobin(market.streams...) {
			if !yield(frame) {
				return
			}
		}
	}
}

/*
Replay drives handlers through the composed market timeline.
*/
func (market *Market) Replay(handlers Handlers) {
	Replay(handlers, market.Frames())
}

/*
RoundRobin interleaves frame streams one frame at a time.
*/
func RoundRobin(streams ...iter.Seq[Frame]) iter.Seq[Frame] {
	return func(yield func(Frame) bool) {
		if len(streams) == 0 {
			return
		}

		pulls := make([]streamPull, len(streams))

		for index, stream := range streams {
			next, stop := iter.Pull(stream)
			pulls[index] = streamPull{next: next, stop: stop, live: true}
		}

		defer func() {
			for _, pull := range pulls {
				pull.stop()
			}
		}()

		alive := len(pulls)
		index := 0

		for alive > 0 {
			pull := &pulls[index]

			if !pull.live {
				index = (index + 1) % len(pulls)
				continue
			}

			frame, ok := pull.next()

			if !ok {
				pull.live = false
				alive--
				index = (index + 1) % len(pulls)

				continue
			}

			if !yield(frame) {
				return
			}

			index = (index + 1) % len(pulls)
		}
	}
}

type streamPull struct {
	next func() (Frame, bool)
	stop func()
	live bool
}
