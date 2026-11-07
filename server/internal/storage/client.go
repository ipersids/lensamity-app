package storage

import (
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

type Config struct {
	Region           string
	AccessKeyID      string
	SecretAccessKey  string
	InternalEndpoint string
	PublicEndpoint   string
	UsePathStyle     bool
	Bucket           string
}

type Client struct {
	Bucket  string
	Client  *s3.Client
	Presign *s3.PresignClient
}

func NewS3Client(c Config) (*Client, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	// build aws.Config for server-side S3 calls inside the Docker network.
	cfg := aws.Config{
		Region:       c.Region,
		BaseEndpoint: &c.InternalEndpoint,
		Credentials:  aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(c.AccessKeyID, c.SecretAccessKey, "")),
	}

	// build S3 client
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = c.UsePathStyle
	})

	if client == nil {
		return nil, errors.New("failed to initialize S3 client from config")
	}

	// build aws.Config for URLs returned to the browser.
	presignCfg := aws.Config{
		Region:       c.Region,
		BaseEndpoint: &c.PublicEndpoint,
		Credentials:  aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(c.AccessKeyID, c.SecretAccessKey, "")),
	}

	presignClient := s3.NewPresignClient(s3.NewFromConfig(presignCfg, func(o *s3.Options) {
		o.UsePathStyle = c.UsePathStyle
	}))

	if presignClient == nil {
		return nil, errors.New("failed to initialize S3 presign client from config")
	}

	return &Client{
		Bucket:  c.Bucket,
		Client:  client,
		Presign: presignClient,
	}, nil
}

func (c Config) validate() error {
	if strings.TrimSpace(c.Region) == "" {
		return errors.New("storage: region is required")
	}
	if strings.TrimSpace(c.AccessKeyID) == "" {
		return errors.New("storage: access key id is required")
	}
	if strings.TrimSpace(c.SecretAccessKey) == "" {
		return errors.New("storage: secret access key is required")
	}
	if strings.TrimSpace(c.InternalEndpoint) == "" {
		return errors.New("storage: internal endpoint is required")
	}
	if strings.TrimSpace(c.PublicEndpoint) == "" {
		return errors.New("storage: public endpoint is required")
	}
	if strings.TrimSpace(c.Bucket) == "" {
		return errors.New("storage: bucket is required")
	}
	return nil
}

func IsObjectNotFound(err error) bool {
	var notFound *s3types.NotFound
	if errors.As(err, &notFound) {
		return true
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey", "404":
			return true
		}
	}

	var responseErr *awshttp.ResponseError
	return errors.As(err, &responseErr) && responseErr.HTTPStatusCode() == 404
}
