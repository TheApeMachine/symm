package replay

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

/*
LoadFrames reads newline-delimited Kraken v2 websocket JSON frames from path.
Blank lines and whitespace-only lines are skipped.
*/
func LoadFrames(path string) ([][]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open replay file: %w", err)
	}

	defer file.Close()

	return ReadFrames(file)
}

/*
ReadFrames parses newline-delimited websocket JSON frames from reader.
*/
func ReadFrames(reader io.Reader) ([][]byte, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	frames := make([][]byte, 0, 128)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())

		if len(line) == 0 {
			continue
		}

		if line[0] != '{' {
			return nil, fmt.Errorf("replay frame must be JSON object, got %q", truncateLine(line))
		}

		frame := append([]byte(nil), line...)
		frames = append(frames, frame)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read replay frames: %w", err)
	}

	if len(frames) == 0 {
		return nil, fmt.Errorf("replay source contained no frames")
	}

	return frames, nil
}

func truncateLine(line []byte) string {
	text := string(line)

	if len(text) <= 80 {
		return text
	}

	return strings.TrimSpace(text[:80]) + "..."
}
