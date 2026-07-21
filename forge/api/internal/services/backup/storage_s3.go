package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3StorageAdapter implements StorageAdapter for S3-compatible storage (AWS S3, MinIO, etc.)
type S3StorageAdapter struct {
	bucket       string
	prefix       string
	client       *s3.Client
	usePathStyle bool
}

// NewS3StorageAdapter creates a new S3StorageAdapter
func NewS3StorageAdapter(s3Config *S3StorageConfig) (*S3StorageAdapter, error) {
	if s3Config == nil {
		return nil, fmt.Errorf("S3 config is required")
	}

	region := strings.TrimSpace(s3Config.Region)
	if region == "" {
		region = "us-east-1"
	}

	endpoint := strings.TrimSpace(s3Config.Endpoint)
	bucket := strings.TrimSpace(s3Config.Bucket)

	// Create AWS config
	var opts []func(*config.LoadOptions) error
	opts = append(opts, config.WithRegion(region))
	opts = append(opts, config.WithCredentialsProvider(
		credentials.NewStaticCredentialsProvider(s3Config.AccessKeyID, s3Config.SecretAccessKey, ""),
	))

	cfg, err := config.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client options
	s3Opts := []func(*s3.Options){
		func(o *s3.Options) {
			o.UsePathStyle = s3Config.UsePathStyle
		},
	}

	if endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}

	client := s3.NewFromConfig(cfg, s3Opts...)

	return &S3StorageAdapter{
		bucket:       bucket,
		prefix:       strings.TrimRight(s3Config.Prefix, "/"),
		client:       client,
		usePathStyle: s3Config.UsePathStyle,
	}, nil
}

// Name returns the name of the storage adapter
func (a *S3StorageAdapter) Name() string {
	return "s3"
}

// key generates the S3 object key for a given path
func (a *S3StorageAdapter) key(path string) string {
	if a.prefix != "" {
		return a.prefix + "/" + strings.TrimLeft(path, "/")
	}
	return path
}

// Upload uploads data to S3 with retry
func (a *S3StorageAdapter) Upload(ctx context.Context, path string, data []byte) error {
	cfg := retryConfig{maxRetries: 3, baseBackoff: time.Second, maxBackoff: 30 * time.Second}
	err := withRetry(ctx, cfg, func(ctx context.Context) error {
		input := &s3.PutObjectInput{
			Bucket: aws.String(a.bucket),
			Key:    aws.String(a.key(path)),
			Body:   bytes.NewReader(data),
		}
		_, innerErr := a.client.PutObject(ctx, input)
		return innerErr
	})
	if err != nil {
		return fmt.Errorf("S3 upload failed after retries: %w", err)
	}
	return nil
}

// UploadStream uploads data from a stream to S3 with retry
func (a *S3StorageAdapter) UploadStream(ctx context.Context, path string, reader io.Reader, size int64) error {
	cfg := retryConfig{maxRetries: 3, baseBackoff: time.Second, maxBackoff: 30 * time.Second}
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("S3 upload stream read: %w", err)
	}
	err = withRetry(ctx, cfg, func(ctx context.Context) error {
		input := &s3.PutObjectInput{
			Bucket: aws.String(a.bucket),
			Key:    aws.String(a.key(path)),
			Body:   bytes.NewReader(data),
		}
		if size > 0 {
			input.ContentLength = aws.Int64(size)
		}
		_, innerErr := a.client.PutObject(ctx, input)
		return innerErr
	})
	if err != nil {
		return fmt.Errorf("S3 upload stream failed after retries: %w", err)
	}
	return nil
}

// Download downloads data from S3 with retry
func (a *S3StorageAdapter) Download(ctx context.Context, path string) ([]byte, error) {
	cfg := retryConfig{maxRetries: 3, baseBackoff: time.Second, maxBackoff: 30 * time.Second}
	var data []byte
	err := withRetry(ctx, cfg, func(ctx context.Context) error {
		input := &s3.GetObjectInput{
			Bucket: aws.String(a.bucket),
			Key:    aws.String(a.key(path)),
		}
		output, innerErr := a.client.GetObject(ctx, input)
		if innerErr != nil {
			if isS3NotFound(innerErr) {
				return innerErr
			}
			return innerErr
		}
		defer output.Body.Close()
		data, innerErr = io.ReadAll(output.Body)
		return innerErr
	})
	if err != nil {
		if isS3NotFound(err) {
			return nil, fmt.Errorf("S3 object not found: %s", path)
		}
		return nil, fmt.Errorf("S3 download failed after retries: %w", err)
	}
	return data, nil
}

// DownloadStream downloads data from S3 to a stream with retry
func (a *S3StorageAdapter) DownloadStream(ctx context.Context, path string) (io.Reader, error) {
	cfg := retryConfig{maxRetries: 3, baseBackoff: time.Second, maxBackoff: 30 * time.Second}
	var output *s3.GetObjectOutput
	err := withRetry(ctx, cfg, func(ctx context.Context) error {
		input := &s3.GetObjectInput{
			Bucket: aws.String(a.bucket),
			Key:    aws.String(a.key(path)),
		}
		var innerErr error
		output, innerErr = a.client.GetObject(ctx, input)
		return innerErr
	})
	if err != nil {
		if isS3NotFound(err) {
			return nil, fmt.Errorf("S3 object not found: %s", path)
		}
		return nil, fmt.Errorf("S3 download stream failed after retries: %w", err)
	}
	return &s3ReadCloser{ReadCloser: output.Body}, nil
}

// Delete deletes data from S3 with retry
func (a *S3StorageAdapter) Delete(ctx context.Context, path string) error {
	cfg := retryConfig{maxRetries: 3, baseBackoff: time.Second, maxBackoff: 30 * time.Second}
	err := withRetry(ctx, cfg, func(ctx context.Context) error {
		input := &s3.DeleteObjectInput{
			Bucket: aws.String(a.bucket),
			Key:    aws.String(a.key(path)),
		}
		_, innerErr := a.client.DeleteObject(ctx, input)
		return innerErr
	})
	if err != nil {
		return fmt.Errorf("S3 delete failed after retries: %w", err)
	}
	return nil
}

// List lists files in S3 with the given prefix with retry
func (a *S3StorageAdapter) List(ctx context.Context, prefix string) ([]string, error) {
	cfg := retryConfig{maxRetries: 3, baseBackoff: time.Second, maxBackoff: 30 * time.Second}
	fullPrefix := a.key(prefix)

	type listResult struct {
		names []string
		err   error
	}
	var result listResult

	err := withRetry(ctx, cfg, func(ctx context.Context) error {
		input := &s3.ListObjectsV2Input{
			Bucket: aws.String(a.bucket),
			Prefix: aws.String(fullPrefix),
		}
		paginator := s3.NewListObjectsV2Paginator(a.client, input)
		var names []string
		for paginator.HasMorePages() {
			page, innerErr := paginator.NextPage(ctx)
			if innerErr != nil {
				return innerErr
			}
			for _, obj := range page.Contents {
				if obj.Key != nil {
					relativePath := strings.TrimPrefix(*obj.Key, fullPrefix)
					if relativePath != "" && relativePath[0] == '/' {
						relativePath = relativePath[1:]
					}
					names = append(names, relativePath)
				}
			}
		}
		result.names = names
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("S3 list failed after retries: %w", err)
	}
	return result.names, nil
}

// Exists checks if a file exists in S3 with retry
func (a *S3StorageAdapter) Exists(ctx context.Context, path string) (bool, error) {
	cfg := retryConfig{maxRetries: 3, baseBackoff: time.Second, maxBackoff: 30 * time.Second}
	var exists bool
	err := withRetry(ctx, cfg, func(ctx context.Context) error {
		input := &s3.HeadObjectInput{
			Bucket: aws.String(a.bucket),
			Key:    aws.String(a.key(path)),
		}
		_, innerErr := a.client.HeadObject(ctx, input)
		if innerErr != nil {
			if isS3NotFound(innerErr) {
				exists = false
				return nil
			}
			return innerErr
		}
		exists = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("S3 head object failed after retries: %w", err)
	}
	return exists, nil
}

// GetFileInfo gets information about a file in S3 with retry
func (a *S3StorageAdapter) GetFileInfo(ctx context.Context, path string) (FileInfo, error) {
	cfg := retryConfig{maxRetries: 3, baseBackoff: time.Second, maxBackoff: 30 * time.Second}
	var info FileInfo
	err := withRetry(ctx, cfg, func(ctx context.Context) error {
		input := &s3.HeadObjectInput{
			Bucket: aws.String(a.bucket),
			Key:    aws.String(a.key(path)),
		}
		output, innerErr := a.client.HeadObject(ctx, input)
		if innerErr != nil {
			if isS3NotFound(innerErr) {
				return fmt.Errorf("S3 object not found: %s", path)
			}
			return innerErr
		}
		var modified time.Time
		if output.LastModified != nil {
			modified = *output.LastModified
		}
		info = FileInfo{
			Name:     filepath.Base(path),
			Path:     path,
			Size:     aws.ToInt64(output.ContentLength),
			Modified: modified,
			ETag:     aws.ToString(output.ETag),
			IsDir:    false,
		}
		return nil
	})
	if err != nil {
		return FileInfo{}, err
	}
	return info, nil
}

// isS3NotFound checks if an error is an S3 not found error
func isS3NotFound(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "NotFound"),
		strings.Contains(errStr, "NoSuchKey"),
		strings.Contains(errStr, "404"):
		return true
	}
	return false
}

// s3ReadCloser wraps the S3 GetObject output body to ensure it's closed
type s3ReadCloser struct {
	io.ReadCloser
}

// Close closes the underlying S3 response body
func (r *s3ReadCloser) Close() error {
	if r.ReadCloser != nil {
		return r.ReadCloser.Close()
	}
	return nil
}

// MinIOStorageAdapter implements StorageAdapter for MinIO storage
// MinIO is S3-compatible, so we can reuse the S3 adapter
type MinIOStorageAdapter struct {
	*S3StorageAdapter
}

// NewMinIOStorageAdapter creates a new MinIOStorageAdapter
func NewMinIOStorageAdapter(config *MinIOStorageConfig) (*MinIOStorageAdapter, error) {
	if config == nil {
		return nil, fmt.Errorf("MinIO config is required")
	}

	// Convert MinIO config to S3 config
	s3Config := &S3StorageConfig{
		Region:          "us-east-1", // MinIO typically doesn't care about region
		Endpoint:        config.Endpoint,
		Bucket:          config.Bucket,
		Prefix:          config.Prefix,
		AccessKeyID:     config.AccessKeyID,
		SecretAccessKey: config.SecretAccessKey,
		UsePathStyle:    config.UseSSL, // Use path style for MinIO
	}

	adapter, err := NewS3StorageAdapter(s3Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 adapter for MinIO: %w", err)
	}

	return &MinIOStorageAdapter{adapter}, nil
}

// Name returns the name of the storage adapter
func (a *MinIOStorageAdapter) Name() string {
	return "minio"
}

// AzureStorageAdapter implements StorageAdapter for Azure Blob Storage
type AzureStorageAdapter struct {
	container string
	prefix    string
	client    *azblob.Client
}

// NewAzureStorageAdapter creates a new AzureStorageAdapter
func NewAzureStorageAdapter(config *AzureStorageConfig) (*AzureStorageAdapter, error) {
	if config == nil {
		return nil, fmt.Errorf("Azure config is required")
	}

	containerName := strings.TrimSpace(config.Container)
	if containerName == "" {
		return nil, fmt.Errorf("Azure container name is required")
	}

	client, err := azblob.NewClientFromConnectionString(config.ConnectionString, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure client: %w", err)
	}

	return &AzureStorageAdapter{
		container: containerName,
		prefix:    strings.TrimRight(config.Prefix, "/"),
		client:    client,
	}, nil
}

// Name returns the name of the storage adapter
func (a *AzureStorageAdapter) Name() string {
	return "azure"
}

// blobPath generates the blob path for a given path
func (a *AzureStorageAdapter) blobPath(path string) string {
	if a.prefix != "" {
		return a.prefix + "/" + strings.TrimLeft(path, "/")
	}
	return path
}

// Upload uploads data to Azure Blob Storage
func (a *AzureStorageAdapter) Upload(ctx context.Context, path string, data []byte) error {
	_, err := a.client.UploadBuffer(ctx, a.container, a.blobPath(path), data, nil)
	if err != nil {
		return fmt.Errorf("Azure upload failed: %w", err)
	}
	return nil
}

// UploadStream uploads data from a stream to Azure Blob Storage
func (a *AzureStorageAdapter) UploadStream(ctx context.Context, path string, reader io.Reader, size int64) error {
	_, err := a.client.UploadStream(ctx, a.container, a.blobPath(path), reader, nil)
	if err != nil {
		return fmt.Errorf("Azure upload stream failed: %w", err)
	}
	return nil
}

// Download downloads data from Azure Blob Storage
func (a *AzureStorageAdapter) Download(ctx context.Context, path string) ([]byte, error) {
	resp, err := a.client.DownloadStream(ctx, a.container, a.blobPath(path), nil)
	if err != nil {
		return nil, fmt.Errorf("Azure download failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Azure read failed: %w", err)
	}
	return data, nil
}

// DownloadStream downloads data from Azure Blob Storage to a stream
func (a *AzureStorageAdapter) DownloadStream(ctx context.Context, path string) (io.Reader, error) {
	resp, err := a.client.DownloadStream(ctx, a.container, a.blobPath(path), nil)
	if err != nil {
		return nil, fmt.Errorf("Azure download stream failed: %w", err)
	}
	return resp.Body, nil
}

// Delete deletes data from Azure Blob Storage
func (a *AzureStorageAdapter) Delete(ctx context.Context, path string) error {
	_, err := a.client.DeleteBlob(ctx, a.container, a.blobPath(path), nil)
	if err != nil {
		return fmt.Errorf("Azure delete failed: %w", err)
	}
	return nil
}

// List lists files in Azure Blob Storage with the given prefix
func (a *AzureStorageAdapter) List(ctx context.Context, prefix string) ([]string, error) {
	blobPrefix := a.prefix
	if prefix != "" {
		if a.prefix != "" {
			blobPrefix = a.prefix + "/" + prefix
		} else {
			blobPrefix = prefix
		}
	}

	var names []string
	pager := a.client.NewListBlobsFlatPager(a.container, &azblob.ListBlobsFlatOptions{
		Prefix: &blobPrefix,
	})
	for pager.More() {
		listResp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("Azure list failed: %w", err)
		}
		for _, blob := range listResp.Segment.BlobItems {
			name := *blob.Name
			if strings.HasPrefix(name, a.prefix+"/") {
				name = strings.TrimPrefix(name, a.prefix+"/")
			} else if strings.HasPrefix(name, a.prefix) {
				name = strings.TrimPrefix(name, a.prefix)
			}
			if name != "" {
				names = append(names, name)
			}
		}
	}

	return names, nil
}

// Exists checks if a file exists in Azure Blob Storage
func (a *AzureStorageAdapter) Exists(ctx context.Context, path string) (bool, error) {
	blobClient := a.client.ServiceClient().NewContainerClient(a.container).NewBlobClient(a.blobPath(path))
	_, err := blobClient.GetProperties(ctx, nil)
	if err != nil {
		if bloberror.HasCode(err, bloberror.BlobNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("Azure exists check failed: %w", err)
	}
	return true, nil
}

// GetFileInfo gets information about a file in Azure Blob Storage
func (a *AzureStorageAdapter) GetFileInfo(ctx context.Context, path string) (FileInfo, error) {
	blobClient := a.client.ServiceClient().NewContainerClient(a.container).NewBlobClient(a.blobPath(path))
	props, err := blobClient.GetProperties(ctx, nil)
	if err != nil {
		return FileInfo{}, fmt.Errorf("Azure get file info failed: %w", err)
	}

	return FileInfo{
		Name:     filepath.Base(path),
		Path:     path,
		Size:     *props.ContentLength,
		Modified: *props.LastModified,
		IsDir:    false,
	}, nil
}

// GCSStorageAdapter implements StorageAdapter for Google Cloud Storage
// This adapter is a stub that returns an error. To use GCS, build with the GCS SDK.
type GCSStorageAdapter struct{}

// NewGCSStorageAdapter creates a new GCSStorageAdapter
func NewGCSStorageAdapter(config *GCSStorageConfig) (*GCSStorageAdapter, error) {
	return nil, fmt.Errorf("GCS storage adapter is not available in this build")
}

func (a *GCSStorageAdapter) Name() string {
	return "gcs"
}

func (a *GCSStorageAdapter) Upload(ctx context.Context, path string, data []byte) error {
	return fmt.Errorf("GCS storage adapter is not available in this build")
}

func (a *GCSStorageAdapter) UploadStream(ctx context.Context, path string, reader io.Reader, size int64) error {
	return fmt.Errorf("GCS storage adapter is not available in this build")
}

func (a *GCSStorageAdapter) Download(ctx context.Context, path string) ([]byte, error) {
	return nil, fmt.Errorf("GCS storage adapter is not available in this build")
}

func (a *GCSStorageAdapter) DownloadStream(ctx context.Context, path string) (io.Reader, error) {
	return nil, fmt.Errorf("GCS storage adapter is not available in this build")
}

func (a *GCSStorageAdapter) Delete(ctx context.Context, path string) error {
	return fmt.Errorf("GCS storage adapter is not available in this build")
}

func (a *GCSStorageAdapter) List(ctx context.Context, prefix string) ([]string, error) {
	return nil, fmt.Errorf("GCS storage adapter is not available in this build")
}

func (a *GCSStorageAdapter) Exists(ctx context.Context, path string) (bool, error) {
	return false, fmt.Errorf("GCS storage adapter is not available in this build")
}

func (a *GCSStorageAdapter) GetFileInfo(ctx context.Context, path string) (FileInfo, error) {
	return FileInfo{}, fmt.Errorf("GCS storage adapter is not available in this build")
}

// LocalStorageAdapter implements StorageAdapter for local filesystem storage
// This extends the existing LocalAdapter to implement the full StorageAdapter interface
type LocalStorageAdapter struct {
	basePath string
}

// NewLocalStorageAdapter creates a new LocalStorageAdapter
func NewLocalStorageAdapter(basePath string) (*LocalStorageAdapter, error) {
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("local adapter path: %w", err)
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return nil, fmt.Errorf("local adapter mkdir: %w", err)
	}
	return &LocalStorageAdapter{basePath: abs}, nil
}

// Name returns the name of the storage adapter
func (a *LocalStorageAdapter) Name() string {
	return "local"
}

// Upload uploads data to local storage
func (a *LocalStorageAdapter) Upload(ctx context.Context, path string, data []byte) error {
	fullPath := filepath.Join(a.basePath, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("local upload mkdir: %w", err)
	}
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return fmt.Errorf("local upload write: %w", err)
	}
	return nil
}

// UploadStream uploads data from a stream to local storage
func (a *LocalStorageAdapter) UploadStream(ctx context.Context, path string, reader io.Reader, size int64) error {
	fullPath := filepath.Join(a.basePath, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("local upload stream mkdir: %w", err)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("local upload stream create: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("local upload stream copy: %w", err)
	}

	return nil
}

// Download downloads data from local storage
func (a *LocalStorageAdapter) Download(ctx context.Context, path string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(a.basePath, path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("local file not found: %s", path)
		}
		return nil, fmt.Errorf("local download: %w", err)
	}
	return data, nil
}

// DownloadStream downloads data from local storage to a stream
func (a *LocalStorageAdapter) DownloadStream(ctx context.Context, path string) (io.Reader, error) {
	file, err := os.Open(filepath.Join(a.basePath, path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("local file not found: %s", path)
		}
		return nil, fmt.Errorf("local download stream: %w", err)
	}
	return file, nil
}

// Delete deletes data from local storage
func (a *LocalStorageAdapter) Delete(ctx context.Context, path string) error {
	if err := os.Remove(filepath.Join(a.basePath, path)); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("local delete: %w", err)
	}
	return nil
}

// List lists files in local storage with the given prefix
func (a *LocalStorageAdapter) List(ctx context.Context, prefix string) ([]string, error) {
	var names []string
	basePath := filepath.Join(a.basePath, prefix)

	entries, err := os.ReadDir(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return names, nil
		}
		return nil, fmt.Errorf("local list: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// Exists checks if a file exists in local storage
func (a *LocalStorageAdapter) Exists(ctx context.Context, path string) (bool, error) {
	_, err := os.Stat(filepath.Join(a.basePath, path))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("local exists: %w", err)
}

// GetFileInfo gets information about a file in local storage
func (a *LocalStorageAdapter) GetFileInfo(ctx context.Context, path string) (FileInfo, error) {
	fullPath := filepath.Join(a.basePath, path)

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return FileInfo{}, fmt.Errorf("local file not found: %s", path)
		}
		return FileInfo{}, fmt.Errorf("local get file info: %w", err)
	}

	return FileInfo{
		Name:     info.Name(),
		Path:     path,
		Size:     info.Size(),
		Modified: info.ModTime(),
		IsDir:    info.IsDir(),
	}, nil
}
