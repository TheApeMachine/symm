package rawdump

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
)

const defaultSeedLimit = 8192

/*
ObservationSeed reads the most recent JSONL observations for one signal dump field.
It scans runs/<name>_raw.jsonl (or signals.<name>.raw_dump_file) from the tail so
calibrators can warm-start from the previous session instead of generic seed edges.
*/
func ObservationSeed(signalName, jsonField string, limit int) ([]float64, error) {
	path := strings.TrimSpace(viper.GetString("signals." + signalName + ".raw_dump_file"))

	if path == "" {
		path = filepath.Join(Dir(), signalName+"_raw.jsonl")
	}

	return ObservationSeedFromPath(path, jsonField, limit)
}

/*
ObservationSeedFromPath reads observations from an explicit JSONL path.
*/
func ObservationSeedFromPath(path, jsonField string, limit int) ([]float64, error) {
	if limit <= 0 {
		limit = defaultSeedLimit
	}

	file, err := os.Open(path)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("rawdump seed %s: %w", path, err)
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)

	const maxLineBytes = 1024 * 1024
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, maxLineBytes)

	lines := make([]string, 0, limit)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		lines = append(lines, line)

		if len(lines) > limit {
			lines = lines[1:]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("rawdump seed %s: scan: %w", path, err)
	}

	observations := make([]float64, 0, len(lines))

	for _, line := range lines {
		var row map[string]any

		if err := sonic.UnmarshalString(line, &row); err != nil {
			continue
		}

		value, ok := row[jsonField].(float64)

		if !ok || value <= 0 {
			continue
		}

		observations = append(observations, value)
	}

	return observations, nil
}
