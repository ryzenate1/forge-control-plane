package backup

import (
	"fmt"
)

func NewS3Factory(config map[string]string) (StorageAdapter, error) {
	region := config["region"]
	endpoint := config["endpoint"]
	bucket := config["bucket"]
	prefix := config["prefix"]
	accessKeyID := config["accessKeyId"]
	secretAccessKey := config["secretAccessKey"]
	usePathStyle := config["usePathStyle"] == "true"
	if region == "" {
		region = "us-east-1"
	}
	if bucket == "" {
		return nil, fmt.Errorf("s3 adapter: bucket is required")
	}
	s3Config := &S3StorageConfig{
		Region:          region,
		Endpoint:        endpoint,
		Bucket:          bucket,
		Prefix:          prefix,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		UsePathStyle:    usePathStyle,
	}
	return NewS3StorageAdapter(s3Config)
}

func NewGCSFactory(config map[string]string) (StorageAdapter, error) {
	bucketName := config["bucket"]
	projectID := config["projectId"]
	serviceAccount := config["serviceAccount"]
	prefix := config["prefix"]
	if bucketName == "" {
		return nil, fmt.Errorf("gcs adapter: bucket is required")
	}
	gcsConfig := &GCSStorageConfig{
		ProjectID:      projectID,
		Bucket:         bucketName,
		Prefix:         prefix,
		ServiceAccount: serviceAccount,
	}
	return NewGCSStorageAdapter(gcsConfig)
}

func NewAzureFactory(config map[string]string) (StorageAdapter, error) {
	containerName := config["container"]
	connString := config["connectionString"]
	prefix := config["prefix"]
	if containerName == "" {
		return nil, fmt.Errorf("azure adapter: container is required")
	}
	if connString == "" {
		return nil, fmt.Errorf("azure adapter: connectionString is required")
	}
	azureConfig := &AzureStorageConfig{
		ConnectionString: connString,
		Container:        containerName,
		Prefix:           prefix,
	}
	return NewAzureStorageAdapter(azureConfig)
}

func NewLocalFactory(config map[string]string) (StorageAdapter, error) {
	basePath := config["basePath"]
	if basePath == "" {
		basePath = "/tmp/backups"
	}
	return NewLocalStorageAdapter(basePath)
}
