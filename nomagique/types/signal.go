package types

// MaxChannels is how many independent lanes one Signal carries.
const MaxChannels = 16

/*
Signal is the unboxed multi-channel carrier every node reads from and writes
to.

A single scalar wire cannot express a real measurement: a stage that consumed
one value and emitted another destroyed everything upstream of it, so a
terminal stage saw only the last transformation and every earlier quantity had
to be fetched out of band. Carrying independent lanes means each stage writes
what it computed to its own channel and nothing it did not touch is disturbed.

It is a fixed array rather than a map or a slice so a Signal is stack
allocated and passed by pointer between stages at zero heap cost. Channels are
plain indices: a composition names them with its own constants, so nothing
here knows what any lane means.

Time is the instant this observation was made, in seconds, and is what a
temporal stage measures against. Active marks which lanes have been written,
so an untouched channel is distinguishable from one deliberately set to zero.
*/
type Signal struct {
	Channels [MaxChannels]Scalar
	Time     float64
	Active   uint16
}

// Get reads one channel. An index outside the carrier reads as zero.
func (signal *Signal) Get(channel int) Scalar {
	if channel < 0 || channel >= MaxChannels {
		return 0
	}

	return signal.Channels[channel]
}

// Set writes one channel and marks it active.
func (signal *Signal) Set(channel int, value Scalar) {
	if channel < 0 || channel >= MaxChannels {
		return
	}

	signal.Channels[channel] = value
	signal.Active |= 1 << uint(channel)
}

// Written reports whether a channel has been written this observation.
func (signal *Signal) Written(channel int) bool {
	if channel < 0 || channel >= MaxChannels {
		return false
	}

	return signal.Active&(1<<uint(channel)) != 0
}

// Reset clears every lane so one Signal can carry the next observation.
func (signal *Signal) Reset() {
	signal.Channels = [MaxChannels]Scalar{}
	signal.Active = 0
	signal.Time = 0
}
