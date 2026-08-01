package creative

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
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
	client            *http.Client
	allowedHostClient *http.Client
	maxBytes          int64
	allowedHosts      []string
}

func NewDownloader(timeout time.Duration, maxBytes int64, allowedHosts []string) *Downloader {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxDownloadBytes
	}
	normalizedHosts := make([]string, 0, len(allowedHosts))
	for _, host := range allowedHosts {
		if host = strings.ToLower(strings.TrimSpace(host)); host != "" {
			normalizedHosts = append(normalizedHosts, host)
		}
	}
	downloader := &Downloader{maxBytes: maxBytes, allowedHosts: normalizedHosts}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = downloader.dialContext
	downloader.client = downloader.newHTTPClient(timeout, transport, false)
	allowedHostTransport := http.DefaultTransport.(*http.Transport).Clone()
	allowedHostTransport.Proxy = http.ProxyFromEnvironment
	downloader.allowedHostClient = downloader.newHTTPClient(timeout, allowedHostTransport, true)
	return downloader
}

func (d *Downloader) newHTTPClient(timeout time.Duration, transport http.RoundTripper, requireAllowedHost bool) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if err := d.validateURL(req.URL); err != nil {
				return err
			}
			if requireAllowedHost && !d.hostExplicitlyAllowed(req.URL.Hostname()) {
				return fmt.Errorf("creative asset redirect host %q is not allowed", req.URL.Hostname())
			}
			return nil
		},
	}
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
	client := d.client
	if d.hostExplicitlyAllowed(parsed.Hostname()) {
		client = d.allowedHostClient
	}
	resp, err := client.Do(req)
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

func (d *Downloader) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if d.hostExplicitlyAllowed(host) {
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, address)
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, address := range addresses {
		if !publicCreativeAddress(address) {
			continue
		}
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(address.String(), port))
	}
	return nil, fmt.Errorf("creative asset host %q did not resolve to a public address", host)
}

func (d *Downloader) hostExplicitlyAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for _, allowed := range d.allowedHosts {
		if strings.HasPrefix(allowed, "*.") {
			if strings.HasSuffix(host, strings.TrimPrefix(allowed, "*")) {
				return true
			}
			continue
		}
		if host == allowed {
			return true
		}
	}
	return false
}

func publicCreativeAddress(address netip.Addr) bool {
	return address.IsValid() && !address.IsUnspecified() && !address.IsLoopback() &&
		!address.IsPrivate() && !address.IsLinkLocalUnicast() && !address.IsLinkLocalMulticast() &&
		!address.IsMulticast()
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
