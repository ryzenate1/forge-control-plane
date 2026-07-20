package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Adapter struct {
	bucket string
	prefix string
	client *s3.Client
}

func NewS3Adapter(ctx context.Context, region, endpoint, bucket, prefix, accessKeyID, secretAccessKey string, usePathStyle bool) (*S3Adapter, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		region = "us-east-1"
	}
	endpoint = strings.TrimSpace(endpoint)
	bucket = strings.TrimSpace(bucket)

	var opts []func(*config.LoadOptions) error
	opts = append(opts, config.WithRegion(region))
	opts = append(opts, config.WithCredentialsProvider(
		credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
	))

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	s3Opts := []func(*s3.Options){
		func(o *s3.Options) {
			o.UsePathStyle = usePathStyle
		},
	}
	if endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}

	client := s3.NewFromConfig(cfg, s3Opts...)

	return &S3Adapter{
		bucket: bucket,
		prefix: strings.TrimRight(prefix, "/"),
		client: client,
	}, nil
}

func (a *S3Adapter) Name() string { return "s3" }

func (a *S3Adapter) key(path string) string {
	if a.prefix != "" {
		return a.prefix + "/" + strings.TrimLeft(path, "/")
	}
	return path
}

func (a *S3Adapter) Upload(ctx context.Context, path string, data []byte) error {
	input := &s3.PutObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(a.key(path)),
		Body:   bytes.NewReader(data),
	}
	_, err := a.client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("s3 upload: %w", err)
	}
	return nil
}

func (a *S3Adapter) Download(ctx context.Context, path string) ([]byte, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(a.key(path)),
	}
	output, err := a.client.GetObject(ctx, input)
	if err != nil {
		if isS3NotFound(err) {
			return nil, fmt.Errorf("s3 download: object not found: %s", path)
		}
		return nil, fmt.Errorf("s3 download: %w", err)
	}
	defer output.Body.Close()
	data, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, fmt.Errorf("s3 download read: %w", err)
	}
	return data, nil
}

func (a *S3Adapter) Delete(ctx context.Context, path string) error {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(a.key(path)),
	}
	_, err := a.client.DeleteObject(ctx, input)
	if err != nil {
		return fmt.Errorf("s3 delete: %w", err)
	}
	return nil
}

func (a *S3Adapter) List(ctx context.Context, prefix string) ([]string, error) {
	fullPrefix := a.key(prefix)
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(a.bucket),
		Prefix: aws.String(fullPrefix),
	}
	paginator := s3.NewListObjectsV2Paginator(a.client, input)
	var names []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3 list: %w", err)
		}
		for _, obj := range page.Contents {
			if obj.Key != nil {
				names = append(names, *obj.Key)
			}
		}
	}
	return names, nil
}

func (a *S3Adapter) Exists(ctx context.Context, path string) (bool, error) {
	input := &s3.HeadObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(a.key(path)),
	}
	_, err := a.client.HeadObject(ctx, input)
	if err != nil {
		if isS3NotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("s3 exists: %w", err)
	}
	return true, nil
}

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
