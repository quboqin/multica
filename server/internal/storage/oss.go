package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	aliyunoss "github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

type ossClient interface {
	PutObject(context.Context, *aliyunoss.PutObjectRequest, ...func(*aliyunoss.Options)) (*aliyunoss.PutObjectResult, error)
	GetObject(context.Context, *aliyunoss.GetObjectRequest, ...func(*aliyunoss.Options)) (*aliyunoss.GetObjectResult, error)
	DeleteObject(context.Context, *aliyunoss.DeleteObjectRequest, ...func(*aliyunoss.Options)) (*aliyunoss.DeleteObjectResult, error)
	Presign(context.Context, any, ...func(*aliyunoss.PresignOptions)) (*aliyunoss.PresignResult, error)
}

// OSSStorage stores every Multica attachment in Alibaba Cloud OSS. The
// bucket is expected to be private unless OSS_PUBLIC_BASE_URL explicitly
// advertises a public origin; authenticated attachment endpoints continue to
// use signed URLs or the server proxy in the private-bucket case.
type OSSStorage struct {
	client        ossClient
	bucket        string
	region        string
	endpoint      string
	publicBaseURL string
}

var errOSSVerifyIntegrity = errors.New("OSS object integrity mismatch")

// NewOSSStorageFromEnv creates an OSS-backed Storage. OSS_BUCKET enables the
// backend. Region and credentials are required by OSS Signature V4; endpoint
// is optional because the SDK can derive the regional public endpoint.
//
// Environment variables:
//   - OSS_BUCKET (required to enable OSS)
//   - OSS_REGION (required)
//   - OSS_ENDPOINT (optional regional service endpoint or custom endpoint)
//   - OSS_PUBLIC_BASE_URL (optional public bucket/CDN origin)
//   - OSS_ACCESS_KEY_ID / OSS_ACCESS_KEY_SECRET (required credentials)
func NewOSSStorageFromEnv() *OSSStorage {
	bucket := strings.TrimSpace(os.Getenv("OSS_BUCKET"))
	if bucket == "" {
		return nil
	}

	region := strings.TrimSpace(os.Getenv("OSS_REGION"))
	if region == "" {
		slog.Error("OSS_BUCKET is set but OSS_REGION is empty; OSS storage disabled")
		return nil
	}
	if strings.TrimSpace(os.Getenv("OSS_ACCESS_KEY_ID")) == "" || strings.TrimSpace(os.Getenv("OSS_ACCESS_KEY_SECRET")) == "" {
		slog.Error("OSS_BUCKET is set but OSS credentials are incomplete; OSS storage disabled")
		return nil
	}

	endpoint, err := normalizeOSSBaseURL(os.Getenv("OSS_ENDPOINT"))
	if err != nil {
		slog.Error("OSS_ENDPOINT is invalid; OSS storage disabled", "error", err)
		return nil
	}
	publicBaseURL, err := normalizeOSSBaseURL(os.Getenv("OSS_PUBLIC_BASE_URL"))
	if err != nil {
		slog.Error("OSS_PUBLIC_BASE_URL is invalid; OSS storage disabled", "error", err)
		return nil
	}

	cfg := aliyunoss.LoadDefaultConfig().
		WithCredentialsProvider(credentials.NewEnvironmentVariableCredentialsProvider()).
		WithRegion(region)
	if endpoint != "" {
		cfg = cfg.WithEndpoint(endpoint)
	}

	slog.Info("Alibaba Cloud OSS storage initialized",
		"bucket", bucket,
		"region", region,
		"endpoint", endpoint,
		"public_base_url", publicBaseURL,
	)
	return &OSSStorage{
		client:        aliyunoss.NewClient(cfg),
		bucket:        bucket,
		region:        region,
		endpoint:      endpoint,
		publicBaseURL: publicBaseURL,
	}
}

func normalizeOSSBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("expected an http(s) URL")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("query strings and fragments are not supported")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func (s *OSSStorage) CdnDomain() string {
	if s.publicBaseURL == "" {
		return ""
	}
	u, err := url.Parse(s.publicBaseURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func (s *OSSStorage) privateObjectBaseURL() string {
	if s.endpoint != "" {
		u, err := url.Parse(s.endpoint)
		if err == nil && u.Host != "" {
			bucketPrefix := strings.ToLower(s.bucket) + "."
			if strings.HasPrefix(strings.ToLower(u.Hostname()), bucketPrefix) {
				return strings.TrimRight(u.String(), "/")
			}
			u.Host = s.bucket + "." + u.Host
			return strings.TrimRight(u.String(), "/")
		}
	}
	return fmt.Sprintf("https://%s.oss-%s.aliyuncs.com", s.bucket, s.region)
}

func (s *OSSStorage) objectBaseURL() string {
	if s.publicBaseURL != "" {
		return s.publicBaseURL
	}
	return s.privateObjectBaseURL()
}

func (s *OSSStorage) uploadedURL(key string) string {
	return s.objectBaseURL() + "/" + escapeOSSKey(key)
}

func escapeOSSKey(key string) string {
	parts := strings.Split(strings.TrimLeft(key, "/"), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func keyFromURLBase(rawURL, baseURL string) (string, bool) {
	raw, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	base, err := url.Parse(baseURL)
	if err != nil || !strings.EqualFold(raw.Scheme, base.Scheme) || !strings.EqualFold(raw.Host, base.Host) {
		return "", false
	}
	basePath := strings.TrimRight(base.EscapedPath(), "/") + "/"
	if !strings.HasPrefix(raw.EscapedPath(), basePath) {
		return "", false
	}
	key, err := url.PathUnescape(strings.TrimPrefix(raw.EscapedPath(), basePath))
	if err != nil || !isSafeOSSKey(key) {
		return "", false
	}
	return key, true
}

// isSafeOSSKey accepts the object-key shape produced by Multica uploads. It
// rejects traversal segments so attachment deletion cannot turn a database URL
// into an arbitrary key in the configured bucket.
func isSafeOSSKey(key string) bool {
	key = strings.TrimLeft(key, "/")
	if key == "" || path.IsAbs(key) || path.Clean(key) == "." {
		return false
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

// IsURL reports whether rawURL belongs to this OSS bucket/public origin.
// It is used by the resumable local-to-OSS migration command to skip rows
// that were already migrated in a previous run.
func (s *OSSStorage) IsURL(rawURL string) bool {
	for _, baseURL := range s.ownedObjectBaseURLs() {
		if _, ok := keyFromURLBase(rawURL, baseURL); ok {
			return true
		}
	}
	return false
}

func (s *OSSStorage) KeyFromURL(rawURL string) string {
	for _, baseURL := range s.ownedObjectBaseURLs() {
		if key, ok := keyFromURLBase(rawURL, baseURL); ok {
			return key
		}
	}
	return ""
}

func (s *OSSStorage) ownedObjectBaseURLs() []string {
	primary := s.objectBaseURL()
	private := s.privateObjectBaseURL()
	if primary == private {
		return []string{primary}
	}
	return []string{primary, private}
}

func (s *OSSStorage) Upload(ctx context.Context, key string, data []byte, contentType string, filename string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("oss PutObject: empty key")
	}
	_, err := s.client.PutObject(ctx, &aliyunoss.PutObjectRequest{
		Bucket:             aliyunoss.Ptr(s.bucket),
		Key:                aliyunoss.Ptr(key),
		Body:               bytes.NewReader(data),
		ContentLength:      aliyunoss.Ptr(int64(len(data))),
		ContentType:        aliyunoss.Ptr(contentType),
		ContentDisposition: aliyunoss.Ptr(ContentDisposition(contentType, filename)),
		CacheControl:       aliyunoss.Ptr("max-age=432000,public"),
		StorageClass:       aliyunoss.StorageClassStandard,
	})
	if err != nil {
		return "", fmt.Errorf("oss PutObject: %w", err)
	}
	return s.uploadedURL(key), nil
}

func (s *OSSStorage) GetReader(ctx context.Context, key string) (io.ReadCloser, error) {
	if key == "" {
		return nil, fmt.Errorf("oss GetObject: empty key")
	}
	out, err := s.client.GetObject(ctx, &aliyunoss.GetObjectRequest{
		Bucket: aliyunoss.Ptr(s.bucket),
		Key:    aliyunoss.Ptr(key),
	})
	if err != nil {
		return nil, fmt.Errorf("oss GetObject: %w", err)
	}
	if out == nil || out.Body == nil {
		return nil, fmt.Errorf("oss GetObject: response has no body")
	}
	return out.Body, nil
}

// Verify compares an OSS object with the expected bytes by both length and
// SHA-256. The local-to-OSS migration command uses this read-after-write check
// before it is allowed to update the attachment URL or delete a local source.
func (s *OSSStorage) Verify(ctx context.Context, key string, expected []byte) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(time.Duration(1<<uint(attempt-1)) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		if err := s.verifyOnce(ctx, key, expected); err == nil {
			return nil
		} else {
			if errors.Is(err, errOSSVerifyIntegrity) {
				return err
			}
			lastErr = err
		}
	}
	return lastErr
}

func (s *OSSStorage) verifyOnce(ctx context.Context, key string, expected []byte) error {
	reader, err := s.GetReader(ctx, key)
	if err != nil {
		return fmt.Errorf("verify OSS object: %w", err)
	}
	defer reader.Close()

	hash := sha256.New()
	read, err := io.Copy(hash, reader)
	if err != nil {
		return fmt.Errorf("verify OSS object read: %w", err)
	}
	if read != int64(len(expected)) {
		return fmt.Errorf("%w: size got %d, want %d", errOSSVerifyIntegrity, read, len(expected))
	}
	expectedHash := sha256.Sum256(expected)
	if !bytes.Equal(hash.Sum(nil), expectedHash[:]) {
		return fmt.Errorf("%w: SHA-256", errOSSVerifyIntegrity)
	}
	return nil
}

func (s *OSSStorage) Delete(ctx context.Context, key string) {
	if key == "" {
		return
	}
	_, err := s.client.DeleteObject(ctx, &aliyunoss.DeleteObjectRequest{
		Bucket: aliyunoss.Ptr(s.bucket),
		Key:    aliyunoss.Ptr(key),
	})
	if err != nil {
		slog.Error("oss DeleteObject failed", "key", key, "error", err)
	}
}

func (s *OSSStorage) DeleteKeys(ctx context.Context, keys []string) {
	for _, key := range keys {
		s.Delete(ctx, key)
	}
}

func (s *OSSStorage) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return s.PresignGetWithContentDisposition(ctx, key, ttl, "")
}

func (s *OSSStorage) PresignGetWithContentDisposition(ctx context.Context, key string, ttl time.Duration, contentDisposition string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("oss Presign: empty key")
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	request := &aliyunoss.GetObjectRequest{
		Bucket: aliyunoss.Ptr(s.bucket),
		Key:    aliyunoss.Ptr(key),
	}
	if contentDisposition != "" {
		request.ResponseContentDisposition = aliyunoss.Ptr(contentDisposition)
	}
	result, err := s.client.Presign(ctx, request, aliyunoss.PresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("oss Presign: %w", err)
	}
	if result == nil || result.URL == "" {
		return "", fmt.Errorf("oss Presign: response has no URL")
	}
	return result.URL, nil
}
