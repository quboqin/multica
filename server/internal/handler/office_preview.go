package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"time"
)

const maxOfficePreviewSize = 20 << 20
const maxOfficePDFSize = 50 << 20

// Conversion is isolated in a private Gotenberg service with no storage or
// credentials mounted. Never forward user-provided URLs or request headers.
var officePreviewSlots = make(chan struct{}, 2)

func officePreviewExtension(contentType, filename string) string {
	ext := strings.ToLower(path.Ext(filename))
	switch ext {
	case ".doc", ".docx", ".ppt", ".pptx":
		return ext
	}
	switch strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])) {
	case "application/msword":
		return ".doc"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "application/vnd.ms-powerpoint":
		return ".ppt"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return ".pptx"
	}
	return ""
}

func (h *Handler) GetAttachmentOfficePreview(w http.ResponseWriter, r *http.Request) {
	att, ok := h.loadAttachmentForRequest(w, r)
	if !ok {
		return
	}
	ext := officePreviewExtension(att.ContentType, att.Filename)
	if ext == "" {
		writeError(w, http.StatusUnsupportedMediaType, "preview not supported for this file type")
		return
	}
	if h.cfg.OfficePreviewURL == "" || h.Storage == nil {
		writeError(w, http.StatusServiceUnavailable, "office preview is not configured")
		return
	}
	if att.SizeBytes > maxOfficePreviewSize {
		writeError(w, http.StatusRequestEntityTooLarge, "file too large for inline preview")
		return
	}
	select {
	case officePreviewSlots <- struct{}{}:
		defer func() { <-officePreviewSlots }()
	default:
		w.Header().Set("Retry-After", "5")
		writeError(w, http.StatusServiceUnavailable, "office preview is busy; try again")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 65*time.Second)
	defer cancel()
	reader, err := h.Storage.GetReader(ctx, h.Storage.KeyFromURL(att.Url))
	if err != nil {
		writeError(w, http.StatusNotFound, "attachment object not found")
		return
	}
	defer reader.Close()
	body, err := io.ReadAll(io.LimitReader(reader, maxOfficePreviewSize+1))
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to read attachment")
		return
	}
	if len(body) > maxOfficePreviewSize {
		writeError(w, http.StatusRequestEntityTooLarge, "file too large for inline preview")
		return
	}
	pdf, err := convertOfficePreview(ctx, h.cfg.OfficePreviewURL, ext, body)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("office preview conversion failed", "attachment_id", uuidToString(att.ID), "error", err)
		}
		writeError(w, http.StatusBadGateway, "office preview conversion failed")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="preview.pdf"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	h.setAttachmentPreviewSecurityHeaders(w)
	w.Header().Set("Content-Length", fmt.Sprint(len(pdf)))
	_, _ = w.Write(pdf)
}

func convertOfficePreview(ctx context.Context, serviceURL, ext string, body []byte) ([]byte, error) {
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	file, err := writer.CreateFormFile("files", "document"+ext)
	if err != nil {
		return nil, err
	}
	if _, err = file.Write(body); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(serviceURL, "/")+"/forms/libreoffice/convert", &payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{
		Timeout:       65 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("converter status %d", res.StatusCode)
	}
	pdf, err := io.ReadAll(io.LimitReader(res.Body, maxOfficePDFSize+1))
	if err != nil {
		return nil, err
	}
	if len(pdf) > maxOfficePDFSize || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		return nil, fmt.Errorf("invalid converter PDF")
	}
	return pdf, nil
}
