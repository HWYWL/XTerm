package app

import (
	"testing"

	"finallyshell/internal/config"
	"finallyshell/internal/icons"

	"github.com/egoist/mygo/ui"
)

// objectHost is a host of a bucket of provider id.
func objectHost(id string) *config.Host {
	return &config.Host{
		Protocol: config.ProtocolObject,
		Object:   &config.ObjectSettings{Provider: id, Bucket: "b"},
	}
}

// A host's logo says what it connects to: the distribution of a shell, a
// Redis server, or the cloud of the bucket. The ones an icon set has none
// of keep the server's.
func TestHostBrands(t *testing.T) {
	cases := []struct {
		what string
		h    *config.Host
		want string
	}{
		{"a shell", &config.Host{Addr: "10.0.0.1"}, "linux"},
		{"a Redis server", &config.Host{Protocol: config.ProtocolRedis}, "redis"},
		{"Aliyun OSS", objectHost(config.ObjectAliyunOSS), "alibabacloud"},
		{"Tencent COS", objectHost(config.ObjectTencentCOS), "tencentcloud"},
		{"Huawei OBS", objectHost(config.ObjectHuaweiOBS), "huaweicloud"},
		{"Qiniu Kodo", objectHost(config.ObjectQiniu), "qiniu"},
		{"AWS S3", objectHost(config.ObjectAWS), "aws"},
		{"Google Cloud Storage", objectHost(config.ObjectGCS), "googlecloud"},
		{"Azure Blob", objectHost(config.ObjectAzure), "azure"},
		{"MinIO", objectHost(config.ObjectMinIO), "minio"},
		{"RustFS", objectHost(config.ObjectRustFS), ""},
		{"a directory", objectHost(config.ObjectLocal), ""},
	}
	for _, c := range cases {
		got, _ := hostBrand(c.h)
		if got != c.want {
			t.Errorf("%s: %q, want %q", c.what, got, c.want)
			continue
		}
		if got != "" && icons.Brand(got) == nil {
			t.Errorf("%s: no logo %q", c.what, got)
		}
	}
	if name, _ := hostBrand(nil); name != "" {
		t.Errorf("no host: %q", name)
	}
}

// The connections tree draws those logos: one row per protocol and cloud,
// each under the name it connects to.
func TestConnectionTreeBrands(t *testing.T) {
	a, p := testApp(t)
	p.SetLocale("en")
	a.hosts.Folders = []*config.Folder{{ID: "cloud", Name: "Cloud"}}
	for _, h := range []*config.Host{
		{ID: "web", Name: "web-1", Addr: "203.0.113.10", User: "root", Auth: config.AuthKey},
		{ID: "cache", Name: "cache", Addr: "10.0.0.7", Protocol: config.ProtocolRedis, Auth: config.AuthKey},
		objectHostWith("oss", "Alibaba", config.ObjectAliyunOSS, "cloud"),
		objectHostWith("aws", "AWS bucket", config.ObjectAWS, "cloud"),
		objectHostWith("cos", "Tencent bucket", config.ObjectTencentCOS, "cloud"),
		objectHostWith("obs", "Huawei bucket", config.ObjectHuaweiOBS, "cloud"),
		objectHostWith("kodo", "Qiniu bucket", config.ObjectQiniu, "cloud"),
		objectHostWith("gcs", "Google bucket", config.ObjectGCS, "cloud"),
		objectHostWith("azure", "Azure container", config.ObjectAzure, "cloud"),
		objectHostWith("minio", "MinIO", config.ObjectMinIO, "cloud"),
		objectHostWith("dir", "D:\\files", config.ObjectLocal, "cloud"),
	} {
		a.hosts.Add(h)
	}
	a.connTree.tree.Open.Add(managerItem{folder: "cloud"})
	a.restoreLeftPanel()
	a.setLeftPage(leftConnections)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.SetScale(2)
	p.tt = tt
	tt.Frame()
	saveShot(t, tt, "tree-brands-en.png")
}

// objectHostWith is objectHost in a folder, named name.
func objectHostWith(id, name, provider, folder string) *config.Host {
	h := objectHost(provider)
	h.ID, h.Name, h.Folder = id, name, folder
	h.Auth = config.AuthKey
	return h
}
