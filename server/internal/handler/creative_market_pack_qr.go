package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

const maxMarketPackTemplateBytes = 64 << 20

var requiredPrimeTemplateRoles = []string{"prime_square", "prime_landscape", "prime_portrait"}

type marketPackQRPolicy struct {
	Payload          string
	CanonicalPayload string
	ApprovalStatus   string
	AllowedDomains   []string
}

type marketPackTemplateQR struct {
	Role     string `json:"role"`
	Filename string `json:"filename"`
	Payload  string `json:"decoded_payload"`
}

type marketPackQRValidation struct {
	Status          string                 `json:"status"`
	ApprovedPayload string                 `json:"approved_payload"`
	AllowedDomain   string                 `json:"allowed_domain"`
	ValidatedAt     string                 `json:"validated_at"`
	Templates       []marketPackTemplateQR `json:"templates"`
}

func parseMarketPackQRPolicy(config json.RawMessage) (marketPackQRPolicy, error) {
	var value struct {
		Payload          string   `json:"qr_payload"`
		CanonicalPayload string   `json:"qr_canonical_payload"`
		ApprovalStatus   string   `json:"qr_approval_status"`
		AllowedDomains   []string `json:"qr_allowed_domains"`
	}
	if err := json.Unmarshal(config, &value); err != nil {
		return marketPackQRPolicy{}, errors.New("market pack QR config is invalid")
	}
	policy := marketPackQRPolicy{
		Payload:          strings.TrimSpace(value.Payload),
		CanonicalPayload: strings.TrimSpace(value.CanonicalPayload),
		ApprovalStatus:   strings.ToLower(strings.TrimSpace(value.ApprovalStatus)),
		AllowedDomains:   uniqueLowercaseStrings(value.AllowedDomains),
	}
	if policy.ApprovalStatus != "approved" {
		return marketPackQRPolicy{}, errors.New("market pack QR approval status must be approved")
	}
	if policy.Payload == "" || policy.CanonicalPayload == "" {
		return marketPackQRPolicy{}, errors.New("market pack QR payload and canonical payload are required")
	}
	if policy.Payload != policy.CanonicalPayload {
		return marketPackQRPolicy{}, errors.New("market pack QR payload must exactly match the canonical payload")
	}
	parsed, err := url.Parse(policy.Payload)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return marketPackQRPolicy{}, errors.New("market pack QR payload must be an absolute HTTPS URL")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if len(policy.AllowedDomains) == 0 || !slices.Contains(policy.AllowedDomains, hostname) {
		return marketPackQRPolicy{}, fmt.Errorf("market pack QR host %q is not an allowed domain", hostname)
	}
	return policy, nil
}

func validateMarketPackQRPolicy(policy marketPackQRPolicy, templates []marketPackTemplateQR) (marketPackQRValidation, error) {
	byRole := make(map[string]marketPackTemplateQR, len(templates))
	for _, template := range templates {
		if !slices.Contains(requiredPrimeTemplateRoles, template.Role) {
			continue
		}
		if _, exists := byRole[template.Role]; exists {
			return marketPackQRValidation{}, fmt.Errorf("market pack has more than one %s template", template.Role)
		}
		byRole[template.Role] = template
	}
	ordered := make([]marketPackTemplateQR, 0, len(requiredPrimeTemplateRoles))
	for _, role := range requiredPrimeTemplateRoles {
		template, ok := byRole[role]
		if !ok {
			return marketPackQRValidation{}, fmt.Errorf("market pack is missing the %s template", role)
		}
		if strings.TrimSpace(template.Payload) == "" {
			return marketPackQRValidation{}, fmt.Errorf("%s template QR code could not be decoded", role)
		}
		if template.Payload != policy.CanonicalPayload {
			return marketPackQRValidation{}, fmt.Errorf("%s template QR payload does not match the approved canonical payload", role)
		}
		ordered = append(ordered, template)
	}
	parsed, _ := url.Parse(policy.Payload)
	return marketPackQRValidation{
		Status:          "passed",
		ApprovedPayload: policy.Payload,
		AllowedDomain:   strings.ToLower(parsed.Hostname()),
		ValidatedAt:     time.Now().UTC().Format(time.RFC3339),
		Templates:       ordered,
	}, nil
}

func (h *Handler) validateMarketPackQRConfig(ctx context.Context, workspaceID, resourceID pgtype.UUID, config json.RawMessage) (json.RawMessage, error) {
	policy, err := parseMarketPackQRPolicy(config)
	if err != nil {
		return nil, err
	}
	files, err := h.loadCreativeResourceFiles(ctx, workspaceID, resourceID, 0)
	if err != nil {
		return nil, errors.New("market pack template files are unavailable")
	}
	templates := make([]marketPackTemplateQR, 0, len(requiredPrimeTemplateRoles))
	for _, file := range files {
		if !slices.Contains(requiredPrimeTemplateRoles, file.Role) {
			continue
		}
		if h.Storage == nil {
			return nil, errors.New("market pack template storage is unavailable")
		}
		attachmentID, parseErr := parseUUIDString(file.AttachmentID)
		if parseErr != nil {
			return nil, fmt.Errorf("%s template attachment is invalid", file.Role)
		}
		attachment, loadErr := h.Queries.GetAttachmentByIDOnly(ctx, attachmentID)
		if loadErr != nil || attachment.WorkspaceID != workspaceID {
			return nil, fmt.Errorf("%s template attachment is unavailable", file.Role)
		}
		reader, loadErr := h.Storage.GetReader(ctx, h.Storage.KeyFromURL(attachment.Url))
		if loadErr != nil {
			return nil, fmt.Errorf("%s template could not be loaded", file.Role)
		}
		payload, decodeErr := decodeQRCodeFromReader(reader)
		reader.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("%s template QR code could not be decoded", file.Role)
		}
		templates = append(templates, marketPackTemplateQR{Role: file.Role, Filename: file.Filename, Payload: payload})
	}
	validation, err := validateMarketPackQRPolicy(policy, templates)
	if err != nil {
		return nil, err
	}
	var merged map[string]any
	if err := json.Unmarshal(config, &merged); err != nil || merged == nil {
		return nil, errors.New("market pack config is invalid")
	}
	merged["qr_validation"] = validation
	return json.Marshal(merged)
}

func decodeQRCodeFromReader(reader io.Reader) (string, error) {
	img, _, err := image.Decode(io.LimitReader(reader, maxMarketPackTemplateBytes))
	if err != nil {
		return "", err
	}
	for _, candidate := range qrDecodeCandidates(img) {
		bitmap, bitmapErr := gozxing.NewBinaryBitmapFromImage(candidate)
		if bitmapErr != nil {
			continue
		}
		result, decodeErr := qrcode.NewQRCodeReader().Decode(bitmap, map[gozxing.DecodeHintType]interface{}{
			gozxing.DecodeHintType_TRY_HARDER: true,
		})
		if decodeErr == nil && strings.TrimSpace(result.GetText()) != "" {
			return strings.TrimSpace(result.GetText()), nil
		}
	}
	return "", errors.New("QR code not found")
}

func qrDecodeCandidates(img image.Image) []image.Image {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	candidates := []image.Image{img}
	if width < 2 || height < 2 {
		return candidates
	}
	regions := []image.Rectangle{
		image.Rect(width*55/100, 0, width, height*45/100),
		image.Rect(0, 0, width*45/100, height*45/100),
		image.Rect(width*55/100, height*55/100, width, height),
		image.Rect(0, height*55/100, width*45/100, height),
	}
	for _, region := range regions {
		region = region.Add(bounds.Min)
		cropped := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
		draw.Draw(cropped, cropped.Bounds(), img, region.Min, draw.Src)
		candidates = append(candidates, cropped)
	}
	return candidates
}

func uniqueLowercaseStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func stripCreativeResourceValidation(config json.RawMessage) json.RawMessage {
	var value map[string]any
	if json.Unmarshal(config, &value) != nil || value == nil {
		return config
	}
	delete(value, "qr_validation")
	stripped, err := json.Marshal(value)
	if err != nil {
		return config
	}
	return stripped
}
