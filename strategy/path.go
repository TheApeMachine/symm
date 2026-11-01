package strategy

import (
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
)

/*
InitialPathResistance is the desk baseline for a brand-new leaf. Profit and
loss apply ±deltas on top; paper eligibility only opens near ≤0.2, so a
path must earn its way down from here instead of starting paper-ready at 0.
*/
const InitialPathResistance = 0.5

/*
PathBook is one action leaf under model/paths — plain JSON like Envelope.
Resistance is the absolute desk book.
*/
type PathBook struct {
	Action     string  `json:"action"`
	Resistance float64 `json:"resistance"`
}

/*
NewPathBook seeds a leaf at InitialPathResistance.
*/
func NewPathBook(action Action) *PathBook {
	return &PathBook{
		Action:     action.String(),
		Resistance: InitialPathResistance,
	}
}

/*
Apply folds a settled ±delta into the absolute book.
*/
func (book *PathBook) Apply(delta float64) {
	book.Resistance += delta
}

/*
Bytes marshals the leaf as JSON for S3.
*/
func (book *PathBook) Bytes() []byte {
	buf, err := runtime.Sonic.Marshal(book)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO, "[strategy.path] marshal path book", err,
		))

		return nil
	}

	return buf
}

/*
DecodePathBook reads a JSON path leaf from S3 bytes.
*/
func DecodePathBook(raw []byte) *PathBook {
	if len(raw) == 0 {
		return nil
	}

	var book PathBook

	if err := runtime.Sonic.Unmarshal(raw, &book); err != nil {
		return nil
	}

	if book.Action == "" {
		return nil
	}

	return &book
}

/*
StorageKey is the S3 object key for this leaf under a token path.
*/
func (book *PathBook) StorageKey(path string) string {
	if path == "" {
		return ""
	}

	return "model/paths/" + path + "/" + book.Action + ".bin"
}

/*
PathFromStorageKey strips model/paths/ and the action leaf filename.
*/
func PathFromStorageKey(key string) string {
	const prefix = "model/paths/"

	if !strings.HasPrefix(key, prefix) {
		return ""
	}

	rest := key[len(prefix):]
	slash := strings.LastIndex(rest, "/")

	if slash < 0 {
		return ""
	}

	tail := rest[slash+1:]

	if strings.HasSuffix(tail, ".bin") || strings.HasSuffix(tail, ".json") {
		return rest[:slash]
	}

	return rest
}
