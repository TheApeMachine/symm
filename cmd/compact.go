package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/system"
)

var (
	compactEpochTarget int64
	compactTableName   string
)

var compactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Consolidate fragmented Parquet files into target-sized files",
	Long: `compact scans historical epochs in Iceberg tables and consolidates fragmented
micro-files into target-sized Parquet files estimated via measurement.ApproximateBytes().`,
	RunE: func(cmd *cobra.Command, args []string) error {
		system.NewConfig()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tables.Open(ctx)

		if catalog == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[compact] unable to open iceberg catalog",
				nil,
			))
		}

		errnie.Info(fmt.Sprintf("[compact] starting compaction for table %s epoch %d", compactTableName, compactEpochTarget))

		if err := catalog.Compact(ctx, compactTableName, compactEpochTarget); err != nil {
			return errnie.Error(err)
		}

		errnie.Info(fmt.Sprintf("[compact] compaction complete for table %s epoch %d", compactTableName, compactEpochTarget))

		return nil
	},
}

func init() {
	compactCmd.Flags().Int64Var(&compactEpochTarget, "epoch", 0, "Epoch to compact (0 for all epochs)")
	compactCmd.Flags().StringVar(&compactTableName, "table", tables.Measurements, "Table to compact (default: measurements)")
	rootCmd.AddCommand(compactCmd)
}
