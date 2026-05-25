package numeric

import "math/bits"

func XOR(a, b uint64) uint64 {
	return a ^ b
}

func AND(a, b uint64) uint64 {
	return a & b
}

func OR(a, b uint64) uint64 {
	return a | b
}

func NOT(a uint64) uint64 {
	return ^a
}

func SHIFTLEFT(a uint64, b int) uint64 {
	return a << b
}

func SHIFTRIGHT(a uint64, b int) uint64 {
	return a >> b
}

func ROTATELEFT(a uint64, b int) uint64 {
	return bits.RotateLeft64(a, b)
}

func ROTATERIGHT(a uint64, b int) uint64 {
	return bits.RotateLeft64(a, -b)
}
