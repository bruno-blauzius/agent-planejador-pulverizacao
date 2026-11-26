// Package objectstorage keeps the files of owner documents in Amazon S3 (ADR
// 0020). Credentials come from the default AWS chain (environment, shared
// profile, or the IAM role of the workload); nothing secret is configured here.
package objectstorage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// maxObjectBytes bounds a downloaded object (owner documents are a few MB).
const maxObjectBytes = 64 << 20

// Config locates the bucket of owner documents.
type Config struct {
	Bucket string
	Region string
	// Endpoint replaces the AWS endpoint (an S3-compatible emulator in tests);
	// empty uses Amazon S3.
	Endpoint     string
	UsePathStyle bool
}

// LookupFunc matches os.LookupEnv.
type LookupFunc func(key string) (string, bool)

// ConfigFromEnv reads API_DOCUMENTS_BUCKET, API_DOCUMENTS_REGION,
// API_DOCUMENTS_S3_ENDPOINT and API_DOCUMENTS_S3_PATH_STYLE. enabled is false
// when no bucket is configured: the API then answers 503 on document routes.
func ConfigFromEnv(lookup LookupFunc) (cfg Config, enabled bool, err error) {
	value := func(key string) string {
		v, _ := lookup(key)
		return strings.TrimSpace(v)
	}
	cfg = Config{Bucket: value("API_DOCUMENTS_BUCKET"), Region: value("API_DOCUMENTS_REGION"), Endpoint: value("API_DOCUMENTS_S3_ENDPOINT")}
	if cfg.Bucket == "" {
		return Config{}, false, nil
	}
	if cfg.Region == "" {
		return Config{}, false, errors.New("API_DOCUMENTS_REGION is required when API_DOCUMENTS_BUCKET is set")
	}
	if raw := value("API_DOCUMENTS_S3_PATH_STYLE"); raw != "" {
		if cfg.UsePathStyle, err = strconv.ParseBool(raw); err != nil {
			return Config{}, false, fmt.Errorf("invalid API_DOCUMENTS_S3_PATH_STYLE %q: must be true or false", raw)
		}
	}
	return cfg, true, nil
}

// S3Store implements usecase.DocumentStorage on a bucket.
type S3Store struct {
	client *s3.Client
	bucket string
}

var _ usecase.DocumentStorage = (*S3Store)(nil)

// NewS3Store builds the client from the default AWS configuration chain.
func NewS3Store(ctx context.Context, cfg Config) (*S3Store, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.UsePathStyle
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			// Emulators do not all speak the flexible checksums of Amazon S3.
			o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
			o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
		}
	})
	return &S3Store{client: client, bucket: cfg.Bucket}, nil
}

// unavailable wraps a storage failure so the API answers 503 and the cause is logged.
func unavailable(operation string, err error) error {
	return fmt.Errorf("%w: %s: %w", usecase.ErrDocumentStorageUnavailable, operation, err)
}

// PutDocument implements usecase.DocumentStorage. Encryption at rest is the
// bucket's default (SSE-KMS, see infra/terraform).
func (s *S3Store) PutDocument(ctx context.Context, key, contentType string, content []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), Body: bytes.NewReader(content),
		ContentType: aws.String(contentType), ContentLength: aws.Int64(int64(len(content))),
	})
	if err != nil {
		return unavailable("put object", err)
	}
	return nil
}

// GetDocument implements usecase.DocumentStorage.
func (s *S3Store) GetDocument(ctx context.Context, key string) (content []byte, err error) {
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		var missing *types.NoSuchKey
		if errors.As(err, &missing) {
			return nil, fmt.Errorf("get object: %w", usecase.ErrNotFound)
		}
		return nil, unavailable("get object", err)
	}
	defer func() { err = errors.Join(err, output.Body.Close()) }()
	content, err = io.ReadAll(io.LimitReader(output.Body, maxObjectBytes+1))
	if err != nil {
		return nil, unavailable("read object", err)
	}
	if len(content) > maxObjectBytes {
		return nil, fmt.Errorf("get object: larger than %d bytes", maxObjectBytes)
	}
	return content, nil
}

// DeleteDocument implements usecase.DocumentStorage.
func (s *S3Store) DeleteDocument(ctx context.Context, key string) error {
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}); err != nil {
		return unavailable("delete object", err)
	}
	return nil
}

// Disabled is the storage of an API without a bucket: every call is
// ErrDocumentStorageUnavailable, so document routes answer 503 while the rest
// of the API keeps working.
type Disabled struct{}

var _ usecase.DocumentStorage = Disabled{}

var errNotConfigured = fmt.Errorf("%w: API_DOCUMENTS_BUCKET is not configured", usecase.ErrDocumentStorageUnavailable)

// PutDocument implements usecase.DocumentStorage.
func (Disabled) PutDocument(context.Context, string, string, []byte) error { return errNotConfigured }

// GetDocument implements usecase.DocumentStorage.
func (Disabled) GetDocument(context.Context, string) ([]byte, error) { return nil, errNotConfigured }

// DeleteDocument implements usecase.DocumentStorage.
func (Disabled) DeleteDocument(context.Context, string) error { return errNotConfigured }
