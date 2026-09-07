package core

import "errors"

/* Decoder accumulates field failures while projecting one record at a Go boundary. */
type Decoder struct {
	fields map[string]Primitive
	err    error
}

func NewDecoder(fields map[string]Primitive) *Decoder { return &Decoder{fields: fields} }
func (decoder *Decoder) Error() error                 { return decoder.err }

/* Decode preserves the distinction between an absent field and an observed zero. */
func Decode[Value any](decoder *Decoder, path ...string) Value {
	value, err := Field[Value](decoder.fields, path...)
	decoder.err = errors.Join(decoder.err, err)
	return value
}
