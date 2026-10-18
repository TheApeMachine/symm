package transport

import (
	"io"
)

/*
NewFlipFlop creates a bidirectional data flow between two ReadWriters.
It first copies data from the source to the destination, then copies
any response back from the destination to the source. This creates
a complete round-trip data exchange.
*/
type FlipFlop struct {
	from io.ReadWriter
	to   io.ReadWriter
	done bool
}

func NewFlipFlop(from, to io.ReadWriter) *FlipFlop {
	return &FlipFlop{from: from, to: to}
}

func (flipflop *FlipFlop) Read(p []byte) (int, error) {
	if err := flipflop.exchange(); err != nil {
		return 0, err
	}

	return flipflop.from.Read(p)
}

func (flipflop *FlipFlop) Write(p []byte) (int, error) {
	flipflop.done = false
	return flipflop.from.Write(p)
}

func (flipflop *FlipFlop) WriteTo(w io.Writer) (int64, error) {
	if err := flipflop.exchange(); err != nil {
		return 0, err
	}

	if writerTo, ok := flipflop.from.(io.WriterTo); ok {
		return writerTo.WriteTo(w)
	}

	n, err := Copy(w, flipflop.from)
	return int64(n), err
}

func (flipflop *FlipFlop) exchange() error {
	if flipflop.done {
		return nil
	}

	if _, err := Copy(flipflop.to, flipflop.from); err != nil && err != io.EOF {
		return err
	}

	if _, err := Copy(flipflop.from, flipflop.to); err != nil && err != io.EOF {
		return err
	}

	flipflop.done = true
	return nil
}
