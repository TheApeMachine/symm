package tables

import (
	"io"
	"sync"

	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/klauspost/compress/zstd"
)

var zstdDecoderPool = sync.Pool{
	New: func() any {
		dec, _ := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.IgnoreChecksum(true))
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

func (c safeZstdCodec) Decode(dst, src []byte) (out []byte) {
	dec := zstdDecoderPool.Get().(*zstd.Decoder)

	var (
		err      error
		panicked any
	)

	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = r
			}
		}()
		out, err = dec.DecodeAll(src, dst[:0])
	}()

	if panicked != nil || err != nil {
		dec.Close()
		fresh, freshErr := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.IgnoreChecksum(true))
		if freshErr == nil {
			defer fresh.Close()
			out, err = fresh.DecodeAll(src, dst[:0])
			if err == nil {
				return out
			}
		}
		if panicked != nil {
			panic(panicked)
		}
		panic(err)
	}

	zstdDecoderPool.Put(dec)
	return out
}

func (c safeZstdCodec) NewReader(r io.Reader) io.ReadCloser {
	ret, _ := zstd.NewReader(r, zstd.WithDecoderConcurrency(1), zstd.IgnoreChecksum(true))
	return &zstdcloser{Decoder: ret}
}

func init() {
	orig, _ := compress.GetCodec(compress.Codecs.Zstd)
	compress.RegisterCodec(compress.Codecs.Zstd, safeZstdCodec{Codec: orig})
}
