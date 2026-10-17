package system

import (
	"time"

	"github.com/spf13/viper"
)

type Storage struct {
	S3 *S3
}

type S3 struct {
	Bucket                string
	Endpoint              string
	Region                string
	AccessKeyID           string
	SecretAccessKey       string
	Anonymous             bool
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
	IdleConnTimeout       time.Duration
	ResponseHeaderTimeout time.Duration
	RequestTimeout        time.Duration
}

func NewStorage() *Storage {
	viper.SetDefault("storage.s3.bucket", "symm")
	viper.SetDefault("storage.s3.region", "us-east-1")
	viper.SetDefault("storage.s3.endpoint", "http://s3.seaweed.home.arpa")
	viper.SetDefault("storage.s3.anonymous", true)
	viper.SetDefault("storage.s3.max_idle_conns", 128)
	viper.SetDefault("storage.s3.max_idle_conns_per_host", 128)
	viper.SetDefault("storage.s3.max_conns_per_host", 128)
	viper.SetDefault("storage.s3.idle_conn_timeout", 90*time.Second)
	viper.SetDefault("storage.s3.response_header_timeout", 30*time.Second)
	viper.SetDefault("storage.s3.request_timeout", 10*time.Second)

	return &Storage{
		S3: &S3{
			Bucket:                viper.GetString("storage.s3.bucket"),
			Endpoint:              viper.GetString("storage.s3.endpoint"),
			Region:                viper.GetString("storage.s3.region"),
			AccessKeyID:           viper.GetString("storage.s3.access_key_id"),
			SecretAccessKey:       viper.GetString("storage.s3.secret_access_key"),
			Anonymous:             viper.GetBool("storage.s3.anonymous"),
			MaxIdleConns:          viper.GetInt("storage.s3.max_idle_conns"),
			MaxIdleConnsPerHost:   viper.GetInt("storage.s3.max_idle_conns_per_host"),
			MaxConnsPerHost:       viper.GetInt("storage.s3.max_conns_per_host"),
			IdleConnTimeout:       viper.GetDuration("storage.s3.idle_conn_timeout"),
			ResponseHeaderTimeout: viper.GetDuration("storage.s3.response_header_timeout"),
			RequestTimeout:        viper.GetDuration("storage.s3.request_timeout"),
		},
	}
}
