package tables

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/catalog/rest"
	icebergio "github.com/apache/iceberg-go/io"
	"github.com/apache/iceberg-go/utils"
	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"

	// The catalog hands back s3:// metadata locations, and iceberg resolves
	// those through its own FileIO registry rather than through this project's
	// object store. Without this registration every append fails on an
	// unregistered scheme.
	_ "github.com/apache/iceberg-go/io/gocloud"
	"github.com/apache/iceberg-go/table"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

/*
anonymousCredential is what an unauthenticated venue is signed with. Its value
is never verified; it exists so the SDK stops looking for a real one.
*/
const anonymousCredential = "anonymous"

/*
Catalog is Hindsight's connection to one Iceberg table bucket.

SeaweedFS serves a separate Iceberg catalog per table bucket, so the warehouse
location selects which catalog is addressed rather than naming a directory
inside a shared one.
*/
type Catalog struct {
	catalog catalog.Catalog

	cacheMu       sync.RWMutex
	cachedEpochs  []int64
	epochsLoaded  time.Time
	timelineIndex map[int64]*runTimelineIndex
	awsConfig     *aws.Config
}

/*
Wrap adapts any Iceberg catalog implementation. Production uses the REST
catalog SeaweedFS serves; tests use a SQLite catalog over a temporary
directory, which exercises the same schemas, encoders, and scans without
needing a catalog server.
*/
func Wrap(underlying catalog.Catalog) *Catalog {
	return &Catalog{
		catalog:       underlying,
		timelineIndex: make(map[int64]*runTimelineIndex),
	}
}

/*
Context enriches ctx with the catalog's pooled AWS configuration so internal
FileIO operations reuse the same HTTP transport and avoid TCP port exhaustion.
*/
func (c *Catalog) Context(ctx context.Context) context.Context {
	if c == nil || c.awsConfig == nil {
		return ctx
	}

	return utils.WithAwsConfig(ctx, c.awsConfig)
}

/*
Open connects to the Iceberg REST catalog named by storage.iceberg.uri, for the
table bucket named by storage.iceberg.warehouse.
*/
func Open(ctx context.Context) *Catalog {
	uri := viper.GetString("storage.iceberg.uri")
	warehouse := viper.GetString("storage.iceberg.warehouse")

	if uri == "" || warehouse == "" {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[iceberg] storage.iceberg.uri and storage.iceberg.warehouse are both required",
			nil,
		))

		return nil
	}

	s3Endpoint := viper.GetString("storage.s3.endpoint")
	s3Region := viper.GetString("storage.s3.region")
	accessKey := viper.GetString("storage.s3.access_key_id")
	secretKey := viper.GetString("storage.s3.secret_access_key")

	if viper.GetBool("storage.s3.anonymous") {
		accessKey = anonymousCredential
		secretKey = anonymousCredential
	}

	properties := iceberg.Properties{
		icebergio.S3EndpointURL:     s3Endpoint,
		icebergio.S3Region:          s3Region,
		icebergio.S3AccessKeyID:     accessKey,
		icebergio.S3SecretAccessKey: secretKey,
	}

	// Shared HTTP transport for both REST catalog and S3 blob operations.
	// Reusing connections prevents ephemeral TCP port exhaustion under high concurrency.
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

	httpClient := awshttp.NewBuildableClient().WithTransportOptions(func(t *http.Transport) {
		t.Proxy = http.ProxyFromEnvironment
		t.DialContext = (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext
		t.MaxIdleConns = 512
		t.MaxIdleConnsPerHost = 256
		t.MaxConnsPerHost = 256
		t.IdleConnTimeout = 90 * time.Second
	})

	awsOpts := []func(*config.LoadOptions) error{
		config.WithHTTPClient(httpClient),
	}

	if s3Region != "" {
		awsOpts = append(awsOpts, config.WithRegion(s3Region))
	}

	if accessKey != "" || secretKey != "" {
		awsOpts = append(awsOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
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
		rest.WithWarehouseLocation(warehouse),
		rest.WithAdditionalProps(properties),
		rest.WithCustomTransport(transport),
	}

	if err == nil {
		restOpts = append(restOpts, rest.WithAwsConfig(awsCfg))
	}

	connected, err := rest.NewCatalog(ctx, "seaweed", uri, restOpts...)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to connect to catalog "+uri,
			err,
		))

		return nil
	}

	cat := Wrap(connected)
	cat.awsConfig = &awsCfg

	return cat
}

/*
Ensure creates the Hindsight namespace and every table it owns if they are not
already present, and applies the configured Iceberg commit retry budget.

A concurrent creator racing to the same table is not an error: the loser sees
ErrTableAlreadyExists and proceeds against what the winner created, since both
would have written the identical schema.
*/
func (c *Catalog) Ensure(ctx context.Context) error {
	properties := iceberg.Properties{}

	if viper.IsSet("storage.iceberg.commit_retries") {
		retries, err := strconv.Atoi(viper.GetString("storage.iceberg.commit_retries"))

		if err != nil || retries < 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation, "[iceberg] commit_retries must be a nonnegative integer", err,
			))
		}

		properties[table.CommitNumRetriesKey] = strconv.Itoa(retries)
	}

	namespace := table.Identifier{Namespace}

	if err := c.catalog.CreateNamespace(ctx, namespace, nil); err != nil {
		if !errors.Is(err, catalog.ErrNamespaceAlreadyExists) {
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
		{SpotLevel3, SpotLevel3Schema(), SpotLevel3Partitioning()},
		{SpotTicker, SpotTickerSchema(), SpotTickerPartitioning()},
		{SpotTrade, SpotTradeSchema(), SpotTradePartitioning()},
		{FuturesTicker, FuturesTickerSchema(), FuturesTickerPartitioning()},
		{FuturesTrade, FuturesTradeSchema(), FuturesTradePartitioning()},
		{Executions, ExecutionsSchema(), ExecutionsPartitioning()},
		{Measurements, MeasurementsSchema(), MeasurementsPartitioning()},
		{Models, ModelsSchema(), ModelsPartitioning()},
		{Grids, GridsSchema(), GridsPartitioning()},
		{Positions, PositionsSchema(), PositionsPartitioning()},
		{Decisions, DecisionsSchema(), DecisionsPartitioning()},
		{Outcomes, OutcomesSchema(), OutcomesPartitioning()},
	}

	for _, family := range families {
		if err := c.ensureTable(ctx, family.name, family.schema, family.partitioning, properties); err != nil {
			return err
		}
	}

	return nil
}

// ensureTable creates one table if the catalog does not already hold it.
func (c *Catalog) ensureTable(
	ctx context.Context,
	name string,
	schema *iceberg.Schema,
	partitioning iceberg.PartitionSpec,
	properties iceberg.Properties,
) error {
	ctx = c.Context(ctx)
	identifier := table.Identifier{Namespace, name}
	exists, err := c.catalog.CheckTableExists(ctx, identifier)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to check table "+name,
			err,
		))
	}

	if exists {
		return c.configure(ctx, name, schema, properties)
	}

	if _, err := c.catalog.CreateTable(
		ctx, identifier, schema,
		catalog.WithPartitionSpec(&partitioning), catalog.WithProperties(properties),
	); err != nil {
		if errors.Is(err, catalog.ErrTableAlreadyExists) {
			return c.configure(ctx, name, schema, properties)
		}

		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to create table "+name,
			err,
		))
	}

	return nil
}

// configure updates existing tables when schema requires evolution or retry budget differs.
func (c *Catalog) configure(
	ctx context.Context,
	name string,
	schema *iceberg.Schema,
	properties iceberg.Properties,
) error {
	loaded, err := c.Load(ctx, name)

	if err != nil {
		return err
	}

	transaction := loaded.NewTransaction()
	needsCommit := false

	var missingFields []iceberg.NestedField

	for _, field := range schema.Fields() {
		if _, found := loaded.Schema().FindFieldByName(field.Name); !found {
			missingFields = append(missingFields, field)
		}
	}

	if len(missingFields) > 0 {
		updater := transaction.UpdateSchema(false, true)

		for _, field := range missingFields {
			updater.AddColumn([]string{field.Name}, field.Type, field.Doc, false, nil)
		}

		if err := updater.Commit(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[iceberg] evolve schema "+name,
				err,
			))
		}

		needsCommit = true
	}

	if len(properties) > 0 && loaded.Properties()[table.CommitNumRetriesKey] != properties[table.CommitNumRetriesKey] {
		if err := transaction.SetProperties(properties); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[iceberg] configure "+name,
				err,
			))
		}

		needsCommit = true
	}

	if !needsCommit {
		return nil
	}

	if _, err := transaction.Commit(ctx); err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] commit table updates "+name,
			err,
		))
	}

	return nil
}

// Load returns one of the Hindsight tables for reading or appending.
func (c *Catalog) Load(ctx context.Context, name string) (*table.Table, error) {
	loaded, err := c.catalog.LoadTable(c.Context(ctx), table.Identifier{Namespace, name})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to load table "+name,
			err,
		))
	}

	return loaded, nil
}
