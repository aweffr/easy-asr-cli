package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3API interface {
	PutObject(ctx context.Context, input *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(ctx context.Context, input *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

type S3Presigner interface {
	PresignGetObject(ctx context.Context, input *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

type S3Options struct {
	Bucket     string
	KeyPrefix  string
	PresignTTL time.Duration
	API        S3API
	Presigner  S3Presigner
}

type S3Client struct {
	bucket     string
	keyPrefix  string
	presignTTL time.Duration
	api        S3API
	presigner  S3Presigner
}

func NewS3Client(options S3Options) *S3Client {
	if options.PresignTTL <= 0 {
		options.PresignTTL = 24 * time.Hour
	}
	return &S3Client{
		bucket:     options.Bucket,
		keyPrefix:  strings.Trim(options.KeyPrefix, "/"),
		presignTTL: options.PresignTTL,
		api:        options.API,
		presigner:  options.Presigner,
	}
}

func NewS3ClientFromConfig(cfg S3Config) (*S3Client, error) {
	awsConfig, err := awscfg.LoadDefaultConfig(
		context.Background(),
		awscfg.WithRegion(cfg.Region),
		awscfg.WithCredentialsProvider(awscreds.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.AccessKeySecret, "")),
		awscfg.WithBaseEndpoint(cfg.Endpoint),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.UsePathStyle = cfg.UsePathStyle
	})
	return NewS3Client(S3Options{
		Bucket:     cfg.Bucket,
		KeyPrefix:  cfg.KeyPrefix,
		PresignTTL: cfg.PresignTTL,
		API:        client,
		Presigner:  s3.NewPresignClient(client),
	}), nil
}

type S3Config struct {
	Region          string
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	AccessKeySecret string
	KeyPrefix       string
	PresignTTL      time.Duration
	UsePathStyle    bool
}

func (c *S3Client) Upload(ctx context.Context, path string) (StoredObject, error) {
	if c.api == nil {
		return StoredObject{}, fmt.Errorf("S3 API is required")
	}
	if strings.TrimSpace(c.bucket) == "" {
		return StoredObject{}, fmt.Errorf("S3 bucket is required")
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
	_, err = c.api.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          file,
		ContentLength: aws.Int64(stat.Size()),
		ContentType:   aws.String(contentTypeForName(stat.Name())),
	})
	if err != nil {
		return StoredObject{}, fmt.Errorf("put S3 object %s: %w", key, err)
	}
	return StoredObject{Bucket: c.bucket, Key: key}, nil
}

func (c *S3Client) PresignGet(ctx context.Context, key string) (string, error) {
	if c.presigner == nil {
		return "", fmt.Errorf("S3 presigner is required")
	}
	result, err := c.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}, func(options *s3.PresignOptions) {
		options.Expires = c.presignTTL
	})
	if err != nil {
		return "", fmt.Errorf("presign S3 object %s: %w", key, err)
	}
	if strings.TrimSpace(result.URL) == "" {
		return "", fmt.Errorf("presign S3 object %s returned empty URL", key)
	}
	return result.URL, nil
}

func (c *S3Client) Delete(ctx context.Context, key string) error {
	if _, err := c.api.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("delete S3 object %s: %w", key, err)
	}
	return nil
}
