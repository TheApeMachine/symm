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

/*
JournalStore manages persistent file storage for holdings and postmortem findings.
It ensures trade journal records survive system restarts.
*/
type JournalStore struct {
	mu       sync.Mutex
	filePath string
}

/*
JournalPayload defines the JSON serialization schema for persisted trade history.
*/
type JournalPayload struct {
	Holdings []*types.Holding `json:"holdings"`
	Findings []types.Finding  `json:"findings"`
}

/*
NewJournalStore constructs a new JournalStore pointing to journal.json in system data_path.
*/
func NewJournalStore() *JournalStore {
	dir := utils.ResolveDataPath()

	if dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}

	return &JournalStore{
		filePath: filepath.Join(dir, "journal.json"),
	}
}

/*
Save writes active and historical holdings plus postmortem findings to disk.
*/
func (journalStore *JournalStore) Save(holdings []*types.Holding, findings []types.Finding) error {
	if journalStore == nil || journalStore.filePath == "" {
		return nil
	}

	journalStore.mu.Lock()
	defer journalStore.mu.Unlock()

	payload := JournalPayload{
		Holdings: holdings,
		Findings: findings,
	}

	data, err := sonic.Marshal(payload)

	if err != nil {
		return errnie.Error(err)
	}

	return os.WriteFile(journalStore.filePath, data, 0644)
}

/*
Load retrieves saved holdings and findings from disk on boot.
*/
func (journalStore *JournalStore) Load() ([]*types.Holding, []types.Finding, error) {
	if journalStore == nil || journalStore.filePath == "" {
		return nil, nil, nil
	}

	journalStore.mu.Lock()
	defer journalStore.mu.Unlock()

	data, err := os.ReadFile(journalStore.filePath)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}

		return nil, nil, errnie.Error(err)
	}

	var payload JournalPayload

	if err := sonic.Unmarshal(data, &payload); err != nil {
		return nil, nil, errnie.Error(err)
	}

	return payload.Holdings, payload.Findings, nil
}
