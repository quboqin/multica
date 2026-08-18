package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

const maxMarketPackTemplateBytes = 64 << 20

var primeCanvasSizes = map[string][2]int{
	"1080x1080": {1080, 1080},
	"1200x628":  {1200, 628},
	"800x1000":  {800, 1000},
}

var primeTemplateSizes = []string{"1080x1080", "1200x628", "800x1000"}
var primeTemplateIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)

var legacyPrimeQRConfigFields = []string{
	"qr_payload", "qr_canonical_payload", "qr_allowed_domains", "qr_approval_status", "qr_approval_note",
}

func stripLegacyPrimeQRConfigFields(config map[string]any) {
	for _, field := range legacyPrimeQRConfigFields {
		delete(config, field)
	}
}

func defaultPrimeTemplateSetConfig() map[string]any {
	return map[string]any{
		"schema_version": 2,
		"selection_mode": "automatic_family_contrast",
		"families": []any{
			map[string]any{
				"id": "light_background", "label": "明亮底图方案", "description": "绿色 AdaKami 标识，适合浅色或明亮的画面。",
				"templates": map[string]any{
					"1080x1080": map[string]any{"source_role": "prime_light_square"},
					"1200x628":  map[string]any{"source_role": "prime_light_landscape"},
					"800x1000":  map[string]any{"source_role": "prime_light_portrait"},
				},
			},
			map[string]any{
				"id": "dark_background", "label": "深色底图方案", "description": "白色 AdaKami 标识，适合深色或低明度的画面。",
				"templates": map[string]any{
					"1080x1080": map[string]any{"source_role": "prime_dark_square"},
					"1200x628":  map[string]any{"source_role": "prime_dark_landscape"},
					"800x1000":  map[string]any{"source_role": "prime_dark_portrait"},
				},
			},
		},
	}
}

type primeTemplateSetConfig struct {
	SchemaVersion int                         `json:"schema_version"`
	SelectionMode string                      `json:"selection_mode"`
	Families      []primeTemplateFamilyConfig `json:"families"`
}

type primeTemplateConfig struct {
	SourceRole string `json:"source_role"`
}

type primeTemplateFamilyConfig struct {
	ID          string                         `json:"id"`
	Label       string                         `json:"label"`
	Description string                         `json:"description"`
	Templates   map[string]primeTemplateConfig `json:"templates"`
}

type primeTemplateValidation struct {
	SourceRole       string `json:"source_role"`
	Filename         string `json:"filename"`
	SourceDimensions []int  `json:"source_dimensions"`
	HeaderEnd        int    `json:"header_end"`
	FooterStart      int    `json:"footer_start"`
	QRPayload        string `json:"qr_payload,omitempty"`
}

type primeTemplateFamilyValidation struct {
	ID        string                             `json:"id"`
	Label     string                             `json:"label"`
	Templates map[string]primeTemplateValidation `json:"templates"`
}

type primeTemplateSetValidation struct {
	Status        string                          `json:"status"`
	SchemaVersion int                             `json:"schema_version"`
	SelectionMode string                          `json:"selection_mode"`
	ValidatedAt   string                          `json:"validated_at"`
	Families      []primeTemplateFamilyValidation `json:"families"`
}

func parsePrimeTemplateSetConfig(config json.RawMessage) (*primeTemplateSetConfig, error) {
	var envelope struct {
		PrimeTemplateSet json.RawMessage `json:"prime_template_set"`
	}
	if err := json.Unmarshal(config, &envelope); err != nil {
		return nil, errors.New("market pack config is invalid")
	}
	if len(envelope.PrimeTemplateSet) == 0 || string(envelope.PrimeTemplateSet) == "null" {
		return nil, errors.New("market pack prime_template_set is required")
	}
	var templateSet primeTemplateSetConfig
	if err := json.Unmarshal(envelope.PrimeTemplateSet, &templateSet); err != nil {
		return nil, errors.New("market pack prime_template_set is invalid")
	}
	templateSet.SelectionMode = strings.ToLower(strings.TrimSpace(templateSet.SelectionMode))
	if templateSet.SchemaVersion != 2 {
		return nil, errors.New("market pack prime_template_set schema_version must be 2")
	}
	if templateSet.SelectionMode != "automatic_family_contrast" {
		return nil, errors.New("market pack prime_template_set selection_mode must be automatic_family_contrast")
	}
	if len(templateSet.Families) == 0 {
		return nil, errors.New("market pack prime_template_set requires at least one complete family")
	}
	usedRoles := map[string]struct{}{}
	seenFamilies := map[string]struct{}{}
	for index := range templateSet.Families {
		family := &templateSet.Families[index]
		family.ID = strings.ToLower(strings.TrimSpace(family.ID))
		family.Label = strings.TrimSpace(family.Label)
		family.Description = strings.TrimSpace(family.Description)
		if !primeTemplateIDPattern.MatchString(family.ID) || family.Label == "" {
			return nil, fmt.Errorf("market pack prime_template_set family %d id and label are required", index+1)
		}
		if _, duplicate := seenFamilies[family.ID]; duplicate {
			return nil, fmt.Errorf("market pack prime_template_set family id %q is duplicated", family.ID)
		}
		seenFamilies[family.ID] = struct{}{}
		if len(family.Templates) != len(primeTemplateSizes) {
			return nil, fmt.Errorf("market pack prime_template_set family %q must define every output size", family.ID)
		}
		for _, size := range primeTemplateSizes {
			template, ok := family.Templates[size]
			if !ok {
				return nil, fmt.Errorf("market pack prime_template_set family %q is missing %s", family.ID, size)
			}
			template.SourceRole = strings.TrimSpace(template.SourceRole)
			if template.SourceRole == "" {
				return nil, fmt.Errorf("market pack prime_template_set family %q %s source_role is required", family.ID, size)
			}
			if _, duplicate := usedRoles[template.SourceRole]; duplicate {
				return nil, fmt.Errorf("market pack prime_template_set source_role %q is reused across families", template.SourceRole)
			}
			usedRoles[template.SourceRole] = struct{}{}
			family.Templates[size] = template
		}
	}
	return &templateSet, nil
}

func requiredPrimeTemplateSourceRoles(templateSet *primeTemplateSetConfig) map[string]struct{} {
	required := make(map[string]struct{})
	for _, family := range templateSet.Families {
		for _, template := range family.Templates {
			required[template.SourceRole] = struct{}{}
		}
	}
	return required
}

func primeTemplateFilesByRole(templateSet *primeTemplateSetConfig, files []creativeResourceFileResponse) (map[string]creativeResourceFileResponse, error) {
	required := requiredPrimeTemplateSourceRoles(templateSet)
	matched := make(map[string]creativeResourceFileResponse, len(required))
	for _, file := range files {
		if _, wanted := required[file.Role]; !wanted {
			continue
		}
		if _, duplicate := matched[file.Role]; duplicate {
			return nil, fmt.Errorf("market pack has more than one %s template", file.Role)
		}
		matched[file.Role] = file
	}
	for role := range required {
		if _, found := matched[role]; !found {
			return nil, fmt.Errorf("market pack is missing the %s template", role)
		}
	}
	return matched, nil
}

func normalizeMarketPackPrimeTemplateSet(config json.RawMessage, files []creativeResourceFileResponse) (json.RawMessage, error) {
	var merged map[string]any
	if err := json.Unmarshal(config, &merged); err != nil || merged == nil {
		return nil, errors.New("market pack config is invalid")
	}
	if value, exists := merged["prime_template_set"]; exists && value != nil {
		return config, nil
	}
	defaultConfig := defaultPrimeTemplateSetConfig()
	defaultEnvelope, err := json.Marshal(map[string]any{"prime_template_set": defaultConfig})
	if err != nil {
		return nil, errors.New("market pack prime_template_set is invalid")
	}
	templateSet, err := parsePrimeTemplateSetConfig(defaultEnvelope)
	if err != nil {
		return nil, err
	}
	if _, err := primeTemplateFilesByRole(templateSet, files); err != nil {
		return config, nil
	}
	merged["prime_template_set"] = defaultConfig
	normalized, err := json.Marshal(merged)
	if err != nil {
		return nil, errors.New("market pack config is invalid")
	}
	return normalized, nil
}

func (h *Handler) loadPrimeTemplateImage(ctx context.Context, workspaceID pgtype.UUID, file creativeResourceFileResponse) (image.Image, error) {
	if h.Storage == nil {
		return nil, errors.New("market pack template storage is unavailable")
	}
	attachmentID, err := parseUUIDString(file.AttachmentID)
	if err != nil {
		return nil, fmt.Errorf("market pack %s template attachment is invalid", file.Role)
	}
	attachment, err := h.Queries.GetAttachmentByIDOnly(ctx, attachmentID)
	if err != nil || attachment.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("market pack %s template attachment is unavailable", file.Role)
	}
	reader, err := h.Storage.GetReader(ctx, h.Storage.KeyFromURL(attachment.Url))
	if err != nil {
		return nil, fmt.Errorf("market pack %s template could not be loaded", file.Role)
	}
	defer reader.Close()
	decoded, _, err := image.Decode(io.LimitReader(reader, maxMarketPackTemplateBytes))
	if err != nil {
		return nil, fmt.Errorf("market pack %s template is not a decodable image", file.Role)
	}
	if decoded.Bounds().Dx() < 1 || decoded.Bounds().Dy() < 1 {
		return nil, fmt.Errorf("market pack %s template has invalid dimensions", file.Role)
	}
	return decoded, nil
}

func templateBandBounds(source image.Image, targetHeight int) (int, int) {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	headerBottom, footerTop := 0, height
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			_, _, _, alpha := source.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			if alpha <= 0x0808 {
				continue
			}
			if y < height/2 {
				headerBottom = max(headerBottom, y+1)
			} else {
				footerTop = min(footerTop, y)
			}
		}
	}
	headerEnd := int(math.Ceil(float64(headerBottom) * float64(targetHeight) / float64(height)))
	footerStart := int(math.Floor(float64(footerTop) * float64(targetHeight) / float64(height)))
	return headerEnd, footerStart
}

func primeTemplateMatchesCanvas(source image.Image, canvas [2]int) bool {
	sourceWidth, sourceHeight := source.Bounds().Dx(), source.Bounds().Dy()
	if sourceWidth < 1 || sourceHeight < 1 {
		return false
	}
	sourceRatio := float64(sourceWidth) / float64(sourceHeight)
	canvasRatio := float64(canvas[0]) / float64(canvas[1])
	return math.Abs(sourceRatio-canvasRatio)/canvasRatio <= 0.002
}

func (h *Handler) validatePrimeTemplates(ctx context.Context, workspaceID pgtype.UUID, templateSet *primeTemplateSetConfig, files []creativeResourceFileResponse) ([]primeTemplateFamilyValidation, error) {
	filesByRole, err := primeTemplateFilesByRole(templateSet, files)
	if err != nil {
		return nil, err
	}
	validated := make([]primeTemplateFamilyValidation, 0, len(templateSet.Families))
	for _, family := range templateSet.Families {
		familyValidation := primeTemplateFamilyValidation{
			ID: family.ID, Label: family.Label, Templates: make(map[string]primeTemplateValidation, len(primeTemplateSizes)),
		}
		for _, size := range primeTemplateSizes {
			canvas := primeCanvasSizes[size]
			template := family.Templates[size]
			file := filesByRole[template.SourceRole]
			decoded, imageErr := h.loadPrimeTemplateImage(ctx, workspaceID, file)
			if imageErr != nil {
				return nil, imageErr
			}
			if !primeTemplateMatchesCanvas(decoded, canvas) {
				return nil, fmt.Errorf("market pack %s template dimensions do not match the %s canvas", template.SourceRole, size)
			}
			headerEnd, footerStart := templateBandBounds(decoded, canvas[1])
			// QR is optional evidence: a missing or unreadable code never blocks publication.
			qrPayload, _ := decodeQRCodeImage(decoded)
			familyValidation.Templates[size] = primeTemplateValidation{
				SourceRole: template.SourceRole, Filename: file.Filename,
				SourceDimensions: []int{decoded.Bounds().Dx(), decoded.Bounds().Dy()},
				HeaderEnd:        headerEnd, FooterStart: footerStart, QRPayload: qrPayload,
			}
		}
		validated = append(validated, familyValidation)
	}
	return validated, nil
}

func decodeQRCodeImage(img image.Image) (string, error) {
	for _, candidate := range qrDecodeCandidates(opaqueTemplateImage(img)) {
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

func opaqueTemplateImage(img image.Image) image.Image {
	bounds := img.Bounds()
	result := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(result, result.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(result, result.Bounds(), img, bounds.Min, draw.Over)
	return result
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
		cropped := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
		draw.Draw(cropped, cropped.Bounds(), img, region.Min, draw.Src)
		candidates = append(candidates, cropped)
	}
	return candidates
}

func compilePrimeTemplateLayoutContract(validations []primeTemplateFamilyValidation) map[string]any {
	layouts := make(map[string]map[string]any, len(primeTemplateSizes))
	for _, size := range primeTemplateSizes {
		canvas := primeCanvasSizes[size]
		hardRegions := make([]map[string]any, 0, len(validations)*2)
		topEnd, bottomStart := 0, canvas[1]
		for _, family := range validations {
			template := family.Templates[size]
			if template.HeaderEnd > 0 {
				hardRegions = append(hardRegions, map[string]any{"id": family.ID + ":header", "kind": "template", "x1": 0, "y1": 0, "x2": canvas[0], "y2": template.HeaderEnd})
				topEnd = max(topEnd, template.HeaderEnd)
			}
			if template.FooterStart < canvas[1] {
				hardRegions = append(hardRegions, map[string]any{"id": family.ID + ":footer", "kind": "template", "x1": 0, "y1": template.FooterStart, "x2": canvas[0], "y2": canvas[1]})
				bottomStart = min(bottomStart, template.FooterStart)
			}
		}
		layouts[size] = map[string]any{"hard_regions": hardRegions, "top_key_content_exclusion_end": topEnd, "bottom_key_content_exclusion_start": bottomStart}
	}
	return map[string]any{"guide_policy": "full_transparent_template_bands", "layouts": layouts}
}

func (h *Handler) validateMarketPackPrimeTemplates(ctx context.Context, workspaceID, resourceID pgtype.UUID, config json.RawMessage) (json.RawMessage, error) {
	files, err := h.loadCreativeResourceFiles(ctx, workspaceID, resourceID, 0)
	if err != nil {
		return nil, errors.New("market pack template files are unavailable")
	}
	normalizedConfig, err := normalizeMarketPackPrimeTemplateSet(config, files)
	if err != nil {
		return nil, err
	}
	templateSet, err := parsePrimeTemplateSetConfig(normalizedConfig)
	if err != nil {
		return nil, err
	}
	templates, err := h.validatePrimeTemplates(ctx, workspaceID, templateSet, files)
	if err != nil {
		return nil, err
	}
	var merged map[string]any
	if err := json.Unmarshal(normalizedConfig, &merged); err != nil || merged == nil {
		return nil, errors.New("market pack config is invalid")
	}
	stripLegacyPrimeQRConfigFields(merged)
	merged["prime_template_set_validation"] = primeTemplateSetValidation{
		Status: "passed", SchemaVersion: templateSet.SchemaVersion, SelectionMode: templateSet.SelectionMode,
		ValidatedAt: time.Now().UTC().Format(time.RFC3339), Families: templates,
	}
	merged["prime_layout_contract"] = compilePrimeTemplateLayoutContract(templates)
	return json.Marshal(merged)
}

func stripCreativeResourceValidation(config json.RawMessage) json.RawMessage {
	var value map[string]any
	if json.Unmarshal(config, &value) != nil || value == nil {
		return config
	}
	delete(value, "prime_template_set_validation")
	delete(value, "prime_layout_contract")
	stripped, err := json.Marshal(value)
	if err != nil {
		return config
	}
	return stripped
}
