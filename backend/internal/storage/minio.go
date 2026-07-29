package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
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

	// The public endpoint is a browser-facing address (localhost, a domain behind
	// a proxy) that the server itself often cannot reach, so pin the region here
	// instead of letting the signer look it up over the network.
	region, err := client.GetBucketLocation(ctx, o.Bucket)
	if err != nil || region == "" {
		region = "us-east-1"
	}

	// A second client bound to the public endpoint so presigned URLs are
	// signed for (and resolvable by) the browser.
	publicClient, err := minio.New(o.PublicEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(o.AccessKey, o.SecretKey, ""),
		Secure: o.UseSSL,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("minio public client: %w", err)
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

// Download streams an object out of the bucket. The caller must close the reader.
func (s *Storage) Download(ctx context.Context, objectName string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", objectName, err)
	}
	// GetObject is lazy: the request only happens on first read, so probe the
	// object now to surface "missing key" and connection errors here.
	if _, err := obj.Stat(); err != nil {
		obj.Close()
		return nil, fmt.Errorf("download %s: %w", objectName, err)
	}
	return obj, nil
}

// Remove deletes a single object. A missing object is not an error.
func (s *Storage) Remove(ctx context.Context, objectName string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, objectName, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("remove %s: %w", objectName, err)
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

// IsNotFound reports whether err means the object simply does not exist, which
// is permanent: retrying will not bring it back.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var resp minio.ErrorResponse
	if errors.As(err, &resp) {
		return resp.Code == "NoSuchKey" || resp.StatusCode == http.StatusNotFound
	}
	return false
}

// PresignedURL returns a time-limited download URL resolvable by the browser.
func (s *Storage) PresignedURL(ctx context.Context, objectName string, ttl time.Duration) (string, error) {
	u, err := s.publicClient.PresignedGetObject(ctx, s.bucket, objectName, ttl, url.Values{})
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
