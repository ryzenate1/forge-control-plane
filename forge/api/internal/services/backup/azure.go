package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

type AzureAdapter struct {
	containerName string
	client        *azblob.Client
}

func newAzureClient(connString string) (*azblob.Client, error) {
	connURL, err := url.Parse(connString)
	if err == nil && connURL.Scheme == "https" {
		return azblob.NewClientWithNoCredential(connString, nil)
	}
	return azblob.NewClientFromConnectionString(connString, nil)
}

func NewAzureAdapter(containerName, connString string) (*AzureAdapter, error) {
	client, err := newAzureClient(connString)
	if err != nil {
		return nil, fmt.Errorf("create azure client: %w", err)
	}
	return &AzureAdapter{containerName: containerName, client: client}, nil
}

func (a *AzureAdapter) Name() string { return "azure" }

func (a *AzureAdapter) Upload(ctx context.Context, path string, data []byte) error {
	_, err := a.client.UploadStream(ctx, a.containerName, path, bytes.NewReader(data), nil)
	if err != nil {
		return fmt.Errorf("azure upload: %w", err)
	}
	return nil
}

func (a *AzureAdapter) Download(ctx context.Context, path string) ([]byte, error) {
	resp, err := a.client.DownloadStream(ctx, a.containerName, path, nil)
	if err != nil {
		return nil, fmt.Errorf("azure download: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("azure download read: %w", err)
	}
	return data, nil
}

func (a *AzureAdapter) Delete(ctx context.Context, path string) error {
	_, err := a.client.DeleteBlob(ctx, a.containerName, path, nil)
	if err != nil {
		return fmt.Errorf("azure delete: %w", err)
	}
	return nil
}

func (a *AzureAdapter) List(ctx context.Context, prefix string) ([]string, error) {
	var names []string
	pager := a.client.NewListBlobsFlatPager(a.containerName, &azblob.ListBlobsFlatOptions{
		Prefix: &prefix,
	})
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("azure list: %w", err)
		}
		for _, item := range resp.Segment.BlobItems {
			if item.Name != nil {
				names = append(names, *item.Name)
			}
		}
	}
	return names, nil
}

func (a *AzureAdapter) Exists(ctx context.Context, path string) (bool, error) {
	_, err := a.client.DownloadStream(ctx, a.containerName, path, nil)
	if err != nil {
		if isBlobNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("azure exists: %w", err)
	}
	return true, nil
}

func isBlobNotFound(err error) bool {
	var storageErr *azcore.ResponseError
	if errors.As(err, &storageErr) {
		code := string(storageErr.ErrorCode)
		return strings.EqualFold(code, "BlobNotFound") || strings.EqualFold(code, "ContainerNotFound")
	}
	return false
}
