package hindsight

import (
	"hash/fnv"
	"io"
	"slices"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy"
)

/*
KeysFingerprint hashes the listed tick object keys of one run. Any new or
removed tick object invalidates a stored manifest or scan stamp.
*/
func KeysFingerprint(keys []string) uint64 {
	sorted := slices.Clone(keys)
	slices.Sort(sorted)

	hash := fnv.New64a()

	for _, key := range sorted {
		hash.Write([]byte(key))
		hash.Write([]byte{0})
	}

	return hash.Sum64()
}

func manifestPrefix(run uint64) string {
	return data.Prefix(data.TapeCacheOrigin, data.TapeManifestRole, "run", run)
}

func stampPrefix(run uint64) string {
	return data.Prefix(data.TapeCacheOrigin, data.TapeStampRole, "run", run)
}

func readManifest(client *store.Client, run uint64) (keysHash float64, count float64, ok bool) {
	for value, err := range store.Values(client, manifestPrefix(run)) {
		if err != nil || value == nil {
			return 0, 0, false
		}

		metrics := data.Metrics(value)

		return metrics["keys_hash"], metrics["object_count"], true
	}

	return 0, 0, false
}

func readStamp(client *store.Client, run uint64) (retrace, version, drafts float64, ok bool) {
	for value, err := range store.Values(client, stampPrefix(run)) {
		if err != nil || value == nil {
			return 0, 0, 0, false
		}

		metrics := data.Metrics(value)

		return metrics["retrace"], metrics["detector_version"], metrics["draft_count"], true
	}

	return 0, 0, 0, false
}

/*
ScanUpToDate reports whether drafts on store already match this tick listing
and learned retrace. detect and hindsight scan skip work when this is true.
*/
func ScanUpToDate(client *store.Client, run uint64, retrace float64, keys []string) bool {
	if client == nil || len(keys) == 0 {
		return false
	}

	wantHash := float64(KeysFingerprint(keys))
	storedHash, count, ok := readManifest(client, run)

	if !ok || storedHash != wantHash || count != float64(len(keys)) {
		return false
	}

	storedRetrace, version, draftCount, ok := readStamp(client, run)

	if !ok || draftCount < 1 {
		return false
	}

	if version != float64(strategy.ScanVersion) {
		return false
	}

	return storedRetrace == retrace
}

/*
WriteRunManifest records the tick listing fingerprint after a successful assemble.
*/
func WriteRunManifest(client *store.Client, run uint64, keys []string) error {
	if client == nil {
		return nil
	}

	value := data.NewValue(data.TapeCacheOrigin, data.TapeManifestRole, "run")
	value.Epoch = run
	value.Tick = 1
	value.FormatPrefix()
	data.AppendVector(value, []float64{
		float64(KeysFingerprint(keys)),
		float64(len(keys)),
	}, "keys_hash", "object_count")

	_, err := io.Copy(client, value)

	return err
}

func countRunDrafts(client *store.Client, run uint64) int {
	count := 0

	for _, class := range data.ExcursionClasses {
		for value, err := range store.Values(client, data.DraftOrigin+"/"+class+"/") {
			if err != nil || value == nil || value.Epoch != run {
				continue
			}

			count++
		}
	}

	return count
}

/*
WriteScanStamp records the retrace and draft count of the last successful scan.
*/
func WriteScanStamp(client *store.Client, run uint64, retrace float64, _ int) error {
	drafts := countRunDrafts(client, run)
	if client == nil {
		return nil
	}

	value := data.NewValue(data.TapeCacheOrigin, data.TapeStampRole, "run")
	value.Epoch = run
	value.Tick = 1
	value.FormatPrefix()
	data.AppendVector(value, []float64{
		retrace,
		float64(strategy.ScanVersion),
		float64(drafts),
	}, "retrace", "detector_version", "draft_count")

	_, err := io.Copy(client, value)

	return err
}
