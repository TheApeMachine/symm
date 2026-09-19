package tables

import "github.com/spf13/viper"

type StorageConfig struct {
	Iceberg IcebergConfig
	S3      S3Config
}

type IcebergConfig struct {
	URI           string
	Warehouse     string
	CommitRetries int
	AppendBytes   int
}

type S3Config struct {
	Bucket          string
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Anonymous       bool
}

func DefaultStorageConfig() *StorageConfig {
	viper.SetDefault("storage.iceberg.uri", "http://iceberg.seaweed.home.arpa")
	viper.SetDefault("storage.iceberg.warehouse", "s3://symmtables/")
	viper.SetDefault("storage.iceberg.commit_retries", 3)
	viper.SetDefault("storage.iceberg.append_bytes", 8388608)
	viper.SetDefault("storage.s3.bucket", "symm")

	return &StorageConfig{
		Iceberg: IcebergConfig{
			URI:           viper.GetString("storage.iceberg.uri"),
			Warehouse:     viper.GetString("storage.iceberg.warehouse"),
			CommitRetries: viper.GetInt("storage.iceberg.commit_retries"),
			AppendBytes:   viper.GetInt("storage.iceberg.append_bytes"),
		},
		S3: S3Config{
			Bucket:          viper.GetString("storage.s3.bucket"),
			Endpoint:        viper.GetString("storage.s3.endpoint"),
			Region:          viper.GetString("storage.s3.region"),
			AccessKeyID:     viper.GetString("storage.s3.access_key_id"),
			SecretAccessKey: viper.GetString("storage.s3.secret_access_key"),
			Anonymous:       viper.GetBool("storage.s3.anonymous"),
		},
	}
}
