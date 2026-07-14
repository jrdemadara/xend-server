package objectstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type S3Store struct {
	client *s3.Client
	bucket string
	prefix string
}

func NewS3Store(ctx context.Context, bucket, region, prefix string) (*S3Store, error) {
	options := []func(*awsconfig.LoadOptions) error{}
	if strings.TrimSpace(region) != "" {
		options = append(options, awsconfig.WithRegion(strings.TrimSpace(region)))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, err
	}

	return &S3Store{
		client: s3.NewFromConfig(cfg),
		bucket: strings.TrimSpace(bucket),
		prefix: strings.Trim(strings.TrimSpace(prefix), "/"),
	}, nil
}

func (s *S3Store) Put(ctx context.Context, key string, data []byte, contentType string) error {
	objectKey, err := s.objectKey(key)
	if err != nil {
		return err
	}

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(objectKey),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	return err
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	objectKey, err := s.objectKey(key)
	if err != nil {
		return nil
	}

	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
	})
	return err
}

func (s *S3Store) Get(ctx context.Context, key string) ([]byte, string, error) {
	objectKey, err := s.objectKey(key)
	if err != nil {
		return nil, "", err
	}

	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, "", os.ErrNotExist
		}
		return nil, "", err
	}
	defer output.Body.Close()

	data, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, "", err
	}

	contentType := ""
	if output.ContentType != nil {
		contentType = *output.ContentType
	}
	return data, contentType, nil
}

func (s *S3Store) objectKey(key string) (string, error) {
	if s.bucket == "" {
		return "", errors.New("s3 bucket is required")
	}

	cleanedKey, err := cleanKey(key)
	if err != nil {
		return "", err
	}
	if s.prefix == "" {
		return cleanedKey, nil
	}
	return path.Join(s.prefix, cleanedKey), nil
}

func isNotFound(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "NoSuchKey", "NotFound":
		return true
	default:
		return false
	}
}
