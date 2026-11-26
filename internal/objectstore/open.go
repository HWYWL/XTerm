package objectstore

import (
	"fmt"
	"strings"
	"time"
)

// Options are how a storage is opened: the provider's name, where it is,
// and the credential.
type Options struct {
	Provider  string
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	Root      string
	LocalPath string
	PathStyle bool
	Insecure  bool
	Timeout   time.Duration
}

// Open opens the storage the options describe, from the root the host
// shows.
func Open(opts Options) (Store, error) {
	var s Store
	var err error
	switch strings.ToLower(strings.TrimSpace(opts.Provider)) {
	case "local":
		s, err = OpenLocal(opts.LocalPath)
	case "azure":
		s, err = NewAzure(AzureConfig{
			Endpoint: opts.Endpoint, Container: opts.Bucket,
			AccessKey: opts.SecretKey, Timeout: opts.Timeout,
		})
	default:
		s, err = NewS3(S3Config{
			Endpoint: opts.Endpoint, Region: opts.Region, Bucket: opts.Bucket,
			AccessKey: opts.AccessKey, SecretKey: opts.SecretKey,
			PathStyle: opts.PathStyle, Insecure: opts.Insecure, Timeout: opts.Timeout,
		})
	}
	if err != nil {
		return nil, err
	}
	return WithRoot(s, opts.Root), nil
}

// ProviderS3 tells whether the provider is reached with the S3 protocol.
func ProviderS3(provider string) bool {
	switch strings.ToLower(provider) {
	case "local", "azure":
		return false
	}
	return true
}

// ProviderName is the name the editor shows for a provider.
func ProviderName(provider string) string {
	switch strings.ToLower(provider) {
	case "aliyun":
		return "阿里云 OSS"
	case "tencent":
		return "腾讯云 COS"
	case "huawei":
		return "华为云 OBS"
	case "qiniu":
		return "七牛云 Kodo"
	case "aws":
		return "AWS S3"
	case "gcs":
		return "Google Cloud Storage"
	case "azure":
		return "Azure Blob"
	case "minio":
		return "MinIO"
	case "rustfs":
		return "RustFS"
	case "local":
		return "本地文件系统"
	}
	return fmt.Sprint(provider)
}
