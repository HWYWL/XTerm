package config

import "strings"

// ProtocolObject is the protocol of an object storage: a bucket of a
// cloud, or a directory, shown as files (see redis.go for the others).
const ProtocolObject = "object"

// The object storages the app speaks to. The first seven are reached with
// the S3 protocol, which they serve as their own; Azure and the local file
// system have protocols of their own.
const (
	ObjectAliyunOSS  = "aliyun"
	ObjectTencentCOS = "tencent"
	ObjectHuaweiOBS  = "huawei"
	ObjectQiniu      = "qiniu"
	ObjectAWS        = "aws"
	ObjectGCS        = "gcs"
	ObjectMinIO      = "minio"
	ObjectRustFS     = "rustfs"
	ObjectAzure      = "azure"
	ObjectLocal      = "local"
)

// ObjectProvider is a storage the editor offers: its name, the fields it
// starts with, whether it is the S3 protocol, and the regions its endpoint
// is built from — "oss-" plus the region plus ".aliyuncs.com", as OSS has
// it.
type ObjectProvider struct {
	ID        string
	Name      string
	Endpoint  string // the endpoint to start with
	Region    string
	Regions   []string // the regions the editor offers, the first as default
	PathStyle bool     // whether the bucket is a path, as MinIO wants it
	S3        bool     // whether the S3 protocol reaches it
	Local     bool     // whether it is a directory of this machine

	// RegionPrefix and RegionSuffix build the endpoint of a region:
	// RegionPrefix+region+RegionSuffix, "" for the servers whose endpoint
	// is one for every region.
	RegionPrefix string
	RegionSuffix string
}

// ObjectProviders are the storages, in the order the editor lists them.
var ObjectProviders = []ObjectProvider{
	{ID: ObjectAliyunOSS, Name: "阿里云 OSS", Endpoint: "oss-cn-hangzhou.aliyuncs.com", Region: "cn-hangzhou", S3: true,
		Regions:      []string{"cn-hangzhou", "cn-shanghai", "cn-beijing", "cn-shenzhen", "cn-chengdu", "cn-hongkong", "ap-southeast-1", "us-west-1", "eu-central-1"},
		RegionPrefix: "oss-", RegionSuffix: ".aliyuncs.com"},
	{ID: ObjectTencentCOS, Name: "腾讯云 COS", Endpoint: "cos.ap-guangzhou.myqcloud.com", Region: "ap-guangzhou", S3: true,
		Regions:      []string{"ap-guangzhou", "ap-shanghai", "ap-beijing", "ap-chengdu", "ap-nanjing", "ap-hongkong", "ap-singapore", "na-siliconvalley", "eu-frankfurt"},
		RegionPrefix: "cos.", RegionSuffix: ".myqcloud.com"},
	{ID: ObjectHuaweiOBS, Name: "华为云 OBS", Endpoint: "obs.cn-north-4.myhuaweicloud.com", Region: "cn-north-4", S3: true,
		Regions:      []string{"cn-north-4", "cn-north-1", "cn-east-3", "cn-south-1", "cn-southwest-2", "ap-southeast-1", "eu-west-0"},
		RegionPrefix: "obs.", RegionSuffix: ".myhuaweicloud.com"},
	{ID: ObjectQiniu, Name: "七牛云 Kodo", Endpoint: "s3-cn-east-1.qiniucs.com", Region: "cn-east-1", S3: true,
		Regions:      []string{"cn-east-1", "cn-east-2", "cn-north-1", "cn-south-1", "ap-southeast-1"},
		RegionPrefix: "s3-", RegionSuffix: ".qiniucs.com"},
	{ID: ObjectAWS, Name: "AWS S3", Endpoint: "s3.amazonaws.com", Region: "us-east-1", S3: true,
		Regions:      []string{"us-east-1", "us-east-2", "us-west-1", "us-west-2", "ca-central-1", "eu-west-1", "eu-central-1", "eu-north-1", "ap-south-1", "ap-northeast-1", "ap-southeast-1", "ap-southeast-2", "sa-east-1", "cn-north-1", "cn-northwest-1"},
		RegionPrefix: "s3.", RegionSuffix: ".amazonaws.com"},
	{ID: ObjectGCS, Name: "Google Cloud Storage", Endpoint: "storage.googleapis.com", Region: "auto", S3: true,
		Regions: []string{"auto"}},
	{ID: ObjectAzure, Name: "Azure Blob", Endpoint: "blob.core.windows.net"},
	{ID: ObjectMinIO, Name: "MinIO", Endpoint: "http://127.0.0.1:9000", Region: "us-east-1", PathStyle: true, S3: true,
		Regions: []string{"us-east-1"}},
	{ID: ObjectRustFS, Name: "RustFS", Endpoint: "http://127.0.0.1:9000", Region: "us-east-1", PathStyle: true, S3: true,
		Regions: []string{"us-east-1"}},
	{ID: ObjectLocal, Name: "本地文件系统", Local: true},
}

// EndpointForRegion is the endpoint a region is served by, "" when the
// server has one for every region.
func (p ObjectProvider) EndpointForRegion(region string) string {
	if p.RegionPrefix == "" || region == "" {
		return ""
	}
	if p.ID == ObjectAWS {
		switch {
		case region == "us-east-1":
			return "s3.amazonaws.com" // the bucket, not the region, is in the host
		case strings.HasPrefix(region, "cn-"):
			// The regions of AWS China are another partition, with its own
			// domain.
			return "s3." + region + ".amazonaws.com.cn"
		}
	}
	return p.RegionPrefix + region + p.RegionSuffix
}

// IsSelfHosted tells whether the provider is a server of one's own — MinIO
// or RustFS — whose region is only what it signs with.
func (p ObjectProvider) IsSelfHosted() bool {
	return p.ID == ObjectMinIO || p.ID == ObjectRustFS
}

// ObjectProviderOf returns the provider with id, the first for one the app
// does not know.
func ObjectProviderOf(id string) ObjectProvider {
	for _, p := range ObjectProviders {
		if p.ID == id {
			return p
		}
	}
	return ObjectProviders[0]
}

// ObjectSettings are the fields of a storage: where it is, the bucket or
// container, the credential, and the folder of it the host shows.
type ObjectSettings struct {
	Provider  string `json:"provider,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	Region    string `json:"region,omitempty"`
	Bucket    string `json:"bucket,omitempty"`
	AccessKey string `json:"accessKey,omitempty"`
	// Root is the prefix the host opens on, as "backups/2026".
	Root string `json:"root,omitempty"`
	// LocalPath is the directory a local storage shows.
	LocalPath string `json:"localPath,omitempty"`
	// PathStyle puts the bucket in the path, as MinIO and RustFS want it;
	// the other servers take it in the host name.
	PathStyle bool `json:"pathStyle,omitempty"`
	// Insecure allows an endpoint of http, as a local server's.
	Insecure bool `json:"insecure,omitempty"`
}

// IsObject tells whether the host is an object storage; false for nil.
func (h *Host) IsObject() bool { return h != nil && h.Protocol == ProtocolObject }

// ObjectProvider is the host's provider, the first for one it does not
// know.
func (h *Host) ObjectProvider() ObjectProvider {
	if h == nil || h.Object == nil {
		return ObjectProviders[0]
	}
	return ObjectProviderOf(h.Object.Provider)
}

// ObjectRoot is the prefix the host opens on, without the slashes around
// it.
func (h *Host) ObjectRoot() string {
	if h == nil || h.Object == nil {
		return ""
	}
	return strings.Trim(h.Object.Root, "/")
}
