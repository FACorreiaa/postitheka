// Package media keeps uploaded files in S3-compatible object storage.
//
// Clients upload straight to storage with a presigned URL, so a 200 MB video
// never passes through the API process. The API only ever handles metadata,
// and reads the bytes back once, when a publisher streams them to a platform.
package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/cors"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var ErrNotFound = errors.New("media: object not found")

type Options struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

type Store struct {
	client *minio.Client
	bucket string
}

func New(o Options) (*Store, error) {
	client, err := minio.New(o.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(o.AccessKey, o.SecretKey, ""),
		Secure: o.UseSSL,
		Region: o.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("media: %w", err)
	}
	return &Store{client: client, bucket: o.Bucket}, nil
}

// EnsureBucket creates the bucket if it is missing and lets browsers on
// origins PUT to it. For local development only: in production the bucket and
// its CORS rules are infrastructure, and an app that creates its own would
// hide a misconfigured name.
//
// The CORS rule is what lets the web app upload straight to storage. Without
// it the browser's preflight fails and the upload never starts, with nothing
// in the server's logs to say why.
func (s *Store) EnsureBucket(ctx context.Context, origins []string) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("media: bucket exists: %w", err)
	}
	if !exists {
		if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("media: make bucket: %w", err)
		}
	}
	if err := s.client.SetBucketCors(ctx, s.bucket, &cors.Config{CORSRules: []cors.Rule{{
		AllowedOrigin: origins,
		AllowedMethod: []string{http.MethodPut},
		AllowedHeader: []string{"Content-Type"},
		MaxAgeSeconds: 3600,
	}}}); err != nil {
		return fmt.Errorf("media: set bucket cors: %w", err)
	}
	return nil
}

// PresignUpload returns a URL the client can PUT the file to.
//
// A presigned PUT cannot pin the size or type, so neither is trusted from it:
// Stat reads both back from storage before the file can be attached to a post.
func (s *Store) PresignUpload(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedPutObject(ctx, s.bucket, key, ttl)
	if err != nil {
		return "", fmt.Errorf("media: presign put: %w", err)
	}
	return u.String(), nil
}

type Object struct {
	Size        int64
	ContentType string
}

func (s *Store) Stat(ctx context.Context, key string) (Object, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return Object{}, ErrNotFound
		}
		return Object{}, fmt.Errorf("media: stat: %w", err)
	}
	return Object{Size: info.Size, ContentType: info.ContentType}, nil
}

// Delete removes an object. Deleting one that is already gone is not an error,
// so a sweep interrupted halfway can simply run again.
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("media: delete: %w", err)
	}
	return nil
}

// Open implements publisher.MediaSource.
func (s *Store) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("media: get: %w", err)
	}
	return obj, nil
}

// PublicURL implements publisher.MediaSource, for platforms that download the
// file themselves. A presigned GET rather than a public bucket, so uploads
// that were never posted are never reachable.
func (s *Store) PublicURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, nil)
	if err != nil {
		return "", fmt.Errorf("media: presign get: %w", err)
	}
	return u.String(), nil
}
