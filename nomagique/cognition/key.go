package cognition

import (
	"bytes"
)

/*
makeBasinKey builds b/<context>/<class>.
*/
func makeBasinKey(class, context []byte) []byte {
	buf := make([]byte, 2+len(context)+1+len(class))
	buf[0] = 'b'
	buf[1] = '/'
	copy(buf[2:], context)
	buf[2+len(context)] = '/'
	copy(buf[3+len(context):], class)
	return buf
}

/*
makeSensoryKey builds s/<context>.
*/
func makeSensoryKey(context []byte) []byte {
	buf := make([]byte, 2+len(context))
	buf[0] = 's'
	buf[1] = '/'
	copy(buf[2:], context)
	return buf
}

/*
parseBasinKey reads class and context back out of a b/<context>/<class> key.
Suffix/prefix matching order: exact (4), prefix (2), suffix (1).
*/
func parseBasinKey(k []byte) ([]byte, []byte, bool) {
	if len(k) < 4 || k[0] != 'b' || k[1] != '/' {
		return nil, nil, false
	}

	rem := k[2:]
	idx := bytes.LastIndexByte(rem, '/')

	if idx <= 0 || idx == len(rem)-1 {
		return nil, nil, false
	}

	return rem[idx+1:], rem[:idx], true
}
