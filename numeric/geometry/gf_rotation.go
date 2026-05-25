package geometry

/*
GFRotation is a pair of affine coordinates in GF(257).
Used as the address type for nearest-neighbor kernel dispatch.
*/
type GFRotation struct {
	CoordU uint16
	CoordV uint16
}
