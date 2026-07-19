package backup

// Requires: go get cloud.google.com/go/storage@latest
import (
	"bytes"
	"context"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type GCSAdapter struct {
	bucketName string
	client     *storage.Client
}

func NewGCSAdapter(bucketName, keyFile string) (*GCSAdapter, error) {
	var opts []option.ClientOption
	if keyFile != "" {
		opts = append(opts, option.WithCredentialsFile(keyFile))
	}
	client, err := storage.NewClient(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("create gcs client: %w", err)
	}
	return &GCSAdapter{
		bucketName: bucketName,
		client:     client,
	}, nil
}

func (a *GCSAdapter) Name() string { return "gcs" }

func (a *GCSAdapter) Upload(ctx context.Context, path string, data []byte) error {
	w := a.client.Bucket(a.bucketName).Object(path).NewWriter(ctx)
	if _, err := io.Copy(w, bytes.NewReader(data)); err != nil {
		w.Close()
		return fmt.Errorf("gcs upload: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("gcs upload close: %w", err)
	}
	return nil
}

func (a *GCSAdapter) Download(ctx context.Context, path string) ([]byte, error) {
	r, err := a.client.Bucket(a.bucketName).Object(path).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("gcs download: %w", err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("gcs download read: %w", err)
	}
	return data, nil
}

func (a *GCSAdapter) Delete(ctx context.Context, path string) error {
	if err := a.client.Bucket(a.bucketName).Object(path).Delete(ctx); err != nil {
		return fmt.Errorf("gcs delete: %w", err)
	}
	return nil
}

func (a *GCSAdapter) List(ctx context.Context, prefix string) ([]string, error) {
	var names []string
	it := a.client.Bucket(a.bucketName).Objects(ctx, &storage.Query{Prefix: prefix})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("gcs list: %w", err)
		}
		names = append(names, attrs.Name)
	}
	return names, nil
}

func (a *GCSAdapter) Exists(ctx context.Context, path string) (bool, error) {
	_, err := a.client.Bucket(a.bucketName).Object(path).Attrs(ctx)
	if err == storage.ErrObjectNotExist {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("gcs exists: %w", err)
	}
	return true, nil
}
