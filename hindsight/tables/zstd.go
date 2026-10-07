package tables

import (
	"io"
	"sync"

	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/klauspost/compress/zstd"
)

var zstdDecoderPool = sync.Pool{
	New: func() any {
		dec, _ := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
		return dec
	},
}

type zstdcloser struct {
	*zstd.Decoder
}

func (z *zstdcloser) Close() error {
	z.Decoder.Close()
	return nil
}

type safeZstdCodec struct {
	compress.Codec
}

func (c safeZstdCodec) Decode(dst, src []byte) []byte {
	dec := zstdDecoderPool.Get().(*zstd.Decoder)
	defer zstdDecoderPool.Put(dec)

	out, err := dec.DecodeAll(src, dst[:0])
	if err != nil {
		panic(err)
	}
	return out
}

func (c safeZstdCodec) NewReader(r io.Reader) io.ReadCloser {
	ret, _ := zstd.NewReader(r, zstd.WithDecoderConcurrency(1))
	return &zstdcloser{Decoder: ret}
}

func init() {
	orig, _ := compress.GetCodec(compress.Codecs.Zstd)
	compress.RegisterCodec(compress.Codecs.Zstd, safeZstdCodec{Codec: orig})
}
