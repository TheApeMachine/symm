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

func (codec safeZstdCodec) Decode(dst, src []byte) (out []byte) {
	dec := zstdDecoderPool.Get().(*zstd.Decoder)

	var (
		err      error
		panicked any
	)

	func() {
		defer func() {
			if recovery := recover(); recovery != nil {
				panicked = recovery
			}
		}()

		out, err = dec.DecodeAll(src, dst[:0])
	}()

	if panicked == nil && err == nil {
		zstdDecoderPool.Put(dec)
		return out
	}

	dec.Close()

	fresh, freshErr := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.IgnoreChecksum(true))
	if freshErr != nil {
		return nil
	}
	defer fresh.Close()

	func() {
		defer func() {
			if recovery := recover(); recovery != nil {
				out = nil
			}
		}()

		res, decErr := fresh.DecodeAll(src, nil)
		if decErr != nil {
			out = nil
			return
		}

		if len(dst) >= len(res) {
			copy(dst, res)
			out = dst[:len(res)]
			return
		}

		out = res
	}()

	return out
}

func (codec safeZstdCodec) NewReader(reader io.Reader) io.ReadCloser {
	ret, _ := zstd.NewReader(reader, zstd.WithDecoderConcurrency(1), zstd.IgnoreChecksum(true))
	return &zstdcloser{Decoder: ret}
}

func init() {
	orig, _ := compress.GetCodec(compress.Codecs.Zstd)
	compress.RegisterCodec(compress.Codecs.Zstd, safeZstdCodec{Codec: orig})
}
