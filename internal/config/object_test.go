package config

import (
	"slices"
	"testing"
)

// A region is served by the endpoint its cloud gives it: OSS's by the
// region in the host, AWS's China regions by the domain of that partition.
func TestObjectRegionEndpoints(t *testing.T) {
	cases := []struct{ provider, region, want string }{
		{ObjectAWS, "us-east-1", "s3.amazonaws.com"},
		{ObjectAWS, "eu-west-1", "s3.eu-west-1.amazonaws.com"},
		{ObjectAWS, "cn-north-1", "s3.cn-north-1.amazonaws.com.cn"},
		{ObjectAWS, "cn-northwest-1", "s3.cn-northwest-1.amazonaws.com.cn"},
		{ObjectAliyunOSS, "cn-shanghai", "oss-cn-shanghai.aliyuncs.com"},
		{ObjectTencentCOS, "ap-shanghai", "cos.ap-shanghai.myqcloud.com"},
		{ObjectHuaweiOBS, "cn-north-4", "obs.cn-north-4.myhuaweicloud.com"},
		{ObjectQiniu, "cn-east-2", "s3-cn-east-2.qiniucs.com"},
	}
	for _, c := range cases {
		p := ObjectProviderOf(c.provider)
		if got := p.EndpointForRegion(c.region); got != c.want {
			t.Errorf("%s %s: %q, want %q", c.provider, c.region, got, c.want)
		}
	}
	// The regions of AWS China are offered, and each of the clouds' lists
	// builds an endpoint.
	aws := ObjectProviderOf(ObjectAWS)
	for _, r := range []string{"cn-north-1", "cn-northwest-1"} {
		if !slices.Contains(aws.Regions, r) {
			t.Errorf("%q is not offered", r)
		}
	}
	for _, p := range ObjectProviders {
		if !p.S3 {
			continue
		}
		for _, r := range p.Regions {
			if p.RegionPrefix != "" && p.EndpointForRegion(r) == "" {
				t.Errorf("%s has no endpoint for %q", p.ID, r)
			}
		}
	}
}

// The servers of one's own have a region only for the signature, which is
// not a location the editor picks from the clouds'.
func TestObjectSelfHosted(t *testing.T) {
	for _, id := range []string{ObjectMinIO, ObjectRustFS} {
		if p := ObjectProviderOf(id); !p.IsSelfHosted() {
			t.Errorf("%s is not self hosted", id)
		}
	}
	if ObjectProviderOf(ObjectAWS).IsSelfHosted() || ObjectProviderOf(ObjectAliyunOSS).IsSelfHosted() {
		t.Error("a cloud is self hosted")
	}
}
