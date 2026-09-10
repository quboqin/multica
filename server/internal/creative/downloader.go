package creative

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const defaultMaxDownloadBytes = 100 << 20

type Download struct {
	Data        []byte
	ContentType string
	Extension   string
}

type Downloader struct {
	client   *http.Client
	maxBytes int64
}

func NewDownloader(timeout time.Duration, maxBytes int64) *Downloader {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxDownloadBytes
	}
	downloader := &Downloader{maxBytes: maxBytes}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	downloader.client = &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return downloader.validateURL(req.URL)
		},
	}
	return downloader
}

func (d *Downloader) Fetch(ctx context.Context, rawURL string) (Download, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return Download{}, fmt.Errorf("parse creative asset URL: %w", err)
	}
	if err := d.validateURL(parsed); err != nil {
		return Download{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return Download{}, err
	}
	req.Header.Set("User-Agent", "Multica-Creative-Archiver/1.0")
	resp, err := d.client.Do(req)
	if err != nil {
		return Download{}, fmt.Errorf("download creative asset: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Download{}, fmt.Errorf("download creative asset returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, d.maxBytes+1))
	if err != nil {
		return Download{}, fmt.Errorf("read creative asset: %w", err)
	}
	if int64(len(data)) > d.maxBytes {
		return Download{}, fmt.Errorf("creative asset exceeds %d bytes", d.maxBytes)
	}
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(data)
	}
	return Download{
		Data:        data,
		ContentType: contentType,
		Extension:   extensionForAsset(parsed.Path, contentType),
	}, nil
}

func (d *Downloader) validateURL(value *url.URL) error {
	if value == nil || (value.Scheme != "https" && value.Scheme != "http") {
		return errors.New("creative asset URL must use HTTP or HTTPS")
	}
	if strings.TrimSpace(value.Hostname()) == "" {
		return errors.New("creative asset URL has no host")
	}
	if value.User != nil {
		return errors.New("creative asset URL must not contain user info")
	}
	return nil
}

func extensionForAsset(rawPath, contentType string) string {
	if ext := strings.ToLower(filepath.Ext(rawPath)); len(ext) >= 2 && len(ext) <= 6 {
		return ext
	}
	if extensions, _ := mime.ExtensionsByType(contentType); len(extensions) > 0 {
		return extensions[0]
	}
	return ".bin"
}
