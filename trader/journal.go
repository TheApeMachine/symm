package trader

import (
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

	data, err := fastSonic.Marshal(theses)

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

	data, err := os.ReadFile(journalStore.filePath)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, errnie.Error(err)
	}

	var theses []*types.Thesis

	if err := sonic.Unmarshal(data, &theses); err != nil {
		return nil, errnie.Error(err)
	}

	return theses, nil
}
