package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const checksumMetadataKey = "sha256"

type S3Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	Prefix          string
	UsePathStyle    bool
	BackupRoot      string
}

type s3API interface {
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

type s3Uploader interface {
	Upload(context.Context, *s3.PutObjectInput, ...func(*manager.Uploader)) (*manager.UploadOutput, error)
}

type S3Backup struct {
	config    *S3Config
	client    s3API
	uploader  s3Uploader
	local     *LocalBackup
	retryBase time.Duration
	mu        sync.Mutex
	progress  ProgressFunc
}

// NewS3Backup fails closed: a selected S3 adapter is never returned without a
// usable SDK configuration and daemon-owned local staging root.
func NewS3Backup(config *S3Config) (*S3Backup, error) {
	if config == nil {
		return nil, errors.New("S3 configuration is required")
	}
	if strings.TrimSpace(config.Region) == "" || strings.TrimSpace(config.Bucket) == "" || strings.TrimSpace(config.BackupRoot) == "" {
		return nil, errors.New("S3 region, bucket, and backup root are required")
	}
	if strings.TrimSpace(config.AccessKeyID) == "" || strings.TrimSpace(config.SecretAccessKey) == "" {
		return nil, errors.New("S3 access key and secret key are required")
	}
	local, err := NewLocalBackup(config.BackupRoot)
	if err != nil {
		return nil, err
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(config.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(config.AccessKeyID, config.SecretAccessKey, "")),
		awsconfig.WithRetryMaxAttempts(5),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		configureS3Options(options, config)
	})
	return &S3Backup{config: config, client: client, uploader: manager.NewUploader(client), local: local, retryBase: time.Second}, nil
}

func (s *S3Backup) Type() AdapterType { return S3Adapter }

func (s *S3Backup) SetProgressCallback(fn ProgressFunc) {
	s.progress = fn
	s.local.SetProgressCallback(fn)
}

func (s *S3Backup) Create(ctx context.Context, serverRoot, namespace, name string, ignored []string) (*BackupInfo, error) {
	localBackup, err := s.local.Create(ctx, serverRoot, namespace, name, ignored)
	if err != nil {
		return nil, fmt.Errorf("create S3 upload staging archive: %w", err)
	}
	backupPath, err := s.local.archivePath(namespace, name, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.local.Delete(namespace, name) }()

	key := s.getS3Key(namespace, name)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			delay := s.retryBase
			if delay <= 0 {
				delay = time.Second
			}
			timer := time.NewTimer(time.Duration(1<<(attempt-1)) * delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		lastErr = s.uploadToS3(ctx, backupPath, key, localBackup.Checksum)
		if lastErr == nil {
			localBackup.Adapter = S3Adapter
			localBackup.RemotePath = key
			return localBackup, nil
		}
	}
	return nil, fmt.Errorf("S3 upload failed after 3 attempts: %w", lastErr)
}

func (s *S3Backup) List(namespace string) ([]BackupInfo, error) {
	if !validNamespace(namespace) {
		return nil, ErrInvalidNamespace
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	prefix := s.getS3Prefix(namespace)
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.config.Bucket), Prefix: aws.String(prefix),
	})
	backups := make([]BackupInfo, 0)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list S3 backups: %w", err)
		}
		for _, object := range page.Contents {
			if object.Key == nil || !strings.HasPrefix(*object.Key, prefix) {
				continue
			}
			name := strings.TrimPrefix(*object.Key, prefix)
			if strings.Contains(name, "/") || !validBackupName(name) {
				continue
			}
			head, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.config.Bucket), Key: object.Key})
			if err != nil {
				return nil, fmt.Errorf("read S3 metadata for %s: %w", name, err)
			}
			created := time.Time{}
			if object.LastModified != nil {
				created = object.LastModified.UTC()
			}
			size := int64(0)
			if object.Size != nil {
				size = *object.Size
			}
			backups = append(backups, BackupInfo{
				UUID: strings.TrimSuffix(name, ".zip"), Name: name, Checksum: checksumFromMetadata(head.Metadata),
				Size: size, Status: "completed", Created: created, CompletedAt: created,
				Adapter: S3Adapter, RemotePath: *object.Key,
			})
		}
	}
	return backups, nil
}

func (s *S3Backup) Get(namespace, name string) (*BackupInfo, error) {
	if !validNamespace(namespace) || !validBackupName(name) {
		return nil, ErrInvalidName
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	key := s.getS3Key(namespace, name)
	result, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.config.Bucket), Key: aws.String(key)})
	if err != nil {
		return nil, fmt.Errorf("get S3 backup: %w", err)
	}
	size := int64(0)
	if result.ContentLength != nil {
		size = *result.ContentLength
	}
	created := time.Time{}
	if result.LastModified != nil {
		created = result.LastModified.UTC()
	}
	return &BackupInfo{UUID: strings.TrimSuffix(name, ".zip"), Name: name, Checksum: checksumFromMetadata(result.Metadata), Size: size,
		Status: "completed", Created: created, CompletedAt: created, Adapter: S3Adapter, RemotePath: key}, nil
}

func (s *S3Backup) Delete(namespace, name string) error {
	if !validNamespace(namespace) || !validBackupName(name) {
		return ErrInvalidName
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.config.Bucket), Key: aws.String(s.getS3Key(namespace, name))})
	if err != nil {
		return fmt.Errorf("delete S3 backup: %w", err)
	}
	return nil
}

func (s *S3Backup) Restore(ctx context.Context, namespace, name, serverRoot string, truncate bool) error {
	stagedName, cleanup, err := s.downloadToStaging(ctx, namespace, name)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := s.local.Restore(ctx, namespace, stagedName, serverRoot, truncate); err != nil {
		return fmt.Errorf("restore downloaded S3 backup: %w", err)
	}
	return nil
}

func (s *S3Backup) Download(namespace, name string) (io.ReadCloser, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stagedName, cleanup, err := s.downloadToStaging(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	file, err := s.local.Download(namespace, stagedName)
	if err != nil {
		cleanup()
		return nil, err
	}
	return &removeOnClose{ReadCloser: file, cleanup: cleanup}, nil
}

func (s *S3Backup) uploadToS3(ctx context.Context, localPath, key, checksum string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = s.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.config.Bucket), Key: aws.String(key), Body: file,
		Metadata: map[string]string{checksumMetadataKey: checksum},
	})
	if err != nil {
		return fmt.Errorf("multipart upload: %w", err)
	}
	return nil
}

func (s *S3Backup) downloadToStaging(ctx context.Context, namespace, name string) (string, func(), error) {
	if !validNamespace(namespace) || !validBackupName(name) {
		return "", func() {}, ErrInvalidName
	}
	dir, err := s.local.namespaceDir(namespace, true)
	if err != nil {
		return "", func() {}, err
	}
	temp, err := os.CreateTemp(dir, ".s3-download-*.zip")
	if err != nil {
		return "", func() {}, err
	}
	stagedName := filepath.Base(temp.Name())
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(temp.Name())
		_ = os.Remove(temp.Name() + metadataSuffix)
	}
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.config.Bucket), Key: aws.String(s.getS3Key(namespace, name)),
	})
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("download S3 backup: %w", err)
	}
	_, copyErr := copyWithContext(ctx, temp, result.Body)
	bodyCloseErr := result.Body.Close()
	if copyErr == nil {
		copyErr = bodyCloseErr
	}
	if copyErr == nil {
		copyErr = temp.Sync()
	}
	if closeErr := temp.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("stage S3 backup: %w", copyErr)
	}
	info, err := os.Stat(temp.Name())
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	actual, err := calculateChecksum(temp.Name())
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	expected := checksumFromMetadata(result.Metadata)
	if expected != "" && !strings.EqualFold(expected, actual) {
		cleanup()
		return "", func() {}, fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expected, actual)
	}
	if err := writeMetadata(temp.Name(), localMetadata{Checksum: actual, Size: info.Size(), Created: time.Now().UTC()}); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return stagedName, cleanup, nil
}

func (s *S3Backup) getS3Prefix(namespace string) string {
	prefix := strings.Trim(s.config.Prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	return prefix + namespace + "/"
}

func (s *S3Backup) getS3Key(namespace, name string) string {
	return s.getS3Prefix(namespace) + name
}

func configureS3Options(options *s3.Options, config *S3Config) {
	if config.Endpoint != "" {
		options.BaseEndpoint = aws.String(config.Endpoint)
	}
	options.UsePathStyle = config.UsePathStyle
}

func checksumFromMetadata(metadata map[string]string) string {
	for key, value := range metadata {
		if strings.EqualFold(key, checksumMetadataKey) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type removeOnClose struct {
	io.ReadCloser
	cleanup func()
}

func (r *removeOnClose) Close() error {
	err := r.ReadCloser.Close()
	r.cleanup()
	return err
}
