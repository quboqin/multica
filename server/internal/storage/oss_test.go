package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	aliyunoss "github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
)

type fakeOSSClient struct {
	putRequest     *aliyunoss.PutObjectRequest
	getRequest     *aliyunoss.GetObjectRequest
	deleteRequest  *aliyunoss.DeleteObjectRequest
	presignRequest *aliyunoss.GetObjectRequest
	presignTTL     time.Duration
	putErr         error
	getErr         error
	deleteErr      error
	presignErr     error
	getBody        io.ReadCloser
	presignedURL   string
}

func (f *fakeOSSClient) PutObject(_ context.Context, request *aliyunoss.PutObjectRequest, _ ...func(*aliyunoss.Options)) (*aliyunoss.PutObjectResult, error) {
	f.putRequest = request
	return &aliyunoss.PutObjectResult{}, f.putErr
}

func (f *fakeOSSClient) GetObject(_ context.Context, request *aliyunoss.GetObjectRequest, _ ...func(*aliyunoss.Options)) (*aliyunoss.GetObjectResult, error) {
	f.getRequest = request
	return &aliyunoss.GetObjectResult{Body: f.getBody}, f.getErr
}

func (f *fakeOSSClient) DeleteObject(_ context.Context, request *aliyunoss.DeleteObjectRequest, _ ...func(*aliyunoss.Options)) (*aliyunoss.DeleteObjectResult, error) {
	f.deleteRequest = request
	return &aliyunoss.DeleteObjectResult{}, f.deleteErr
}

func (f *fakeOSSClient) Presign(_ context.Context, request any, options ...func(*aliyunoss.PresignOptions)) (*aliyunoss.PresignResult, error) {
	f.presignRequest, _ = request.(*aliyunoss.GetObjectRequest)
	var opts aliyunoss.PresignOptions
	for _, option := range options {
		option(&opts)
	}
	f.presignTTL = opts.Expires
	return &aliyunoss.PresignResult{URL: f.presignedURL}, f.presignErr
}

func newTestOSSStorage(client ossClient) *OSSStorage {
	return &OSSStorage{
		client:        client,
		bucket:        "assets-bucket",
		region:        "ap-southeast-3",
		endpoint:      "https://oss-ap-southeast-3.aliyuncs.com",
		publicBaseURL: "https://static.example.com/media",
	}
}

func TestOSSStorageInterfaces(t *testing.T) {
	var _ Storage = (*OSSStorage)(nil)
	var _ Presigner = (*OSSStorage)(nil)
	var _ DownloadPresigner = (*OSSStorage)(nil)
}

func TestNewOSSStorageFromEnv(t *testing.T) {
	t.Run("disabled without bucket", func(t *testing.T) {
		t.Setenv("OSS_BUCKET", "")
		if got := NewOSSStorageFromEnv(); got != nil {
			t.Fatal("expected nil storage without OSS_BUCKET")
		}
	})

	t.Run("rejects incomplete configuration", func(t *testing.T) {
		t.Setenv("OSS_BUCKET", "assets-bucket")
		t.Setenv("OSS_REGION", "")
		t.Setenv("OSS_ACCESS_KEY_ID", "test-id")
		t.Setenv("OSS_ACCESS_KEY_SECRET", "test-secret")
		if got := NewOSSStorageFromEnv(); got != nil {
			t.Fatal("expected nil storage without OSS_REGION")
		}
	})

	t.Run("normalizes endpoint and public base URL", func(t *testing.T) {
		t.Setenv("OSS_BUCKET", "assets-bucket")
		t.Setenv("OSS_REGION", "ap-southeast-3")
		t.Setenv("OSS_ENDPOINT", "oss-ap-southeast-3.aliyuncs.com/")
		t.Setenv("OSS_PUBLIC_BASE_URL", "static.example.com/media/")
		t.Setenv("OSS_ACCESS_KEY_ID", "test-id")
		t.Setenv("OSS_ACCESS_KEY_SECRET", "test-secret")

		got := NewOSSStorageFromEnv()
		if got == nil {
			t.Fatal("expected configured OSS storage")
		}
		if got.endpoint != "https://oss-ap-southeast-3.aliyuncs.com" {
			t.Fatalf("endpoint = %q", got.endpoint)
		}
		if got.publicBaseURL != "https://static.example.com/media" {
			t.Fatalf("publicBaseURL = %q", got.publicBaseURL)
		}
	})
}

func TestOSSStorageUpload(t *testing.T) {
	client := &fakeOSSClient{}
	store := newTestOSSStorage(client)

	got, err := store.Upload(context.Background(), "uploads/a b/image.png", []byte("image"), "image/png", "campaign.png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if got != "https://static.example.com/media/uploads/a%20b/image.png" {
		t.Fatalf("uploaded URL = %q", got)
	}
	if client.putRequest == nil || *client.putRequest.Bucket != "assets-bucket" || *client.putRequest.Key != "uploads/a b/image.png" {
		t.Fatalf("unexpected PutObject request: %#v", client.putRequest)
	}
	if *client.putRequest.ContentType != "image/png" || *client.putRequest.ContentLength != 5 {
		t.Fatalf("unexpected content metadata: %#v", client.putRequest)
	}
	if *client.putRequest.ContentDisposition != `inline; filename="campaign.png"` {
		t.Fatalf("content disposition = %q", *client.putRequest.ContentDisposition)
	}
	body, err := io.ReadAll(client.putRequest.Body)
	if err != nil || string(body) != "image" {
		t.Fatalf("uploaded body = %q, err = %v", body, err)
	}
}

func TestOSSStorageUploadFailure(t *testing.T) {
	store := newTestOSSStorage(&fakeOSSClient{putErr: errors.New("denied")})
	if _, err := store.Upload(context.Background(), "uploads/file.txt", []byte("x"), "text/plain", "file.txt"); err == nil {
		t.Fatal("expected PutObject error")
	}
}

func TestOSSStorageGetReaderAndDelete(t *testing.T) {
	client := &fakeOSSClient{getBody: io.NopCloser(strings.NewReader("asset"))}
	store := newTestOSSStorage(client)

	reader, err := store.GetReader(context.Background(), "uploads/file.txt")
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.Close()
	body, _ := io.ReadAll(reader)
	if string(body) != "asset" {
		t.Fatalf("body = %q", body)
	}
	if client.getRequest == nil || *client.getRequest.Key != "uploads/file.txt" {
		t.Fatalf("unexpected GetObject request: %#v", client.getRequest)
	}

	store.Delete(context.Background(), "uploads/file.txt")
	if client.deleteRequest == nil || *client.deleteRequest.Key != "uploads/file.txt" {
		t.Fatalf("unexpected DeleteObject request: %#v", client.deleteRequest)
	}
}

func TestOSSStorageVerify(t *testing.T) {
	t.Run("matching size and SHA-256", func(t *testing.T) {
		client := &fakeOSSClient{getBody: io.NopCloser(strings.NewReader("asset"))}
		store := newTestOSSStorage(client)
		if err := store.Verify(context.Background(), "uploads/file.txt", []byte("asset")); err != nil {
			t.Fatalf("Verify: %v", err)
		}
	})

	t.Run("size mismatch", func(t *testing.T) {
		client := &fakeOSSClient{getBody: io.NopCloser(strings.NewReader("short"))}
		store := newTestOSSStorage(client)
		if err := store.Verify(context.Background(), "uploads/file.txt", []byte("longer")); err == nil {
			t.Fatal("expected size mismatch")
		}
	})

	t.Run("hash mismatch", func(t *testing.T) {
		client := &fakeOSSClient{getBody: io.NopCloser(strings.NewReader("other"))}
		store := newTestOSSStorage(client)
		if err := store.Verify(context.Background(), "uploads/file.txt", []byte("asset")); err == nil {
			t.Fatal("expected SHA-256 mismatch")
		}
	})
}

func TestOSSStoragePresignGetWithContentDisposition(t *testing.T) {
	client := &fakeOSSClient{presignedURL: "https://signed.example.com/file?signature=ok"}
	store := newTestOSSStorage(client)

	got, err := store.PresignGetWithContentDisposition(
		context.Background(),
		"uploads/file.txt",
		5*time.Minute,
		`attachment; filename="report.txt"`,
	)
	if err != nil {
		t.Fatalf("PresignGetWithContentDisposition: %v", err)
	}
	if got != client.presignedURL {
		t.Fatalf("presigned URL = %q", got)
	}
	if client.presignRequest == nil || *client.presignRequest.ResponseContentDisposition != `attachment; filename="report.txt"` {
		t.Fatalf("unexpected presign request: %#v", client.presignRequest)
	}
	if client.presignTTL != 5*time.Minute {
		t.Fatalf("presign TTL = %s", client.presignTTL)
	}
}

func TestOSSStorageURLRoundTrip(t *testing.T) {
	store := newTestOSSStorage(&fakeOSSClient{})
	rawURL := "https://static.example.com/media/uploads/a%20b/image.png"
	if !store.IsURL(rawURL) {
		t.Fatalf("expected %q to belong to OSS storage", rawURL)
	}
	if got := store.KeyFromURL(rawURL); got != "uploads/a b/image.png" {
		t.Fatalf("KeyFromURL = %q", got)
	}
	if got := store.CdnDomain(); got != "static.example.com" {
		t.Fatalf("CdnDomain = %q", got)
	}
	// A public origin can be added after uploads already used the bucket URL;
	// those rows must remain readable and deletable through the same store.
	nativeURL := "https://assets-bucket.oss-ap-southeast-3.aliyuncs.com/uploads/a%20b/image.png"
	if !store.IsURL(nativeURL) {
		t.Fatalf("expected native bucket URL %q to belong to OSS storage", nativeURL)
	}
	if got := store.KeyFromURL(nativeURL); got != "uploads/a b/image.png" {
		t.Fatalf("KeyFromURL(native bucket URL) = %q", got)
	}
	if got := store.KeyFromURL("https://external.example.com/uploads/image.png"); got != "" {
		t.Fatalf("KeyFromURL(foreign URL) = %q, want empty", got)
	}
	traversalURL := "https://static.example.com/media/uploads/%2E%2E/other-attachment.png"
	if store.IsURL(traversalURL) || store.KeyFromURL(traversalURL) != "" {
		t.Fatal("encoded traversal URL must not be treated as an owned OSS key")
	}
}

func TestOSSStoragePrivateBucketURL(t *testing.T) {
	store := &OSSStorage{
		client:   &fakeOSSClient{},
		bucket:   "assets-bucket",
		region:   "ap-southeast-3",
		endpoint: "https://oss-ap-southeast-3.aliyuncs.com",
	}
	if got := store.uploadedURL("uploads/file.png"); got != "https://assets-bucket.oss-ap-southeast-3.aliyuncs.com/uploads/file.png" {
		t.Fatalf("uploaded URL = %q", got)
	}
	if got := store.CdnDomain(); got != "" {
		t.Fatalf("private bucket must not advertise a public CDN domain, got %q", got)
	}
}
