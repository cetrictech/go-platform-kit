// Package s3 is an optional object-storage client for S3-compatible services.
// It does not load ambient credentials, install a global session, or apply bucket policy, retention, or encryption unless the caller sets them.
package s3

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"maps"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	tmtypes "github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager/types"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	smithy "github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

const (
	defaultPartSize    int64 = 8 << 20
	maxPartSize        int64 = 64 << 20
	minPartSize        int64 = 5 << 20
	smallPartSize      int64 = 64 << 10
	defaultConcurrency       = 4
	maxConcurrency           = 8
	defaultPresign           = 15 * time.Minute
	maxServicePresign        = 7 * 24 * time.Hour
	maxMetadataEntries       = 32
	maxMetadataKey           = 128
	maxMetadataValue         = 2048
	abortAfterCancel         = 10 * time.Second
)

// ErrNotFound is returned when the object key does not exist.
var ErrNotFound = errors.New("s3: object not found")

// Credentials are explicit access credentials. SessionToken may be empty.
type Credentials struct {
	AccessKeyID, SecretAccessKey, SessionToken string
}

// CredentialProvider supplies credentials for one operation. It must not log the returned secrets.
type CredentialProvider interface {
	Retrieve(context.Context) (Credentials, error)
}

type staticCredentials struct{ credentials Credentials }

// StaticCredentials returns a provider for the supplied access key. The secret is required.
func StaticCredentials(accessKeyID, secretAccessKey, sessionToken string) CredentialProvider {
	return staticCredentials{Credentials{AccessKeyID: accessKeyID, SecretAccessKey: secretAccessKey, SessionToken: sessionToken}}
}

func (s staticCredentials) Retrieve(context.Context) (Credentials, error) {
	if s.credentials.AccessKeyID == "" || s.credentials.SecretAccessKey == "" {
		return Credentials{}, errors.New("s3: access key and secret required")
	}
	return s.credentials, nil
}

type ambientCredentials struct{}

// DefaultCredentialChain reads the standard AWS credential chain when the caller explicitly selects it.
// New does not call this chain on its own.
func DefaultCredentialChain() CredentialProvider { return ambientCredentials{} }

func (ambientCredentials) Retrieve(ctx context.Context) (Credentials, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return Credentials{}, err
	}
	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return Credentials{}, err
	}
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return Credentials{}, errors.New("s3: credential chain returned no access key")
	}
	return Credentials{AccessKeyID: creds.AccessKeyID, SecretAccessKey: creds.SecretAccessKey, SessionToken: creds.SessionToken}, nil
}

type awsProvider struct{ next CredentialProvider }

func (p awsProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	creds, err := p.next.Retrieve(ctx)
	if err != nil {
		return aws.Credentials{}, err
	}
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return aws.Credentials{}, errors.New("s3: provider returned incomplete credentials")
	}
	return aws.Credentials{AccessKeyID: creds.AccessKeyID, SecretAccessKey: creds.SecretAccessKey, SessionToken: creds.SessionToken, Source: "go-platform-kit"}, nil
}

// Config selects one bucket and the credentials used to reach it.
// A zero PartSize uses 8 MiB parts. Concurrency defaults to 4 and is capped at 8.
// A zero MaxPresign caps signed URLs at 15 minutes. The service maximum is 7 days.
// AllowSmallParts permits parts below Amazon S3's 5 MiB minimum for compatible servers.
type Config struct {
	Bucket, Region, Endpoint     string
	UsePathStyle, AllowPlaintext bool
	AllowSmallParts              bool
	Credentials                  CredentialProvider
	HTTPClient                   *http.Client
	TLS                          *tls.Config
	PartSize                     int64
	Concurrency                  int
	MaxPresign                   time.Duration
}

// PutOptions are applied only to the object being written. Empty encryption stores the object without server-side encryption.
type PutOptions struct {
	ContentType          string
	Metadata             map[string]string
	ServerSideEncryption string
	KMSKeyID             string
}

// Info is object metadata without a body.
type Info struct {
	Key, ContentType, ChecksumSHA256 string
	Metadata                         map[string]string
	Size                             int64
}

// Object is a stored payload. The caller must close Body.
type Object struct {
	Info
	Body io.ReadCloser
}

// Client is safe for concurrent operations. Close releases only the transport this client constructed.
type Client struct {
	bucket     string
	api        *awss3.Client
	transfer   *transfermanager.Client
	presign    *awss3.PresignClient
	maxPresign time.Duration
	owned      *http.Client
}

// New validates configuration, checks the bucket, and returns a client that has not installed process-wide settings.
func New(ctx context.Context, c Config) (*Client, error) {
	if ctx == nil {
		return nil, errors.New("s3: context required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validToken(c.Bucket, 3, 63) || strings.Contains(c.Bucket, "/") {
		return nil, errors.New("s3: invalid bucket")
	}
	if !validToken(c.Region, 1, 64) {
		return nil, errors.New("s3: invalid region")
	}
	if c.Credentials == nil {
		return nil, errors.New("s3: credential provider required")
	}
	if c.HTTPClient != nil && c.TLS != nil {
		return nil, errors.New("s3: choose a TLS config or an HTTP client")
	}
	if c.TLS != nil && c.TLS.InsecureSkipVerify && c.TLS.VerifyPeerCertificate == nil && c.TLS.VerifyConnection == nil {
		return nil, errors.New("s3: TLS verification required")
	}
	endpoint, err := endpointURL(c.Endpoint, c.AllowPlaintext)
	if err != nil {
		return nil, err
	}
	partSize, err := boundedPartSize(c.PartSize, c.AllowSmallParts)
	if err != nil {
		return nil, err
	}
	concurrency := c.Concurrency
	if concurrency == 0 {
		concurrency = defaultConcurrency
	}
	if concurrency < 1 || concurrency > maxConcurrency {
		return nil, errors.New("s3: concurrency must be 1..8")
	}
	maxPresign := c.MaxPresign
	if maxPresign == 0 {
		maxPresign = defaultPresign
	}
	if maxPresign <= 0 || maxPresign > maxServicePresign {
		return nil, errors.New("s3: presign limit must be positive and at most 7 days")
	}
	var owned *http.Client
	httpClient := c.HTTPClient
	if httpClient == nil {
		owned = &http.Client{Transport: transport(c.TLS)}
		httpClient = owned
	}
	api := awss3.New(awss3.Options{
		Region:                       c.Region,
		Credentials:                  awsProvider{c.Credentials},
		HTTPClient:                   httpClient,
		BaseEndpoint:                 endpoint,
		UsePathStyle:                 c.UsePathStyle || endpoint != nil,
		Retryer:                      aws.NopRetryer{},
		ContinueHeaderThresholdBytes: -1,
	})
	if _, err = api.HeadBucket(ctx, &awss3.HeadBucketInput{Bucket: aws.String(c.Bucket)}); err != nil {
		if owned != nil {
			owned.CloseIdleConnections()
		}
		return nil, err
	}
	transfer := transfermanager.New(api, func(o *transfermanager.Options) {
		o.PartSizeBytes = partSize
		o.MultipartUploadThreshold = partSize
		o.Concurrency = concurrency
		o.ChecksumAlgorithm = tmtypes.ChecksumAlgorithmSha256
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.FailTimeout = abortAfterCancel
	})
	return &Client{bucket: c.Bucket, api: api, transfer: transfer, presign: awss3.NewPresignClient(api), maxPresign: maxPresign, owned: owned}, nil
}

func endpointURL(raw string, allowPlaintext bool) (*string, error) {
	if raw == "" {
		return nil, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, errors.New("s3: invalid endpoint")
	}
	if parsed.Scheme == "http" && !allowPlaintext {
		return nil, errors.New("s3: verified TLS required; explicitly opt into plaintext for local development")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	value := parsed.String()
	return &value, nil
}

func boundedPartSize(requested int64, allowSmall bool) (int64, error) {
	if requested == 0 {
		return defaultPartSize, nil
	}
	minimum := minPartSize
	if allowSmall {
		minimum = smallPartSize
	}
	if requested < minimum || requested > maxPartSize {
		return 0, errors.New("s3: part size is outside the allowed range")
	}
	return requested, nil
}

func transport(tlsConfig *tls.Config) *http.Transport {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	base.TLSHandshakeTimeout = 5 * time.Second
	base.IdleConnTimeout = 90 * time.Second
	base.MaxIdleConns = 32
	base.MaxIdleConnsPerHost = 8
	base.ExpectContinueTimeout = time.Second
	if tlsConfig != nil {
		base.TLSClientConfig = tlsConfig.Clone()
	}
	return base
}

func validToken(value string, minLen, maxLen int) bool {
	if len(value) < minLen || len(value) > maxLen || strings.TrimSpace(value) != value {
		return false
	}
	return !strings.ContainsAny(value, "\r\n\x00")
}

// Close releases idle connections of a transport this client constructed. An injected HTTP client stays open.
func (c *Client) Close() error {
	if c.owned != nil {
		c.owned.CloseIdleConnections()
	}
	return nil
}

// Put writes one object. Bodies larger than the part size use a multipart upload that is aborted when the operation fails or ctx is canceled.
// The SHA-256 checksum is calculated for the upload. The caller retains ownership of body.
func (c *Client) Put(ctx context.Context, key string, body io.Reader, options PutOptions) error {
	if body == nil {
		return errors.New("s3: body required")
	}
	if err := validateKey(key); err != nil {
		return err
	}
	metadata, contentType, err := objectHeaders(options)
	if err != nil {
		return err
	}
	input := &transfermanager.UploadObjectInput{
		Bucket:            aws.String(c.bucket),
		Key:               aws.String(key),
		Body:              body,
		Metadata:          metadata,
		ChecksumAlgorithm: tmtypes.ChecksumAlgorithmSha256,
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	switch options.ServerSideEncryption {
	case "":
	case "AES256":
		if options.KMSKeyID != "" {
			return errors.New("s3: KMS key is only valid with aws:kms encryption")
		}
		input.ServerSideEncryption = tmtypes.ServerSideEncryptionAes256
	case "aws:kms":
		if options.KMSKeyID == "" {
			return errors.New("s3: KMS key required")
		}
		input.ServerSideEncryption = tmtypes.ServerSideEncryptionAwsKms
		input.SSEKMSKeyID = aws.String(options.KMSKeyID)
	default:
		return errors.New("s3: unsupported server-side encryption")
	}
	_, err = c.transfer.UploadObject(ctx, input)
	return err
}

func objectHeaders(options PutOptions) (map[string]string, string, error) {
	if options.ContentType != "" {
		if len(options.ContentType) > 256 {
			return nil, "", errors.New("s3: content type is too long")
		}
		if _, _, err := mime.ParseMediaType(options.ContentType); err != nil {
			return nil, "", errors.New("s3: invalid content type")
		}
	}
	if len(options.Metadata) > maxMetadataEntries {
		return nil, "", errors.New("s3: too many metadata entries")
	}
	metadata := make(map[string]string, len(options.Metadata))
	for key, value := range options.Metadata {
		if !validToken(key, 1, maxMetadataKey) || len(value) > maxMetadataValue || strings.ContainsAny(value, "\r\n\x00") {
			return nil, "", errors.New("s3: invalid metadata")
		}
		metadata[key] = value
	}
	return metadata, options.ContentType, nil
}

func validateKey(key string) error {
	if !validToken(key, 1, 1024) {
		return errors.New("s3: invalid object key")
	}
	return nil
}

// Get returns a streaming body. The caller must close it.
func (c *Client) Get(ctx context.Context, key string) (*Object, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	out, err := c.api.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, classify(err)
	}
	object := &Object{Info: info(key, aws.ToString(out.ContentType), out.Metadata, aws.ToInt64(out.ContentLength), aws.ToString(out.ChecksumSHA256)), Body: out.Body}
	return object, nil
}

// Head returns object metadata.
func (c *Client) Head(ctx context.Context, key string) (Info, error) {
	if err := validateKey(key); err != nil {
		return Info{}, err
	}
	out, err := c.api.HeadObject(ctx, &awss3.HeadObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)})
	if err != nil {
		return Info{}, classify(err)
	}
	return info(key, aws.ToString(out.ContentType), out.Metadata, aws.ToInt64(out.ContentLength), aws.ToString(out.ChecksumSHA256)), nil
}

func info(key, contentType string, metadata map[string]string, size int64, checksum string) Info {
	return Info{Key: key, ContentType: contentType, Metadata: maps.Clone(metadata), Size: size, ChecksumSHA256: checksum}
}

// Delete removes one object. Deleting a missing key is successful.
func (c *Client) Delete(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	_, err := c.api.DeleteObject(ctx, &awss3.DeleteObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)})
	return err
}

// PresignGet returns a URL that expires after ttl. ttl must be positive and no greater than the client's presign limit.
func (c *Client) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return c.presignURL(ctx, key, ttl, true)
}

// PresignPut returns a URL for one PUT. The same expiration bound as PresignGet applies. Encryption and metadata are not attached.
func (c *Client) PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return c.presignURL(ctx, key, ttl, false)
}

func (c *Client) presignURL(ctx context.Context, key string, ttl time.Duration, get bool) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}
	if ttl <= 0 || ttl > c.maxPresign {
		return "", errors.New("s3: presign expiration exceeds the configured bound")
	}
	options := awss3.WithPresignExpires(ttl)
	if get {
		out, err := c.presign.PresignGetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)}, options)
		if err != nil {
			return "", err
		}
		return out.URL, nil
	}
	out, err := c.presign.PresignPutObject(ctx, &awss3.PutObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)}, options)
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

func classify(err error) error {
	var api smithy.APIError
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return ErrNotFound
		}
	}
	var response *smithyhttp.ResponseError
	if errors.As(err, &response) && response.HTTPStatusCode() == http.StatusNotFound {
		return ErrNotFound
	}
	return err
}
