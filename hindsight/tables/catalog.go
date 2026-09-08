package tables

import (
	"context"
	"errors"

	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/catalog/rest"
	icebergio "github.com/apache/iceberg-go/io"

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
}

/*
Wrap adapts any Iceberg catalog implementation. Production uses the REST
catalog SeaweedFS serves; tests use a SQLite catalog over a temporary
directory, which exercises the same schemas, encoders, and scans without
needing a catalog server.
*/
func Wrap(underlying catalog.Catalog) *Catalog { return &Catalog{catalog: underlying} }

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

	// The FileIO reads the data files directly, so it needs the same endpoint
	// and credential posture as the object store rather than the AWS defaults.
	properties := iceberg.Properties{
		icebergio.S3EndpointURL: viper.GetString("storage.s3.endpoint"),
		icebergio.S3Region:      viper.GetString("storage.s3.region"),
	}

	properties[icebergio.S3AccessKeyID] = viper.GetString("storage.s3.access_key_id")
	properties[icebergio.S3SecretAccessKey] = viper.GetString("storage.s3.secret_access_key")

	// An unauthenticated venue verifies no signature, but the SDK still needs
	// credentials to exist: given none it walks its whole provider chain and
	// ends up waiting on the EC2 instance metadata service, which is not there.
	// Static placeholders keep the request signed and local.
	if viper.GetBool("storage.s3.anonymous") {
		properties[icebergio.S3AccessKeyID] = anonymousCredential
		properties[icebergio.S3SecretAccessKey] = anonymousCredential
	}

	connected, err := rest.NewCatalog(
		ctx, "seaweed", uri,
		rest.WithWarehouseLocation(warehouse),
		rest.WithAdditionalProps(properties),
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to connect to catalog "+uri,
			err,
		))

		return nil
	}

	return &Catalog{catalog: connected}
}

/*
Ensure creates the Hindsight namespace and every table it owns if they are not
already present, and is safe to call on every boot.

A concurrent creator racing to the same table is not an error: the loser sees
ErrTableAlreadyExists and proceeds against what the winner created, since both
would have written the identical schema.
*/
func (c *Catalog) Ensure(ctx context.Context) error {
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
		{Runs, RunsSchema(), RunsPartitioning()},
		{Captures, CapturesSchema(), CapturesPartitioning()},
		{Manifests, ManifestsSchema(), ManifestsPartitioning()},
		{Witnesses, WitnessesSchema(), WitnessesPartitioning()},
		{Lifecycle, LifecycleSchema(), LifecyclePartitioning()},
		{Decisions, DecisionsSchema(), DecisionsPartitioning()},
		{Outcomes, OutcomesSchema(), OutcomesPartitioning()},
		{Gaps, GapsSchema(), GapsPartitioning()},
	}

	for _, family := range families {
		if err := c.ensureTable(ctx, family.name, family.schema, family.partitioning); err != nil {
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
) error {
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
		return nil
	}

	if _, err := c.catalog.CreateTable(
		ctx, identifier, schema, catalog.WithPartitionSpec(&partitioning),
	); err != nil {
		if errors.Is(err, catalog.ErrTableAlreadyExists) {
			return nil
		}

		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to create table "+name,
			err,
		))
	}

	return nil
}

// Load returns one of the Hindsight tables for reading or appending.
func (c *Catalog) Load(ctx context.Context, name string) (*table.Table, error) {
	loaded, err := c.catalog.LoadTable(ctx, table.Identifier{Namespace, name})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to load table "+name,
			err,
		))
	}

	return loaded, nil
}
