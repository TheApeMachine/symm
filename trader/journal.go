package trader

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

var fastSonic = sonic.Config{
	EncodeNullForInfOrNan: true,
}.Froze()

const journalReplayByteBudget = 4 * 1024 * 1024

/*
JournalStore persists the trade journal as Thesis snapshots. The live Thesis
remains the only lifecycle model; the journal file is just a retained slice of
those snapshots for restart replay.
*/
type JournalStore struct {
	mu       sync.Mutex
	filePath string
}

/*
NewJournalStore constructs a new JournalStore pointing to journal.json in the
system data path.
*/
func NewJournalStore() *JournalStore {
	dir := utils.ResolveDataPath()

	if dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}

	return &JournalStore{filePath: filepath.Join(dir, "journal.json")}
}

/*
Save writes the retained Thesis snapshots to disk.
*/
func (journalStore *JournalStore) Save(theses []*types.Thesis) error {
	if journalStore == nil || journalStore.filePath == "" {
		return nil
	}

	journalStore.mu.Lock()
	defer journalStore.mu.Unlock()

	data, err := journalStore.marshal(theses)

	if err != nil {
		return errnie.Error(err)
	}

	temporary, err := os.CreateTemp(filepath.Dir(journalStore.filePath), ".journal-*")

	if err != nil {
		return errnie.Error(err)
	}

	temporaryPath := temporary.Name()

	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return errnie.Error(err)
	}

	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return errnie.Error(err)
	}

	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return errnie.Error(err)
	}

	if err := temporary.Close(); err != nil {
		return errnie.Error(err)
	}

	if err := os.Rename(temporaryPath, journalStore.filePath); err != nil {
		return errnie.Error(err)
	}

	directory, err := os.Open(filepath.Dir(journalStore.filePath))

	if err != nil {
		return errnie.Error(err)
	}

	defer directory.Close()

	return errnie.Error(directory.Sync())
}

/*
Load retrieves saved Thesis snapshots from disk on boot.
*/
func (journalStore *JournalStore) Load() ([]*types.Thesis, error) {
	if journalStore == nil || journalStore.filePath == "" {
		return nil, nil
	}

	journalStore.mu.Lock()
	defer journalStore.mu.Unlock()

	file, err := os.Open(journalStore.filePath)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, errnie.Error(err)
	}

	defer file.Close()

	raw, err := journalStore.tail(file)

	if err != nil {
		return nil, errnie.Error(err)
	}

	data, err := journalStore.array(raw)

	if err != nil {
		return nil, errnie.Error(err)
	}

	var theses []*types.Thesis

	if err := sonic.Unmarshal(data, &theses); err != nil {
		return nil, errnie.Error(err)
	}

	return theses, nil
}

/*
marshal bounds persisted journal size to the replay budget so restart replay can
fit through one retained UI frame instead of growing without limit.
*/
func (journalStore *JournalStore) marshal(theses []*types.Thesis) ([]byte, error) {
	raw, err := journalStore.retain(theses)

	if err != nil {
		return nil, errnie.Error(err)
	}

	return journalStore.array(raw)
}

/*
array joins already-encoded thesis snapshots into one JSON array payload so load
and save share the same bounded replay envelope.
*/
func (journalStore *JournalStore) array(raw [][]byte) ([]byte, error) {

	if len(raw) == 0 {
		return []byte("[]"), nil
	}

	size := 2

	for _, entry := range raw {
		size += len(entry)
	}

	size += len(raw) - 1
	data := make([]byte, 0, size)
	data = append(data, '[')

	for index, entry := range raw {
		if index > 0 {
			data = append(data, ',')
		}

		data = append(data, entry...)
	}

	data = append(data, ']')
	return data, nil
}

/*
tail streams one saved journal file and keeps only the newest entries that fit
inside the replay budget, avoiding a whole-file read on restart.
*/
func (journalStore *JournalStore) tail(reader io.Reader) ([][]byte, error) {
	decoder := json.NewDecoder(reader)
	err := journalStore.expectArrayDelimiter(decoder, '[', "journal: expected thesis array")

	if err != nil {
		if err == io.EOF {
			return nil, nil
		}

		return nil, err
	}

	raw := make([][]byte, 0)
	used := 0

	for decoder.More() {
		var entry json.RawMessage

		if err := decoder.Decode(&entry); err != nil {
			return nil, errnie.Error(err)
		}

		used, raw = journalStore.push(raw, used, entry)
	}

	if err := journalStore.expectArrayDelimiter(
		decoder,
		']',
		"journal: expected thesis array end",
	); err != nil {
		return nil, err
	}

	return raw, nil
}

/*
retain marshals thesis snapshots one by one and keeps only the newest replayable
tail so the saved journal cannot grow into a restart-sized memory spike.
*/
func (journalStore *JournalStore) retain(theses []*types.Thesis) ([][]byte, error) {
	raw := make([][]byte, 0, len(theses))
	used := 0

	for _, thesis := range theses {
		entry, err := fastSonic.Marshal(thesis)

		if err != nil {
			return nil, errnie.Error(err)
		}

		used, raw = journalStore.push(raw, used, entry)
	}

	return raw, nil
}

/*
push appends one encoded thesis while trimming the oldest retained entries until
the replay budget fits, always preserving at least the newest snapshot.
*/
func (journalStore *JournalStore) push(
	raw [][]byte,
	used int,
	entry []byte,
) (int, [][]byte) {
	copyEntry := append([]byte(nil), entry...)
	entrySize := len(copyEntry)

	if entrySize >= journalReplayByteBudget {
		errnie.Warn(fmt.Sprintf(
			"journal: snapshot size %d exceeds replay budget %d",
			entrySize,
			journalReplayByteBudget,
		))
		return entrySize, [][]byte{copyEntry}
	}

	for len(raw) > 0 && used+entrySize > journalReplayByteBudget {
		used -= len(raw[0])
		raw = raw[1:]
	}

	raw = append(raw, copyEntry)
	used += entrySize
	return used, raw
}

/*
expectArrayDelimiter consumes one JSON token and verifies the expected array
delimiter so replay errors stay consistent across both boundary checks.
*/
func (journalStore *JournalStore) expectArrayDelimiter(
	decoder *json.Decoder,
	expected json.Delim,
	message string,
) error {
	token, err := decoder.Token()

	if err != nil {
		return errnie.Error(err)
	}

	delimiter, ok := token.(json.Delim)

	if ok && delimiter == expected {
		return nil
	}

	return errnie.Error(errnie.Err(errnie.Validation, message, nil))
}
