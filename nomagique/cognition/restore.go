package cognition

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"io"
	"iter"
	"math"
	"sync/atomic"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Restore replaces an empty association trie with a snapshot read from the
text key "model". A populated trie is left untouched.
*/
type Restore struct {
	*core.PrimitiveError
	memory *Associate
}

func NewRestore(memory *Associate) *Restore {
	return &Restore{
		PrimitiveError: core.NewPrimitiveError(),
		memory:         memory,
	}
}

func (op *Restore) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil || op.memory == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			current := op.memory.root.Load()

			if current == nil || current.Len() != 0 {
				op.Error(core.ErrDomain)
				return
			}

			var text data.Map[string]

			for pointer := range adapter.Next(data.NewValue(data.NewLiteral("model"))) {
				text = *(*data.Map[string])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			model, held := text.Values["model"]

			if !held || model == "" {
				op.Error(core.ErrNotHeld)
				return
			}

			decoder := gob.NewDecoder(bytes.NewReader([]byte(model)))
			var format string
			var count int

			if err := decoder.Decode(&format); err != nil {
				op.Error(err)
				return
			}

			if format != "cognition/association/1" {
				op.Error(core.ErrDomain)
				return
			}

			if err := decoder.Decode(&count); err != nil {
				op.Error(err)
				return
			}

			if count < 0 {
				op.Error(core.ErrShape)
				return
			}

			transaction := iradix.New[[]byte]().Txn()

			for range count {
				var key, value []byte

				if err := decoder.Decode(&key); err != nil {
					op.Error(err)
					return
				}

				if err := decoder.Decode(&value); err != nil {
					op.Error(err)
					return
				}

				book := len(key) == 1 && (key[0] == 0 || key[0] == 1)

				if book && len(value) != 8 {
					op.Error(core.ErrShape)
					return
				}

				if !book && len(value) != 24 {
					op.Error(core.ErrShape)
					return
				}

				sensory := len(key) > 2 && key[0] == 's' && key[1] == '/'
				basin := false

				if !book && len(key) >= 4 && key[0] == 'b' && key[1] == '/' {
					rest := key[2:]
					slash := bytes.LastIndexByte(rest, '/')
					basin = slash > 0 && slash < len(rest)-1
				}

				if !book && !sensory && !basin {
					op.Error(core.ErrShape)
					return
				}

				if !book {
					weightCount := binary.LittleEndian.Uint64(value[0:8])
					probability := math.Float64frombits(binary.LittleEndian.Uint64(value[8:16]))

					if weightCount == 0 || probability < 0 || probability > 1 {
						op.Error(core.ErrDomain)
						return
					}
				}

				if _, replaced := transaction.Insert(key, value); replaced {
					op.Error(core.ErrShape)
					return
				}
			}

			var trailing any
			err := decoder.Decode(&trailing)

			if !errors.Is(err, io.EOF) {
				op.Error(core.ErrDomain)
				return
			}

			loaded := transaction.Commit()

			if !op.memory.root.CompareAndSwap(current, loaded) {
				op.Error(core.ErrDomain)
				return
			}

			op.memory.classes.Clear()
			iterator := loaded.Root().Iterator()

			for key, _, found := iterator.Next(); found; key, _, found = iterator.Next() {
				if len(key) < 4 || key[0] != 'b' || key[1] != '/' {
					continue
				}

				rest := key[2:]
				slash := bytes.LastIndexByte(rest, '/')

				if slash <= 0 || slash == len(rest)-1 {
					continue
				}

				counter, _ := op.memory.classes.LoadOrStore(string(rest[slash+1:]), &atomic.Int32{})
				counted, _ := counter.(*atomic.Int32)

				if counted != nil {
					counted.Add(1)
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
