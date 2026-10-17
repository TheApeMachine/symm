package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
)

var paperCmd = &cobra.Command{
	Use:   "paper",
	Short: "Upload the paper-trade records `make run` appended locally",
	Long: `paper reads the local paper.records_path file that the dashboard run
(make run) appends one data.Value per traded paper decision to, validates
every record (data.PaperFields), and uploads each under its own key,
paper/<action>/<symbol>/<epoch>/<seq>.bin, with io.Copy. It is the only
writer of the paper/ prefix; run itself never writes storage. A record whose
key is already stored (or repeats in the file) refuses the whole upload
before anything is written: stored paper records are never overwritten.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := system.Cfg.Paper.Validate(); err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "[paper]", err))
		}

		path := system.Cfg.Paper.RecordsPath
		file, err := os.Open(path)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.IO, "[paper] records file "+path, err))
		}

		defer file.Close()

		records, err := readPaperRecords(file)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.UnprocessableContent, "[paper] "+path, err))
		}

		if len(records) == 0 {
			return errnie.Error(errnie.Err(errnie.NotFound, "[paper] "+path+" holds no paper records", nil))
		}

		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		client := store.NewClient(ctx, system.Cfg.Storage.S3.Bucket)

		if client == nil {
			return errnie.Error(errnie.Err(errnie.IO, "[paper] unable to open store client", nil))
		}

		if err := uploadPaper(client, records); err != nil {
			return errnie.Error(errnie.Err(errnie.IO, "[paper] "+path, err))
		}

		errnie.Info(fmt.Sprintf("[paper] uploaded %d paper records from %s", len(records), path))
		return nil
	},
}

/*
readPaperRecords decodes the records file: serialized Values back to back,
exactly as Training appends them with io.Copy. A truncated or non-paper
record is an error naming its position.
*/
func readPaperRecords(r io.Reader) ([]*data.Value, error) {
	reader := bufio.NewReader(r)
	records := make([]*data.Value, 0)

	for {
		value := data.New()

		if n, err := value.ReadFrom(reader); err != nil {
			// Only an end before any byte of a record is the end of the
			// file; anywhere later it is a truncated record.
			if n == 0 && errors.Is(err, io.EOF) {
				return records, nil
			}

			return nil, fmt.Errorf("record %d does not decode: %w", len(records)+1, err)
		}

		if _, err := data.PaperFields(value); err != nil {
			return nil, fmt.Errorf("record %d (%s): %w", len(records)+1, value.Prefix, err)
		}

		records = append(records, value)
	}
}

/*
uploadPaper writes records under their own keys with io.Copy, after checking
that none of those keys is stored already or repeats among the records. Any
collision is an error naming every colliding key, and nothing is written.
*/
func uploadPaper(client *store.Client, records []*data.Value) error {
	stored, err := client.ListKeys(data.PaperActionPrefixes()...)

	if err != nil {
		return fmt.Errorf("listing stored paper records: %w", err)
	}

	taken := make(map[string]bool, len(stored)+len(records))

	for _, key := range stored {
		taken[key] = true
	}

	var collisions error

	for _, record := range records {
		if taken[record.Prefix] {
			collisions = errors.Join(collisions, fmt.Errorf("paper record %s is already stored (or repeats in the file): refusing to overwrite it", record.Prefix))
		}

		taken[record.Prefix] = true
	}

	if collisions != nil {
		return collisions
	}

	for _, record := range records {
		if _, err := io.Copy(client, record); err != nil {
			return fmt.Errorf("upload %s: %w", record.Prefix, err)
		}
	}

	if err := client.Drain(); err != nil {
		return fmt.Errorf("uploads: %w", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(paperCmd)
}
