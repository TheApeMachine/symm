package tables

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
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
)

const anonymousCredential = "anonymous"

/*
Catalog manages connections and tables for generic Iceberg operations.
Zero domain logic, pure generic Iceberg catalog interface.
*/
type Catalog struct {
	underlying    icecat.Catalog
	awsConfig     *aws.Config
	storageConfig *StorageConfig
	mu            sync.RWMutex
}

/*
Wrap adapts an Iceberg catalog implementation (REST for production, SQLite for tests).
*/
func Wrap(underlying icecat.Catalog) *Catalog {
	return &Catalog{
		underlying: underlying,
	}
}

func (catalog *Catalog) SetStorageConfig(cfg *StorageConfig) {
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	catalog.storageConfig = cfg
}

/*
Open connects to the Iceberg REST catalog configured in StorageConfig.
*/
func Open(ctx context.Context) *Catalog {
	storageConfig := DefaultStorageConfig()
	icebergConfig := &storageConfig.Iceberg
	s3Config := &storageConfig.S3

	if icebergConfig.URI == "" || icebergConfig.Warehouse == "" {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[iceberg] storage.iceberg.uri and storage.iceberg.warehouse are both required",
			nil,
		))
		return nil
	}

	accessKey := s3Config.AccessKeyID
	secretKey := s3Config.SecretAccessKey

	if s3Config.Anonymous {
		accessKey = anonymousCredential
		secretKey = anonymousCredential
	}

	properties := iceberg.Properties{
		icebergio.S3EndpointURL:     s3Config.Endpoint,
		icebergio.S3Region:          s3Config.Region,
		icebergio.S3AccessKeyID:     accessKey,
		icebergio.S3SecretAccessKey: secretKey,
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

	connected, err := rest.NewCatalog(ctx, "default", icebergConfig.URI, restOpts...)
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
	cat.storageConfig = storageConfig

	return cat
}

func (catalog *Catalog) ensureBuckets(ctx context.Context) error {
	if catalog.storageConfig == nil {
		return nil
	}
	return ensureStorageBuckets(ctx, &catalog.storageConfig.S3, &catalog.storageConfig.Iceberg)
}

func ensureStorageBuckets(
	ctx context.Context,
	s3Config *S3Config,
	icebergConfig *IcebergConfig,
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

	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to execute table bucket request for "+name,
			err,
		))
	}

	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusCreated || res.StatusCode == http.StatusConflict {
		return nil
	}

	return errnie.Error(errnie.Err(
		errnie.BadGateway,
		fmt.Sprintf("[iceberg] unexpected response creating table bucket %s: %d", name, res.StatusCode),
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
			"[iceberg] failed to execute bucket request for "+name,
			err,
		))
	}

	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusCreated || res.StatusCode == http.StatusConflict {
		return nil
	}

	return errnie.Error(errnie.Err(
		errnie.BadGateway,
		fmt.Sprintf("[iceberg] unexpected response creating bucket %s: %d", name, res.StatusCode),
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
Load loads an Iceberg table by namespace and table name.
*/
func (catalog *Catalog) Load(ctx context.Context, namespace, name string) (*table.Table, error) {
	loaded, err := catalog.underlying.LoadTable(catalog.context(ctx), table.Identifier{namespace, name})
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.BadGateway,
			fmt.Sprintf("[iceberg] failed to load table %s.%s", namespace, name),
			err,
		))
	}
	return loaded, nil
}

/*
CreateTable creates or loads an Iceberg table generically.
*/
func (catalog *Catalog) CreateTable(
	ctx context.Context,
	namespace, name string,
	schema *iceberg.Schema,
) (*table.Table, error) {
	ctx = catalog.context(ctx)
	ident := table.Identifier{namespace, name}

	exists, err := catalog.underlying.CheckTableExists(ctx, ident)
	if err == nil && exists {
		return catalog.Load(ctx, namespace, name)
	}

	tbl, err := catalog.underlying.CreateTable(ctx, ident, schema, nil)
	if err != nil {
		if errors.Is(err, icecat.ErrTableAlreadyExists) {
			return catalog.Load(ctx, namespace, name)
		}
		return nil, errnie.Error(errnie.Err(
			errnie.BadGateway,
			fmt.Sprintf("[iceberg] failed to create table %s.%s", namespace, name),
			err,
		))
	}

	return tbl, nil
}

func (catalog *Catalog) context(ctx context.Context) context.Context {
	if catalog.awsConfig == nil {
		return ctx
	}
	return utils.WithAwsConfig(ctx, catalog.awsConfig)
}
