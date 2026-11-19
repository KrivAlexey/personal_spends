package uploads

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// s3API is the part of *s3.Client S3Blobs uses.
type s3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// S3Blobs keeps uploaded files in one S3 bucket.
type S3Blobs struct {
	client s3API
	bucket string
}

// NewS3Blobs returns an S3Blobs writing to bucket through client.
func NewS3Blobs(client s3API, bucket string) *S3Blobs {
	return &S3Blobs{client: client, bucket: bucket}
}

func (b *S3Blobs) Put(ctx context.Context, key string, body []byte) error {
	return errors.New("uploads: not implemented")
}
