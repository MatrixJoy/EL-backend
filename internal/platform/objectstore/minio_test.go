package objectstore

import (
	"testing"

	"github.com/minio/minio-go/v7"
)

func TestTencentCOSUsesDNSBucketLookup(t *testing.T) {
	if got := bucketLookup("cos.ap-guangzhou.myqcloud.com"); got != minio.BucketLookupDNS {
		t.Fatalf("bucket lookup = %v, want DNS", got)
	}
	if got := bucketLookup("minio:9000"); got != minio.BucketLookupAuto {
		t.Fatalf("MinIO bucket lookup = %v, want auto", got)
	}
}
