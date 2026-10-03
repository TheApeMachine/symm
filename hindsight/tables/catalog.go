package tables

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apache/iceberg-go"
	icecat "github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/catalog/rest"
	icebergio "github.com/apache/iceberg-go/io"
	_ "github.com/apache/iceberg-go/io/gocloud"
	"github.com/apache/iceberg-go/table"
	"github.com/apache/iceberg-go/utils"
	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/system"
)

const anonymousCredential = "anonymous"

/*
Catalog manages connections and schemas for canonical Iceberg tables.
*/
type Catalog struct {
	underlying icecat.Catalog
	awsConfig  *aws.Config
	// commitGate serializes Iceberg Append/commit across Drain and Training so
	// concurrent writers cannot wedge optimistic retries into an apparent hang
	// (UI: durability blocked: writing with writing=true forever). Capacity 1.
	commitGate chan struct{}
}

/*
Wrap adapts an Iceberg catalog implementation (REST for production, SQLite for tests).
*/
func Wrap(underlying icecat.Catalog) *Catalog {
	return &Catalog{
		underlying: underlying,
		commitGate: make(chan struct{}, 1),
	}
}

/*
WithCommit runs fn while holding the catalog commit gate. All Iceberg Append
paths must use this so StoreTee Drain and Training.persistLoop cannot contend.
ctx cancellation unblocks waiters so Training writing cannot latch past
persistCommitTimeout while Drain holds the gate.
*/
func (catalog *Catalog) WithCommit(ctx context.Context, fn func() error) error {
	if catalog == nil {
		return fn()
	}

	if catalog.commitGate == nil {
		catalog.commitGate = make(chan struct{}, 1)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case catalog.commitGate <- struct{}{}:
	}

	defer func() { <-catalog.commitGate }()

	return fn()
}

/*
Open connects to the Iceberg REST catalog configured in system.Cfg.Storage.
*/
func Open(ctx context.Context) *Catalog {
	storageConfig := system.Cfg.Storage

	if storageConfig == nil || storageConfig.Iceberg == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[iceberg] storage configuration required",
			nil,
		))

		return nil
	}

	icebergConfig := storageConfig.Iceberg
	s3Config := storageConfig.S3

	if icebergConfig.URI == "" || icebergConfig.Warehouse == "" {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[iceberg] storage.iceberg.uri and storage.iceberg.warehouse are both required",
			nil,
		))

		return nil
	}

	properties := iceberg.Properties{
		icebergio.S3EndpointURL: s3Config.Endpoint,
		icebergio.S3Region:      s3Config.Region,
	}

	if !s3Config.Anonymous && s3Config.AccessKeyID != "" {
		properties[icebergio.S3AccessKeyID] = s3Config.AccessKeyID
		properties[icebergio.S3SecretAccessKey] = s3Config.SecretAccessKey
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        512,
		MaxIdleConnsPerHost: 256,
		MaxConnsPerHost:     256,
		IdleConnTimeout:     90 * time.Second,
	}

	httpClient := awshttp.NewBuildableClient().WithTransportOptions(func(clientTransport *http.Transport) {
		clientTransport.Proxy = http.ProxyFromEnvironment
		clientTransport.DialContext = (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext
		clientTransport.MaxIdleConns = 512
		clientTransport.MaxIdleConnsPerHost = 256
		clientTransport.MaxConnsPerHost = 256
		clientTransport.IdleConnTimeout = 90 * time.Second
	})

	awsOpts := []func(*config.LoadOptions) error{
		config.WithHTTPClient(httpClient),
	}

	if s3Config.Region != "" {
		awsOpts = append(awsOpts, config.WithRegion(s3Config.Region))
	}

	if s3Config.Anonymous {
		awsOpts = append(awsOpts, config.WithCredentialsProvider(aws.AnonymousCredentials{}))
	} else if s3Config.AccessKeyID != "" || s3Config.SecretAccessKey != "" {
		awsOpts = append(awsOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(s3Config.AccessKeyID, s3Config.SecretAccessKey, ""),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, awsOpts...)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[iceberg] failed to load AWS configuration",
			err,
		))
	}

	restOpts := []rest.Option{
		rest.WithWarehouseLocation(icebergConfig.Warehouse),
		rest.WithAdditionalProps(properties),
		rest.WithCustomTransport(transport),
	}

	if err == nil {
		restOpts = append(restOpts, rest.WithAwsConfig(awsCfg))
	}

	if err := ensureStorageBuckets(ctx, s3Config, icebergConfig); err != nil {
		errnie.Error(err)
	}

	connected, err := rest.NewCatalog(ctx, "seaweed", icebergConfig.URI, restOpts...)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to connect to catalog "+icebergConfig.URI,
			err,
		))

		return nil
	}

	cat := Wrap(connected)
	cat.awsConfig = &awsCfg

	return cat
}

/*
Ensure creates missing canonical tables and applies configured properties to existing tables.
*/
func (catalog *Catalog) Ensure(ctx context.Context) error {
	if err := catalog.ensureBuckets(ctx); err != nil {
		return err
	}

	properties := iceberg.Properties{
		table.MetadataDeleteAfterCommitEnabledKey: "true",
		table.MetadataPreviousVersionsMaxKey:     "5",
		table.ManifestMergeEnabledKey:             "true",
	}

	if system.Cfg.Storage != nil && system.Cfg.Storage.Iceberg != nil {
		retries := system.Cfg.Storage.Iceberg.CommitRetries

		if retries > 0 {
			properties[table.CommitNumRetriesKey] = strconv.Itoa(retries)
		}
	}

	namespace := table.Identifier{Namespace}

	if err := catalog.underlying.CreateNamespace(ctx, namespace, nil); err != nil {
		if !errors.Is(err, icecat.ErrNamespaceAlreadyExists) {
			return errnie.Error(errnie.Err(
				errnie.BadGateway,
				"[iceberg] failed to create namespace "+Namespace,
				err,
			))
		}
	}

	families := []struct {
		name         string
		schema       *iceberg.Schema
		partitioning iceberg.PartitionSpec
	}{
		{Measurements, MeasurementSchema(), MeasurementPartitioning()},
		{Runs, RunsSchema(), RunsPartitioning()},
	}

	for _, family := range families {
		if err := catalog.ensureTable(
			ctx, family.name, family.schema, family.partitioning, properties,
		); err != nil {
			return err
		}
	}

	return nil
}

func (catalog *Catalog) ensureTable(
	ctx context.Context,
	name string,
	schema *iceberg.Schema,
	partitioning iceberg.PartitionSpec,
	properties iceberg.Properties,
) error {
	ctx = catalog.context(ctx)
	identifier := table.Identifier{Namespace, name}
	exists, err := catalog.underlying.CheckTableExists(ctx, identifier)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to check table "+name,
			err,
		))
	}

	if exists {
		if err := catalog.evolveSchema(ctx, name, schema); err != nil {
			return err
		}

		return catalog.ensureProperties(ctx, name, properties)
	}

	if _, err := catalog.underlying.CreateTable(
		ctx, identifier, schema,
		icecat.WithPartitionSpec(&partitioning), icecat.WithProperties(properties),
	); err != nil {
		if errors.Is(err, icecat.ErrTableAlreadyExists) {
			return catalog.ensureProperties(ctx, name, properties)
		}

		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to create table "+name,
			err,
		))
	}

	return nil
}

/*
evolveSchema brings an existing table up to the canonical schema. Columns the
table lacks are added when they are optional, since old rows read them as null;
any other divergence (a required column the table lacks, or a column whose type
differs) cannot be reconciled without rewriting data and fails explicitly.
*/
func (catalog *Catalog) evolveSchema(
	ctx context.Context, name string, canonical *iceberg.Schema,
) error {
	loaded, err := catalog.Load(ctx, name)

	if err != nil {
		return err
	}

	existing := loaded.Schema()
	var missing []iceberg.NestedField

	for _, field := range canonical.Fields() {
		found, ok := existing.FindFieldByName(field.Name)

		if !ok {
			missing = append(missing, field)
			continue
		}

		if found.Type.String() != field.Type.String() {
			return errnie.Error(errnie.Err(
				errnie.Conflict,
				fmt.Sprintf("[iceberg] table %s column %s has type %s, want %s", name, field.Name, found.Type, field.Type),
				nil,
			))
		}
	}

	if len(missing) == 0 {
		return nil
	}

	transaction := loaded.NewTransaction()
	update := transaction.UpdateSchema(true, false)

	for _, field := range missing {
		if field.Required {
			return errnie.Error(errnie.Err(
				errnie.Conflict,
				fmt.Sprintf("[iceberg] table %s lacks required column %s and cannot be evolved", name, field.Name),
				nil,
			))
		}

		update.AddColumn([]string{field.Name}, field.Type, field.Doc, false, nil)
	}

	if err := update.Commit(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway, "[iceberg] failed to evolve schema of "+name, err,
		))
	}

	if _, err := transaction.Commit(ctx); err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway, "[iceberg] failed to commit schema of "+name, err,
		))
	}

	return nil
}

func (catalog *Catalog) ensureProperties(ctx context.Context, name string, properties iceberg.Properties) error {
	if len(properties) == 0 {
		return nil
	}

	loaded, err := catalog.Load(ctx, name)

	if err != nil {
		return err
	}

	changed := iceberg.Properties{}

	for key, value := range properties {
		if loaded.Properties()[key] != value {
			changed[key] = value
		}
	}

	if len(changed) == 0 {
		return nil
	}

	transaction := loaded.NewTransaction()

	if err := transaction.SetProperties(changed); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "[iceberg] invalid properties for "+name, err))
	}

	if _, err := transaction.Commit(ctx); err != nil {
		return errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] failed to configure "+name, err))
	}

	return nil
}

func (catalog *Catalog) ensureBuckets(ctx context.Context) error {
	if system.Cfg.Storage == nil || system.Cfg.Storage.S3 == nil {
		return nil
	}

	return ensureStorageBuckets(ctx, system.Cfg.Storage.S3, system.Cfg.Storage.Iceberg)
}

func ensureStorageBuckets(
	ctx context.Context,
	s3Config *system.S3,
	icebergConfig *system.Iceberg,
) error {
	if s3Config == nil || s3Config.Endpoint == "" {
		return nil
	}

	client := &http.Client{
		Transport: http.DefaultTransport,
		Timeout:   10 * time.Second,
	}

	if icebergConfig != nil && icebergConfig.Warehouse != "" {
		tableBucket := parseBucketName(icebergConfig.Warehouse)

		if tableBucket != "" {
			if err := ensureTableBucket(ctx, client, s3Config.Endpoint, tableBucket); err != nil {
				return err
			}
		}
	}

	if s3Config.Bucket != "" {
		if err := ensureS3Bucket(ctx, client, s3Config.Endpoint, s3Config.Bucket); err != nil {
			return err
		}
	}

	return nil
}

func ensureTableBucket(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	name string,
) error {
	target := strings.TrimRight(endpoint, "/") + "/"
	payload := fmt.Appendf(nil, `{"name":"%s","format":"ICEBERG"}`, name)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to construct table bucket request for "+name,
			err,
		))
	}

	req.Header.Set("X-Amz-Target", "S3Tables.CreateTableBucket")
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to create table bucket "+name,
			err,
		))
	}

	defer res.Body.Close()

	if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusCreated || res.StatusCode == http.StatusConflict || res.StatusCode == http.StatusForbidden {
		return nil
	}

	body, readErr := io.ReadAll(res.Body)

	if readErr != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to read response body for table bucket "+name,
			readErr,
		))
	}

	if strings.Contains(string(body), "BucketAlreadyExists") {
		return nil
	}

	return errnie.Error(errnie.Err(
		errnie.BadGateway,
		fmt.Sprintf("[iceberg] unexpected response creating table bucket %s: %d %s", name, res.StatusCode, string(body)),
		nil,
	))
}

func ensureS3Bucket(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	name string,
) error {
	target := strings.TrimRight(endpoint, "/") + "/" + name

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, nil)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to construct bucket request for "+name,
			err,
		))
	}

	res, err := client.Do(req)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to create bucket "+name,
			err,
		))
	}

	defer res.Body.Close()

	if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusCreated || res.StatusCode == http.StatusNoContent || res.StatusCode == http.StatusConflict || res.StatusCode == http.StatusForbidden {
		return nil
	}

	body, readErr := io.ReadAll(res.Body)

	if readErr != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to read response body for bucket "+name,
			readErr,
		))
	}

	if strings.Contains(string(body), "BucketAlreadyOwnedByYou") || strings.Contains(string(body), "BucketAlreadyExists") {
		return nil
	}

	return errnie.Error(errnie.Err(
		errnie.BadGateway,
		fmt.Sprintf("[iceberg] unexpected response creating bucket %s: %d %s", name, res.StatusCode, string(body)),
		nil,
	))
}

func parseBucketName(location string) string {
	trimmed := strings.Trim(strings.TrimPrefix(location, "s3://"), "/")

	if bucket, _, found := strings.Cut(trimmed, "/"); found {
		return bucket
	}

	return trimmed
}

/*
Load returns one of the canonical Hindsight tables.
*/
func (catalog *Catalog) Load(ctx context.Context, name string) (*table.Table, error) {
	retries := 3

	if system.Cfg != nil && system.Cfg.Storage != nil && system.Cfg.Storage.Iceberg != nil && system.Cfg.Storage.Iceberg.CommitRetries > 0 {
		retries = system.Cfg.Storage.Iceberg.CommitRetries
	}

	var (
		loaded *table.Table
		err    error
	)

	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 100 * time.Millisecond):
			}
		}

		loaded, err = catalog.underlying.LoadTable(catalog.context(ctx), table.Identifier{Namespace, name})

		if err == nil {
			return loaded, nil
		}

		if errors.Is(err, icecat.ErrNoSuchTable) {
			break
		}
	}

	return nil, errnie.Error(errnie.Err(
		errnie.BadGateway,
		"[iceberg] failed to load table "+name,
		err,
	))
}

func (catalog *Catalog) context(ctx context.Context) context.Context {
	if catalog.awsConfig == nil {
		return ctx
	}

	return utils.WithAwsConfig(ctx, catalog.awsConfig)
}

/*
RecordRun appends one process run fact to the canonical runs metadata table.
*/
func (catalog *Catalog) RecordRun(ctx context.Context, run Run) error {
	tbl, err := catalog.Load(ctx, Runs)

	if err != nil {
		return err
	}

	reader, err := runRecords(tbl.Schema(), []Run{run})

	if err != nil {
		return err
	}

	defer reader.Release()

	var appendErr error
	err = catalog.WithCommit(ctx, func() error {
		_, appendErr = tbl.Append(catalog.context(ctx), reader, nil)
		return appendErr
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to record run",
			err,
		))
	}

	return nil
}

/*
Runs reads all runs from the runs metadata table, ordered newest first.
*/
func (catalog *Catalog) Runs(ctx context.Context) ([]Run, error) {
	if catalog != nil && catalog.awsConfig != nil {
		ctx = utils.WithAwsConfig(ctx, catalog.awsConfig)
	}

	tbl, err := catalog.Load(ctx, Runs)

	if err != nil {
		return nil, err
	}

	tasks, err := tbl.Scan().PlanFiles(ctx)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to plan runs scan",
			err,
		))
	}

	_, batches, err := tbl.Scan().ReadTasks(ctx, tasks)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to read runs tasks",
			err,
		))
	}

	var allRuns []Run

	for batch, batchErr := range batches {
		if batchErr != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.BadGateway,
				"[iceberg] runs batch decode failure",
				batchErr,
			))
		}

		if batch != nil {
			allRuns = append(allRuns, readRuns(batch)...)
			batch.Release()
		}
	}

	sort.Slice(allRuns, func(leftIndex, rightIndex int) bool {
		return allRuns[leftIndex].Epoch > allRuns[rightIndex].Epoch
	})

	seen := make(map[int64]struct{}, len(allRuns))
	runs := make([]Run, 0, len(allRuns))

	for _, run := range allRuns {
		if _, exists := seen[run.Epoch]; exists {
			continue
		}

		seen[run.Epoch] = struct{}{}
		runs = append(runs, run)
	}

	return runs, nil
}

