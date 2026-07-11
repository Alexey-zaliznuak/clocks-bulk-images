package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Storage wraps a MinIO/S3 bucket for storing generated videos.
type Storage struct {
	client         *minio.Client
	publicClient   *minio.Client
	bucket         string
}

type Options struct {
	Endpoint       string // internal endpoint used for uploads (e.g. minio:9000)
	PublicEndpoint string // endpoint reachable from the browser (e.g. localhost:9000)
	AccessKey      string
	SecretKey      string
	Bucket         string
	UseSSL         bool
}

// New creates the storage client and ensures the bucket exists.
func New(ctx context.Context, o Options) (*Storage, error) {
	client, err := minio.New(o.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(o.AccessKey, o.SecretKey, ""),
		Secure: o.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	// A second client bound to the public endpoint so presigned URLs are
	// signed for (and resolvable by) the browser.
	publicClient, err := minio.New(o.PublicEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(o.AccessKey, o.SecretKey, ""),
		Secure: o.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio public client: %w", err)
	}

	// MinIO may still be starting up in docker-compose; retry for a while.
	deadline := time.Now().Add(60 * time.Second)
	for {
		exists, err := client.BucketExists(ctx, o.Bucket)
		if err == nil {
			if !exists {
				if err = client.MakeBucket(ctx, o.Bucket, minio.MakeBucketOptions{}); err != nil {
					return nil, fmt.Errorf("make bucket: %w", err)
				}
			}
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("minio not reachable: %w", err)
		}
		time.Sleep(2 * time.Second)
	}

	return &Storage{client: client, publicClient: publicClient, bucket: o.Bucket}, nil
}

// Upload streams an object into the bucket.
func (s *Storage) Upload(ctx context.Context, objectName string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, objectName, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("upload %s: %w", objectName, err)
	}
	return nil
}

// RemovePrefix deletes every object whose key starts with the given prefix
// (e.g. "<batchId>/"). Missing objects are not treated as an error.
func (s *Storage) RemovePrefix(ctx context.Context, prefix string) error {
	objects := s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})
	for rErr := range s.client.RemoveObjects(ctx, s.bucket, objects, minio.RemoveObjectsOptions{}) {
		if rErr.Err != nil {
			return fmt.Errorf("remove %s: %w", rErr.ObjectName, rErr.Err)
		}
	}
	return nil
}

// PresignedURL returns a time-limited download URL resolvable by the browser.
func (s *Storage) PresignedURL(ctx context.Context, objectName string, ttl time.Duration) (string, error) {
	u, err := s.publicClient.PresignedGetObject(ctx, s.bucket, objectName, ttl, url.Values{})
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
