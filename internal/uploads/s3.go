package uploads

import (
	"bytes"
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
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

// Put stores body unchanged under key, replacing any object there.
func (b *S3Blobs) Put(ctx context.Context, key string, body []byte) error {
	_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(b.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("text/csv"),
	})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}
