package blob

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3 struct {
	client *minio.Client
	bucket string
}

func NewS3(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*S3, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create S3 client: %w", err)
	}
	return &S3{client: client, bucket: bucket}, nil
}

func (s *S3) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check bucket: %w", err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("create bucket: %w", err)
	}
	return nil
}

func (s *S3) PutChunk(ctx context.Context, key string, data []byte) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		return fmt.Errorf("put chunk: %w", err)
	}
	return nil
}

func (s *S3) DeleteChunk(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete chunk: %w", err)
	}
	return nil
}

func (s *S3) GetChunk(ctx context.Context, key string, start, end int64) (io.ReadCloser, error) {
	options := minio.GetObjectOptions{}
	if start != 0 || end != 0 {
		if err := options.SetRange(start, end); err != nil {
			return nil, fmt.Errorf("set chunk range: %w", err)
		}
	}
	object, err := s.client.GetObject(ctx, s.bucket, key, options)
	if err != nil {
		return nil, fmt.Errorf("get chunk: %w", err)
	}
	return object, nil
}
