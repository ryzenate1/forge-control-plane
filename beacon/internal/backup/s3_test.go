package backup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestS3CreateRetrySuccessAfterFailureAndCleansStaging(t *testing.T) {
	base := t.TempDir()
	serverRoot := filepath.Join(base, "servers", "server-one")
	if err := os.MkdirAll(serverRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverRoot, "world.dat"), []byte("world"), 0o640); err != nil {
		t.Fatal(err)
	}
	local, err := NewLocalBackup(filepath.Join(base, "staging"))
	if err != nil {
		t.Fatal(err)
	}
	uploader := &mockUploader{failures: 1}
	adapter := &S3Backup{
		config: &S3Config{Bucket: "bucket", Prefix: "backups"}, client: &mockS3{},
		uploader: uploader, local: local, retryBase: time.Millisecond,
	}
	info, err := adapter.Create(context.Background(), serverRoot, "server-one", "backup.zip", nil)
	if err != nil {
		t.Fatal(err)
	}
	if uploader.calls != 2 {
		t.Fatalf("expected two upload attempts, got %d", uploader.calls)
	}
	if info.Adapter != S3Adapter || info.RemotePath != "backups/server-one/backup.zip" || info.Checksum == "" {
		t.Fatalf("unexpected S3 backup info: %+v", info)
	}
	if uploader.metadata[checksumMetadataKey] != info.Checksum {
		t.Fatalf("checksum metadata not uploaded: %+v", uploader.metadata)
	}
	archivePath, err := local.archivePath("server-one", "backup.zip", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(archivePath); !os.IsNotExist(err) {
		t.Fatalf("successful upload staging was not cleaned: %v", err)
	}
	if _, err := os.Stat(archivePath + metadataSuffix); !os.IsNotExist(err) {
		t.Fatalf("successful upload metadata was not cleaned: %v", err)
	}
}

func TestS3ListPaginatesAndPreservesChecksumMetadata(t *testing.T) {
	now := time.Now().UTC()
	client := &mockS3{
		pages: []*s3.ListObjectsV2Output{
			{Contents: []types.Object{{Key: aws.String("prefix/server-one/first.zip"), Size: aws.Int64(10), LastModified: aws.Time(now)}}, IsTruncated: aws.Bool(true), NextContinuationToken: aws.String("page-two")},
			{Contents: []types.Object{{Key: aws.String("prefix/server-one/second.zip"), Size: aws.Int64(20), LastModified: aws.Time(now.Add(time.Second))}}, IsTruncated: aws.Bool(false)},
		},
		headMetadata: map[string]map[string]string{
			"prefix/server-one/first.zip":  {checksumMetadataKey: "first-sum"},
			"prefix/server-one/second.zip": {"Sha256": "second-sum"},
		},
	}
	adapter := &S3Backup{config: &S3Config{Bucket: "bucket", Prefix: "prefix"}, client: client}
	backups, err := adapter.List("server-one")
	if err != nil {
		t.Fatal(err)
	}
	if client.listCalls != 2 || len(backups) != 2 {
		t.Fatalf("pagination failed: calls=%d backups=%+v", client.listCalls, backups)
	}
	if backups[0].Checksum != "first-sum" || backups[1].Checksum != "second-sum" {
		t.Fatalf("checksum metadata lost: %+v", backups)
	}
}

func TestS3DownloadRejectsChecksumMismatchAndCleansStaging(t *testing.T) {
	base := t.TempDir()
	local, err := NewLocalBackup(filepath.Join(base, "staging"))
	if err != nil {
		t.Fatal(err)
	}
	client := &mockS3{getBody: []byte("archive"), getMetadata: map[string]string{checksumMetadataKey: "wrong"}}
	adapter := &S3Backup{config: &S3Config{Bucket: "bucket"}, client: client, local: local}
	_, err = adapter.Download("server-one", "backup.zip")
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
	entries, readErr := os.ReadDir(filepath.Join(base, "staging", "server-one"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed download left staging files: %+v", entries)
	}
}

func TestConfigureS3OptionsHonorsPathStyleWithAndWithoutEndpoint(t *testing.T) {
	for _, test := range []struct {
		name      string
		endpoint  string
		pathStyle bool
	}{
		{name: "AWS path style", pathStyle: true},
		{name: "custom endpoint virtual host", endpoint: "https://objects.example.test", pathStyle: false},
		{name: "custom endpoint path style", endpoint: "https://objects.example.test", pathStyle: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := &s3.Options{}
			configureS3Options(options, &S3Config{Endpoint: test.endpoint, UsePathStyle: test.pathStyle})
			if options.UsePathStyle != test.pathStyle {
				t.Fatalf("path style=%t, want %t", options.UsePathStyle, test.pathStyle)
			}
			if test.endpoint != "" && (options.BaseEndpoint == nil || *options.BaseEndpoint != test.endpoint) {
				t.Fatalf("endpoint not configured: %v", options.BaseEndpoint)
			}
		})
	}
}

type mockUploader struct {
	calls    int
	failures int
	metadata map[string]string
}

func (m *mockUploader) Upload(_ context.Context, input *s3.PutObjectInput, _ ...func(*manager.Uploader)) (*manager.UploadOutput, error) {
	m.calls++
	if m.calls <= m.failures {
		return nil, errors.New("temporary upload failure")
	}
	if _, err := io.Copy(io.Discard, input.Body); err != nil {
		return nil, err
	}
	m.metadata = input.Metadata
	return &manager.UploadOutput{}, nil
}

type mockS3 struct {
	pages        []*s3.ListObjectsV2Output
	listCalls    int
	headMetadata map[string]map[string]string
	getBody      []byte
	getMetadata  map[string]string
}

func (m *mockS3) ListObjectsV2(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if m.listCalls >= len(m.pages) {
		return &s3.ListObjectsV2Output{}, nil
	}
	page := m.pages[m.listCalls]
	m.listCalls++
	return page, nil
}

func (m *mockS3) HeadObject(_ context.Context, input *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	metadata := m.headMetadata[aws.ToString(input.Key)]
	return &s3.HeadObjectOutput{Metadata: metadata, ContentLength: aws.Int64(1), LastModified: aws.Time(time.Now().UTC())}, nil
}

func (m *mockS3) GetObject(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(m.getBody)), Metadata: m.getMetadata}, nil
}

func (m *mockS3) DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	return &s3.DeleteObjectOutput{}, nil
}
