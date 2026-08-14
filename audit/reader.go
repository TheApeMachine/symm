package audit

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/klauspost/compress/zstd"
)

/* Reader streams plain or Zstandard-compressed recorder output. */
type Reader struct {
	file    *os.File
	decoder *zstd.Decoder
	source  io.Reader
}

/* NewReader opens recorder output without loading the capture into memory. */
func NewReader(filename string) (*Reader, error) {
	file, err := os.Open(filename)

	if err != nil {
		return nil, err
	}

	reader := &Reader{file: file, source: file}

	if !strings.HasSuffix(filename, ".zst") {
		return reader, nil
	}

	reader.decoder, err = zstd.NewReader(file)

	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("audit: open zstd reader: %w", err)
	}

	reader.source = reader.decoder
	return reader, nil
}

/* Read streams the next decoded recorder bytes. */
func (reader *Reader) Read(buffer []byte) (int, error) {
	return reader.source.Read(buffer)
}

/* Close releases the decoder and capture file. */
func (reader *Reader) Close() error {
	if reader.decoder != nil {
		reader.decoder.Close()
	}

	return reader.file.Close()
}
