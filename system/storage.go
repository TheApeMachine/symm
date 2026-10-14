package system

import "github.com/spf13/viper"

type Storage struct {
	S3 *S3
}

type S3 struct {
	Bucket          string
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Anonymous       bool
}

func NewStorage() *Storage {
	viper.SetDefault("storage.s3.bucket", "symm")
	viper.SetDefault("storage.s3.region", "us-east-1")
	viper.SetDefault("storage.s3.endpoint", "http://s3.seaweed.home.arpa")
	viper.SetDefault("storage.s3.anonymous", true)

	return &Storage{
		S3: &S3{
			Bucket:          viper.GetString("storage.s3.bucket"),
			Endpoint:        viper.GetString("storage.s3.endpoint"),
			Region:          viper.GetString("storage.s3.region"),
			AccessKeyID:     viper.GetString("storage.s3.access_key_id"),
			SecretAccessKey: viper.GetString("storage.s3.secret_access_key"),
			Anonymous:       viper.GetBool("storage.s3.anonymous"),
		},
	}
}
