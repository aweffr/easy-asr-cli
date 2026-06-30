package storage_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/aweffr/easy-asr-cli/internal/storage"
)

func TestS3UploadPresignAndDelete(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "voice.mp3")
	if err := os.WriteFile(audio, []byte("audio bytes"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	api := &fakeS3API{url: "https://signed.example/voice.mp3"}
	client := storage.NewS3Client(storage.S3Options{
		Bucket:     "bucket",
		KeyPrefix:  "easy_asr/tmp",
		PresignTTL: 15 * time.Minute,
		API:        api,
		Presigner:  api,
	})

	stored, err := client.Upload(context.Background(), audio)
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if stored.Bucket != "bucket" {
		t.Fatalf("Bucket = %q", stored.Bucket)
	}
	if !strings.HasPrefix(stored.Key, "easy_asr/tmp/") || !strings.HasSuffix(stored.Key, "/voice.mp3") {
		t.Fatalf("unexpected key: %q", stored.Key)
	}
	if api.put == nil || *api.put.Bucket != "bucket" || *api.put.Key != stored.Key {
		t.Fatalf("put = %#v", api.put)
	}
	if !bytes.Equal(api.putBody, []byte("audio bytes")) {
		t.Fatalf("put body = %q", api.putBody)
	}
	if api.put.ContentType == nil || *api.put.ContentType != "audio/mpeg" {
		t.Fatalf("content type = %#v", api.put.ContentType)
	}

	url, err := client.PresignGet(context.Background(), stored.Key)
	if err != nil {
		t.Fatalf("PresignGet returned error: %v", err)
	}
	if url != "https://signed.example/voice.mp3" {
		t.Fatalf("url = %q", url)
	}
	if api.presign == nil || *api.presign.Key != stored.Key {
		t.Fatalf("presign = %#v", api.presign)
	}
	if api.presignTTL != 15*time.Minute {
		t.Fatalf("presign ttl = %s", api.presignTTL)
	}

	if err := client.Delete(context.Background(), stored.Key); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if api.delete == nil || *api.delete.Key != stored.Key {
		t.Fatalf("delete = %#v", api.delete)
	}
}

type fakeS3API struct {
	put        *s3.PutObjectInput
	putBody    []byte
	delete     *s3.DeleteObjectInput
	presign    *s3.GetObjectInput
	presignTTL time.Duration
	url        string
}

func (f *fakeS3API) PutObject(ctx context.Context, input *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.put = input
	body, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	f.putBody = body
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3API) DeleteObject(ctx context.Context, input *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	f.delete = input
	return &s3.DeleteObjectOutput{}, nil
}

func (f *fakeS3API) PresignGetObject(ctx context.Context, input *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error) {
	f.presign = input
	options := &s3.PresignOptions{}
	for _, fn := range optFns {
		fn(options)
	}
	f.presignTTL = options.Expires
	return &v4.PresignedHTTPRequest{URL: f.url}, nil
}
