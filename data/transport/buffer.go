package transport

import (
	"bytes"
)

// testBuffer wraps bytes.Buffer to implement io.ReadWriteCloser
// and simulates real-world stream behavior where reading empties the buffer
type testBuffer struct {
	*bytes.Buffer
	readData []byte // Stores data that has been read
}

func newTestBuffer(data []byte) *testBuffer {
	return &testBuffer{
		Buffer: bytes.NewBuffer(data),
	}
}

// Read overrides bytes.Buffer.Read to simulate stream behavior
func (b *testBuffer) Read(p []byte) (n int, err error) {
	n, err = b.Buffer.Read(p)
	if n > 0 {
		b.readData = append(b.readData, p[:n]...)
	}
	return n, err
}

// Write overrides bytes.Buffer.Write to simulate stream behavior
func (b *testBuffer) Write(p []byte) (n int, err error) {
	b.readData = nil // Reset read data when writing
	return b.Buffer.Write(p)
}

func (b *testBuffer) Close() error {
	return nil
}

// String returns the current buffer contents
func (b *testBuffer) String() string {
	return b.Buffer.String()
}

// ReadString returns the data that has been read
func (b *testBuffer) ReadString() string {
	return string(b.readData)
}
