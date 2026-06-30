package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
	"github.com/aweffr/easy-asr-cli/internal/config"
)

type OSSAPI interface {
	PutObject(ctx context.Context, request *oss.PutObjectRequest, optFns ...func(*oss.Options)) (*oss.PutObjectResult, error)
	DeleteObject(ctx context.Context, request *oss.DeleteObjectRequest, optFns ...func(*oss.Options)) (*oss.DeleteObjectResult, error)
	Presign(ctx context.Context, request any, optFns ...func(*oss.PresignOptions)) (*oss.PresignResult, error)
}

type OSSOptions struct {
	Bucket     string
	KeyPrefix  string
	PresignTTL time.Duration
	API        OSSAPI
}

type StoredObject struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

type OSSClient struct {
	bucket     string
	keyPrefix  string
	presignTTL time.Duration
	api        OSSAPI
}

func NewOSSClient(options OSSOptions) *OSSClient {
	if options.PresignTTL <= 0 {
		options.PresignTTL = 24 * time.Hour
	}
	return &OSSClient{
		bucket:     options.Bucket,
		keyPrefix:  strings.Trim(options.KeyPrefix, "/"),
		presignTTL: options.PresignTTL,
		api:        options.API,
	}
}

func NewOSSClientFromConfig(cfg config.OSSConfig) *OSSClient {
	ossCfg := oss.NewConfig().
		WithRegion(cfg.Region).
		WithEndpoint(cfg.Endpoint).
		WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.AccessKeySecret))
	api := oss.NewClient(ossCfg)
	return NewOSSClient(OSSOptions{
		Bucket:     cfg.Bucket,
		KeyPrefix:  cfg.KeyPrefix,
		PresignTTL: cfg.PresignTTL,
		API:        api,
	})
}

func NewClientFromConfig(cfg config.OSSConfig) (interface {
	Upload(ctx context.Context, path string) (StoredObject, error)
	PresignGet(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
}, error) {
	if strings.Contains(cfg.Endpoint, "myqcloud.com") {
		return NewS3ClientFromConfig(S3Config{
			Region:          cfg.Region,
			Endpoint:        cfg.Endpoint,
			Bucket:          cfg.Bucket,
			AccessKeyID:     cfg.AccessKeyID,
			AccessKeySecret: cfg.AccessKeySecret,
			KeyPrefix:       cfg.KeyPrefix,
			PresignTTL:      cfg.PresignTTL,
		})
	}
	return NewOSSClientFromConfig(cfg), nil
}

func (c *OSSClient) Upload(ctx context.Context, path string) (StoredObject, error) {
	if c.api == nil {
		return StoredObject{}, fmt.Errorf("OSS API is required")
	}
	if strings.TrimSpace(c.bucket) == "" {
		return StoredObject{}, fmt.Errorf("OSS bucket is required")
	}
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return StoredObject{}, fmt.Errorf("open upload file: %w", err)
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return StoredObject{}, fmt.Errorf("stat upload file: %w", err)
	}
	if stat.IsDir() {
		return StoredObject{}, fmt.Errorf("upload path is a directory: %s", path)
	}
	key, err := makeObjectKey(c.keyPrefix, stat.Name())
	if err != nil {
		return StoredObject{}, err
	}
	contentType := contentTypeForName(stat.Name())
	_, err = c.api.PutObject(ctx, &oss.PutObjectRequest{
		Bucket:        oss.Ptr(c.bucket),
		Key:           oss.Ptr(key),
		ContentLength: oss.Ptr(stat.Size()),
		ContentType:   oss.Ptr(contentType),
		Body:          file,
	})
	if err != nil {
		return StoredObject{}, fmt.Errorf("put OSS object %s: %w", key, err)
	}
	return StoredObject{Bucket: c.bucket, Key: key}, nil
}

func (c *OSSClient) PresignGet(ctx context.Context, key string) (string, error) {
	result, err := c.api.Presign(ctx, &oss.GetObjectRequest{
		Bucket: oss.Ptr(c.bucket),
		Key:    oss.Ptr(key),
	}, oss.PresignExpires(c.presignTTL))
	if err != nil {
		return "", fmt.Errorf("presign OSS object %s: %w", key, err)
	}
	if strings.TrimSpace(result.URL) == "" {
		return "", fmt.Errorf("presign OSS object %s returned empty URL", key)
	}
	return result.URL, nil
}

func (c *OSSClient) Delete(ctx context.Context, key string) error {
	_, err := c.api.DeleteObject(ctx, &oss.DeleteObjectRequest{
		Bucket: oss.Ptr(c.bucket),
		Key:    oss.Ptr(key),
	})
	if err != nil {
		return fmt.Errorf("delete OSS object %s: %w", key, err)
	}
	return nil
}

func makeObjectKey(keyPrefix string, filename string) (string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate object key: %w", err)
	}
	parts := []string{}
	if keyPrefix != "" {
		parts = append(parts, strings.Trim(keyPrefix, "/"))
	}
	parts = append(parts, time.Now().Format("20060102"), hex.EncodeToString(random), filepath.Base(filename))
	return strings.Join(parts, "/"), nil
}

func contentTypeForName(name string) string {
	if value := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); value != "" {
		return value
	}
	return "application/octet-stream"
}
