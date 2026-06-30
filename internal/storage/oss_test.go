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

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aweffr/easy-asr-cli/internal/storage"
)

func TestUploadPresignAndDeleteUseOSSRequests(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "voice.mp3")
	if err := os.WriteFile(audio, []byte("audio bytes"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	api := &fakeOSSAPI{presignURL: "https://signed.example/voice.mp3"}
	client := storage.NewOSSClient(storage.OSSOptions{
		Bucket:     "bucket",
		KeyPrefix:  "easy_asr/tmp",
		PresignTTL: 30 * time.Minute,
		API:        api,
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
	if api.putRequest == nil {
		t.Fatal("PutObject was not called")
	}
	if oss.ToString(api.putRequest.Bucket) != "bucket" || oss.ToString(api.putRequest.Key) != stored.Key {
		t.Fatalf("put request bucket/key = %q/%q", oss.ToString(api.putRequest.Bucket), oss.ToString(api.putRequest.Key))
	}
	if !bytes.Equal(api.putBody, []byte("audio bytes")) {
		t.Fatalf("body = %q", api.putBody)
	}
	if api.putRequest.ContentType == nil || *api.putRequest.ContentType != "audio/mpeg" {
		t.Fatalf("content type = %#v", api.putRequest.ContentType)
	}

	url, err := client.PresignGet(context.Background(), stored.Key)
	if err != nil {
		t.Fatalf("PresignGet returned error: %v", err)
	}
	if url != "https://signed.example/voice.mp3" {
		t.Fatalf("signed URL = %q", url)
	}
	if api.presignRequest == nil {
		t.Fatal("Presign was not called")
	}
	getReq := api.presignRequest.(*oss.GetObjectRequest)
	if oss.ToString(getReq.Key) != stored.Key {
		t.Fatalf("presign key = %q", oss.ToString(getReq.Key))
	}
	if api.presignTTL != 30*time.Minute {
		t.Fatalf("presign ttl = %s", api.presignTTL)
	}

	if err := client.Delete(context.Background(), stored.Key); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if api.deleteRequest == nil || oss.ToString(api.deleteRequest.Key) != stored.Key {
		t.Fatalf("delete request = %#v", api.deleteRequest)
	}
}

func TestUploadRequiresExistingFile(t *testing.T) {
	client := storage.NewOSSClient(storage.OSSOptions{
		Bucket: "bucket",
		API:    &fakeOSSAPI{},
	})
	_, err := client.Upload(context.Background(), filepath.Join(t.TempDir(), "missing.mp3"))
	if err == nil {
		t.Fatal("Upload returned nil error")
	}
}

type fakeOSSAPI struct {
	putRequest     *oss.PutObjectRequest
	putBody        []byte
	deleteRequest  *oss.DeleteObjectRequest
	presignRequest any
	presignTTL     time.Duration
	presignURL     string
}

func (f *fakeOSSAPI) PutObject(ctx context.Context, request *oss.PutObjectRequest, _ ...func(*oss.Options)) (*oss.PutObjectResult, error) {
	f.putRequest = request
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	f.putBody = body
	return &oss.PutObjectResult{}, nil
}

func (f *fakeOSSAPI) DeleteObject(ctx context.Context, request *oss.DeleteObjectRequest, _ ...func(*oss.Options)) (*oss.DeleteObjectResult, error) {
	f.deleteRequest = request
	return &oss.DeleteObjectResult{}, nil
}

func (f *fakeOSSAPI) Presign(ctx context.Context, request any, optFns ...func(*oss.PresignOptions)) (*oss.PresignResult, error) {
	f.presignRequest = request
	options := &oss.PresignOptions{}
	for _, fn := range optFns {
		fn(options)
	}
	f.presignTTL = options.Expires
	return &oss.PresignResult{URL: f.presignURL}, nil
}
