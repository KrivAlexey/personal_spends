package uploads

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var _ blobStore = (*S3Blobs)(nil)

type fakeS3 struct {
	err error

	gotBucket, gotKey, gotType string
	gotBody                    []byte
}

func (f *fakeS3) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.gotBucket, f.gotKey, f.gotType = aws.ToString(in.Bucket), aws.ToString(in.Key), aws.ToString(in.ContentType)
	b, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	f.gotBody = b
	return &s3.PutObjectOutput{}, nil
}

func TestS3Blobs_Put(t *testing.T) {
	errS3 := errors.New("s3: access denied")
	tests := []struct {
		name    string
		client  *fakeS3
		wantErr error
	}{
		{name: "puts the exact bytes", client: &fakeS3{}},
		{name: "client error is wrapped", client: &fakeS3{err: errS3}, wantErr: errS3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewS3Blobs(tt.client, "uploads-bucket").Put(context.Background(), bodyKey, body)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Put() error = %v, want errors.Is %v", err, tt.wantErr)
				}
				if strings.Contains(err.Error(), bodySum) {
					t.Errorf("Put() error %q carries the key's hash", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Put() error = %v", err)
			}
			if tt.client.gotBucket != "uploads-bucket" || tt.client.gotKey != bodyKey || tt.client.gotType != "text/csv" {
				t.Errorf("put to %s/%s as %q, want uploads-bucket/%s as text/csv",
					tt.client.gotBucket, tt.client.gotKey, tt.client.gotType, bodyKey)
			}
			if !bytes.Equal(tt.client.gotBody, body) {
				t.Errorf("put body = %q, want %q", tt.client.gotBody, body)
			}
		})
	}
}

// TestS3Blobs_PutIntegration puts to TEST_S3_ENDPOINT (e.g. the compose
// S3Mock: http://localhost:9090) and reads the object back. The bucket is
// TEST_S3_BUCKET, or personal-spends-uploads.
func TestS3Blobs_PutIntegration(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_S3_ENDPOINT not set")
	}
	bucket := os.Getenv("TEST_S3_BUCKET")
	if bucket == "" {
		bucket = "personal-spends-uploads"
	}
	client := s3.New(s3.Options{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(endpoint),
		UsePathStyle: true,
		Credentials:  credentials.NewStaticCredentialsProvider("local", "local", ""),
	})
	ctx := context.Background()
	// ISO-8859-1 bytes must survive unchanged.
	latin1 := append(bytes.Clone(body), []byte("Grundpreis f\xfcr Kontof\xfchrung\n")...)

	if err := NewS3Blobs(client, bucket).Put(ctx, bodyKey, latin1); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	out, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(bodyKey)})
	if err != nil {
		t.Fatalf("GetObject() error = %v", err)
	}
	defer out.Body.Close()
	got, err := io.ReadAll(out.Body)
	if err != nil {
		t.Fatalf("read object: %v", err)
	}
	if !bytes.Equal(got, latin1) {
		t.Errorf("stored object = %q, want %q", got, latin1)
	}
}
