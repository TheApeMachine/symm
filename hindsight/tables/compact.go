package tables

import (
	"context"

	"github.com/apache/iceberg-go/table"
	"github.com/apache/iceberg-go/table/compaction"
	"github.com/theapemachine/errnie"
)

/*
CompactResult reports the file count and byte metrics after table compaction.
*/
type CompactResult struct {
	RewrittenFiles int
	NewFiles       int
	RewrittenBytes int64
	NewBytes       int64
}

/*
CompactTable bin-packs and rewrites small unoptimized data files in an Iceberg table into
coalesced Parquet files, committing the replacement atomically.
*/
func (c *Catalog) CompactTable(
	ctx context.Context,
	tableName string,
	targetFileSizeBytes int64,
) (*CompactResult, error) {
	if c == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[iceberg] compact: catalog is nil",
			nil,
		))
	}

	tbl, err := c.Load(ctx, tableName)

	if err != nil {
		return nil, err
	}

	tasks, err := tbl.Scan().PlanFiles(ctx)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] compact: failed to plan files for "+tableName,
			err,
		))
	}

	if len(tasks) < 2 {
		return &CompactResult{}, nil
	}

	if targetFileSizeBytes <= 0 {
		targetFileSizeBytes = 64 * 1024 * 1024
	}

	cfg := compaction.DefaultConfig()
	cfg.TargetFileSizeBytes = targetFileSizeBytes
	cfg.MinFileSizeBytes = targetFileSizeBytes / 2
	cfg.MaxFileSizeBytes = targetFileSizeBytes * 2
	cfg.MinInputFiles = 2

	plan, err := cfg.PlanCompaction(tasks)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] compact: failed to plan compaction for "+tableName,
			err,
		))
	}

	if len(plan.Groups) == 0 {
		return &CompactResult{}, nil
	}

	groups := make([]table.CompactionTaskGroup, len(plan.Groups))

	for i, g := range plan.Groups {
		groups[i] = table.CompactionTaskGroup{
			PartitionKey:   g.PartitionKey,
			Tasks:          g.Tasks,
			TotalSizeBytes: g.TotalSizeBytes,
		}
	}

	tx := tbl.NewTransaction()
	rewriteResult, err := tx.RewriteDataFiles(ctx, groups, table.RewriteDataFilesOptions{})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] compact: rewrite data files failed for "+tableName,
			err,
		))
	}

	if _, err := tx.Commit(ctx); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] compact: commit rewrite failed for "+tableName,
			err,
		))
	}

	res := &CompactResult{
		RewrittenFiles: rewriteResult.RemovedDataFiles,
		NewFiles:       rewriteResult.AddedDataFiles,
		RewrittenBytes: rewriteResult.BytesBefore,
		NewBytes:       rewriteResult.BytesAfter,
	}

	return res, nil
}

/*
ExpireTableSnapshots retains the newest n snapshots and expires older snapshot metadata.
*/
func (c *Catalog) ExpireTableSnapshots(
	ctx context.Context,
	tableName string,
	retainLast int,
) error {
	if c == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[iceberg] expire: catalog is nil",
			nil,
		))
	}

	if retainLast <= 0 {
		retainLast = 5
	}

	tbl, err := c.Load(ctx, tableName)

	if err != nil {
		return err
	}

	tx := tbl.NewTransaction()

	if err := tx.ExpireSnapshots(table.WithRetainLast(retainLast)); err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] expire: failed to stage snapshot expiration for "+tableName,
			err,
		))
	}

	if _, err := tx.Commit(ctx); err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] expire: failed to commit snapshot expiration for "+tableName,
			err,
		))
	}

	return nil
}

/*
CompactAll triggers compaction and snapshot cleanup across all canonical table families.
*/
func (c *Catalog) CompactAll(ctx context.Context) error {
	families := []string{
		SpotLevel3, SpotTicker, SpotTrade,
		FuturesTicker, FuturesTrade, Executions,
		Measurements, Models, Grids,
		Positions, Decisions, Outcomes,
	}

	for _, family := range families {
		if _, err := c.CompactTable(ctx, family, 64*1024*1024); err != nil {
			errnie.Error(err)
		}

		if err := c.ExpireTableSnapshots(ctx, family, 5); err != nil {
			errnie.Error(err)
		}
	}

	return nil
}
