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
	"regexp"
	"slices"
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

var allowedPrimeComponentIDs = []string{
	"logo", "terms", "qr", "store_badges", "regulatory", "afpi", "pindai_legal", "custom",
}

var allowedPrimeComponentKinds = []string{"image", "text", "qr"}

var allowedPrimeQRModes = []string{"none", "static", "dynamic"}

var customPrimeComponentIDPattern = regexp.MustCompile(`^custom_[a-z0-9_]{1,48}$`)

type primeCompositionConfig struct {
	SchemaVersion int                               `json:"schema_version"`
	QRMode        string                            `json:"qr_mode"`
	Components    []primeCompositionComponent       `json:"components"`
	Layouts       map[string]primeCompositionLayout `json:"layouts"`
}

type primeCompositionComponent struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Kind         string `json:"kind"`
	Enabled      bool   `json:"enabled"`
	SourceRole   string `json:"source_role"`
	Content      string `json:"content"`
	BackdropRule string `json:"backdrop_rule"`
}

type primeCompositionLayout struct {
	Components map[string]primeCompositionPlacement `json:"components"`
}

type primeCompositionPlacement struct {
	DestinationRect []int `json:"destination_rect"`
}

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
	Mode            string                 `json:"mode,omitempty"`
	ApprovedPayload string                 `json:"approved_payload"`
	AllowedDomain   string                 `json:"allowed_domain"`
	ValidatedAt     string                 `json:"validated_at"`
	Templates       []marketPackTemplateQR `json:"templates"`
}

type primeCompositionValidation struct {
	Status        string                    `json:"status"`
	SchemaVersion int                       `json:"schema_version"`
	QRMode        string                    `json:"qr_mode"`
	ValidatedAt   string                    `json:"validated_at"`
	Layouts       map[string]map[string]any `json:"layouts"`
}

func parsePrimeCompositionConfig(config json.RawMessage) (*primeCompositionConfig, error) {
	var envelope struct {
		PrimeComposition json.RawMessage `json:"prime_composition"`
	}
	if err := json.Unmarshal(config, &envelope); err != nil {
		return nil, errors.New("market pack config is invalid")
	}
	if len(envelope.PrimeComposition) == 0 || string(envelope.PrimeComposition) == "null" {
		return nil, errors.New("market pack prime_composition is required")
	}
	var composition primeCompositionConfig
	if err := json.Unmarshal(envelope.PrimeComposition, &composition); err != nil {
		return nil, errors.New("market pack prime_composition is invalid")
	}
	composition.QRMode = strings.ToLower(strings.TrimSpace(composition.QRMode))
	if composition.SchemaVersion != 2 {
		return nil, errors.New("market pack prime_composition schema_version must be 2")
	}
	if !slices.Contains(allowedPrimeQRModes, composition.QRMode) {
		return nil, errors.New("market pack prime_composition qr_mode must be none, static, or dynamic")
	}
	if len(composition.Components) == 0 {
		return nil, errors.New("market pack prime_composition components are required")
	}
	componentByID := make(map[string]primeCompositionComponent, len(composition.Components))
	activeQR := false
	for index := range composition.Components {
		component := &composition.Components[index]
		component.ID = strings.ToLower(strings.TrimSpace(component.ID))
		component.Label = strings.TrimSpace(component.Label)
		component.Kind = strings.ToLower(strings.TrimSpace(component.Kind))
		component.SourceRole = strings.TrimSpace(component.SourceRole)
		component.Content = strings.TrimSpace(component.Content)
		component.BackdropRule = strings.TrimSpace(component.BackdropRule)
		if !slices.Contains(allowedPrimeComponentIDs, component.ID) && !customPrimeComponentIDPattern.MatchString(component.ID) {
			return nil, fmt.Errorf("market pack prime component id %q is not supported", component.ID)
		}
		if _, exists := componentByID[component.ID]; exists {
			return nil, fmt.Errorf("market pack prime component id %q is duplicated", component.ID)
		}
		if !slices.Contains(allowedPrimeComponentKinds, component.Kind) {
			return nil, fmt.Errorf("market pack prime component %q kind must be image, text, or qr", component.ID)
		}
		if component.ID == "qr" && component.Kind != "qr" {
			return nil, errors.New("market pack prime component qr must use kind qr")
		}
		if component.Kind == "qr" && component.ID != "qr" {
			return nil, fmt.Errorf("market pack prime component %q cannot use kind qr", component.ID)
		}
		if component.Enabled && component.Kind == "image" && component.SourceRole == "" {
			return nil, fmt.Errorf("market pack prime image component %q source_role is required", component.ID)
		}
		if component.Enabled && component.Kind == "text" && component.Content == "" {
			return nil, fmt.Errorf("market pack prime text component %q content is required", component.ID)
		}
		if component.Kind == "qr" && component.SourceRole != "" && component.SourceRole != "prime_qr" {
			return nil, errors.New("market pack prime QR component source_role must be prime_qr")
		}
		componentByID[component.ID] = *component
		activeQR = activeQR || (component.ID == "qr" && component.Enabled)
	}
	if composition.QRMode != "none" && !activeQR {
		return nil, fmt.Errorf("market pack prime_composition qr_mode %s requires an enabled qr component", composition.QRMode)
	}
	for size, canvas := range primeCanvasSizes {
		layout, ok := composition.Layouts[size]
		if !ok {
			return nil, fmt.Errorf("market pack prime_composition is missing the %s layout", size)
		}
		for _, component := range composition.Components {
			if !primeComponentActive(component, composition.QRMode) {
				continue
			}
			placement, ok := layout.Components[component.ID]
			if !ok {
				return nil, fmt.Errorf("market pack %s layout is missing enabled component %q", size, component.ID)
			}
			if err := validatePrimeRect(placement.DestinationRect, canvas, "destination_rect"); err != nil {
				return nil, fmt.Errorf("market pack %s component %q: %w", size, component.ID, err)
			}
		}
	}
	return &composition, nil
}

func validatePrimeRectShape(rect []int, field string) error {
	if len(rect) != 4 {
		return fmt.Errorf("%s must contain [x1,y1,x2,y2]", field)
	}
	if rect[0] < 0 || rect[1] < 0 || rect[2] <= rect[0] || rect[3] <= rect[1] {
		return fmt.Errorf("%s must be a non-empty rectangle", field)
	}
	return nil
}

func validatePrimeRect(rect []int, canvas [2]int, field string) error {
	if err := validatePrimeRectShape(rect, field); err != nil {
		return err
	}
	if rect[2] > canvas[0] || rect[3] > canvas[1] {
		return fmt.Errorf("%s must be a non-empty rectangle inside %dx%d", field, canvas[0], canvas[1])
	}
	return nil
}

func primeComponentActive(component primeCompositionComponent, qrMode string) bool {
	return component.Enabled && (component.Kind != "qr" || qrMode != "none")
}

func primeComponentSourceRole(component primeCompositionComponent, qrMode string) string {
	if component.Kind == "image" {
		return component.SourceRole
	}
	if component.Kind == "qr" && qrMode == "static" {
		return "prime_qr"
	}
	return ""
}

func requiredPrimeComponentSourceRoles(composition primeCompositionConfig) map[string]struct{} {
	required := make(map[string]struct{})
	for _, component := range composition.Components {
		if !primeComponentActive(component, composition.QRMode) {
			continue
		}
		if role := primeComponentSourceRole(component, composition.QRMode); role != "" {
			required[role] = struct{}{}
		}
	}
	return required
}

func validatePrimeCompositionFiles(composition primeCompositionConfig, files []creativeResourceFileResponse) error {
	counts := make(map[string]int, len(files))
	for _, file := range files {
		counts[file.Role]++
	}
	for role := range requiredPrimeComponentSourceRoles(composition) {
		if counts[role] == 0 {
			return fmt.Errorf("market pack is missing the %s component source", role)
		}
		if counts[role] > 1 {
			return fmt.Errorf("market pack has more than one %s component source", role)
		}
	}
	return nil
}

func decodePrimeSourceImageSize(reader io.Reader) ([2]int, error) {
	decoded, _, err := image.Decode(io.LimitReader(reader, maxMarketPackTemplateBytes))
	if err != nil {
		return [2]int{}, err
	}
	bounds := decoded.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return [2]int{}, errors.New("image has invalid dimensions")
	}
	return [2]int{width, height}, nil
}

func (h *Handler) validatePrimeCompositionSourceImages(ctx context.Context, workspaceID pgtype.UUID, composition primeCompositionConfig, files []creativeResourceFileResponse) error {
	filesByRole := make(map[string]creativeResourceFileResponse, len(files))
	for _, file := range files {
		filesByRole[file.Role] = file
	}
	for role := range requiredPrimeComponentSourceRoles(composition) {
		file, ok := filesByRole[role]
		if !ok {
			return fmt.Errorf("market pack is missing the %s component source", role)
		}
		if h.Storage == nil {
			return errors.New("market pack component source storage is unavailable")
		}
		attachmentID, err := parseUUIDString(file.AttachmentID)
		if err != nil {
			return fmt.Errorf("market pack %s component source attachment is invalid", role)
		}
		attachment, err := h.Queries.GetAttachmentByIDOnly(ctx, attachmentID)
		if err != nil || attachment.WorkspaceID != workspaceID {
			return fmt.Errorf("market pack %s component source attachment is unavailable", role)
		}
		reader, err := h.Storage.GetReader(ctx, h.Storage.KeyFromURL(attachment.Url))
		if err != nil {
			return fmt.Errorf("market pack %s component source could not be loaded", role)
		}
		_, decodeErr := decodePrimeSourceImageSize(reader)
		reader.Close()
		if decodeErr != nil {
			return fmt.Errorf("market pack %s component source is not a decodable image", role)
		}
	}
	return nil
}

func compilePrimeCompositionValidation(composition primeCompositionConfig) primeCompositionValidation {
	layouts := make(map[string]map[string]any, len(primeCanvasSizes))
	for size, canvas := range primeCanvasSizes {
		layout := composition.Layouts[size]
		hardRegions := make([]map[string]any, 0, len(composition.Components))
		topEnd := 0
		bottomStart := canvas[1]
		for _, component := range composition.Components {
			if !primeComponentActive(component, composition.QRMode) {
				continue
			}
			rect := layout.Components[component.ID].DestinationRect
			region := map[string]any{
				"id": component.ID, "kind": component.Kind,
				"x1": rect[0], "y1": rect[1], "x2": rect[2], "y2": rect[3],
			}
			if component.BackdropRule != "" {
				region["backdrop_rule"] = component.BackdropRule
			}
			hardRegions = append(hardRegions, region)
			if rect[1] < canvas[1]/2 && rect[3] > topEnd {
				topEnd = rect[3]
			}
			if rect[3] > canvas[1]/2 && rect[1] < bottomStart {
				bottomStart = rect[1]
			}
		}
		layouts[size] = map[string]any{
			"hard_regions":                       hardRegions,
			"top_key_content_exclusion_end":      topEnd,
			"bottom_key_content_exclusion_start": bottomStart,
		}
	}
	return primeCompositionValidation{
		Status: "passed", SchemaVersion: composition.SchemaVersion, QRMode: composition.QRMode,
		ValidatedAt: time.Now().UTC().Format(time.RFC3339), Layouts: layouts,
	}
}

func validatePrimeCompositionQRPolicy(composition primeCompositionConfig, config json.RawMessage) (marketPackQRValidation, error) {
	validation := marketPackQRValidation{
		Status: "passed", Mode: composition.QRMode,
		ValidatedAt: time.Now().UTC().Format(time.RFC3339), Templates: []marketPackTemplateQR{},
	}
	if composition.QRMode == "none" {
		return validation, nil
	}
	if composition.QRMode == "static" {
		return marketPackQRValidation{}, errors.New("static QR mode requires source sheet validation")
	}
	policy, err := parseMarketPackQRPolicy(config)
	if err != nil {
		return marketPackQRValidation{}, err
	}
	parsed, _ := url.Parse(policy.Payload)
	validation.ApprovedPayload = policy.Payload
	validation.AllowedDomain = strings.ToLower(parsed.Hostname())
	return validation, nil
}

func validateStaticPrimeQRTemplate(template marketPackTemplateQR) (marketPackQRValidation, error) {
	if template.Role != "prime_qr" {
		return marketPackQRValidation{}, errors.New("static QR source role must be prime_qr")
	}
	template.Payload = strings.TrimSpace(template.Payload)
	if template.Payload == "" {
		return marketPackQRValidation{}, errors.New("prime_qr static QR source could not be decoded")
	}
	return marketPackQRValidation{
		Status: "passed", Mode: "static", ApprovedPayload: template.Payload,
		ValidatedAt: time.Now().UTC().Format(time.RFC3339), Templates: []marketPackTemplateQR{template},
	}, nil
}

func (h *Handler) validateStaticPrimeQRConfig(ctx context.Context, workspaceID pgtype.UUID, files []creativeResourceFileResponse) (marketPackQRValidation, error) {
	var qrFiles []creativeResourceFileResponse
	for _, file := range files {
		if file.Role == "prime_qr" {
			qrFiles = append(qrFiles, file)
		}
	}
	if len(qrFiles) != 1 {
		return marketPackQRValidation{}, errors.New("market pack static QR mode requires exactly one prime_qr source")
	}
	file := qrFiles[0]
	if h.Storage == nil {
		return marketPackQRValidation{}, errors.New("market pack template storage is unavailable")
	}
	attachmentID, err := parseUUIDString(file.AttachmentID)
	if err != nil {
		return marketPackQRValidation{}, errors.New("prime_qr static QR source attachment is invalid")
	}
	attachment, err := h.Queries.GetAttachmentByIDOnly(ctx, attachmentID)
	if err != nil || attachment.WorkspaceID != workspaceID {
		return marketPackQRValidation{}, errors.New("prime_qr static QR source attachment is unavailable")
	}
	reader, err := h.Storage.GetReader(ctx, h.Storage.KeyFromURL(attachment.Url))
	if err != nil {
		return marketPackQRValidation{}, errors.New("prime_qr static QR source could not be loaded")
	}
	payload, decodeErr := decodeQRCodeFromReader(reader)
	reader.Close()
	if decodeErr != nil {
		return marketPackQRValidation{}, errors.New("prime_qr static QR source could not be decoded")
	}
	return validateStaticPrimeQRTemplate(marketPackTemplateQR{Role: "prime_qr", Filename: file.Filename, Payload: payload})
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

func (h *Handler) validateMarketPackQRConfig(ctx context.Context, workspaceID, resourceID pgtype.UUID, config json.RawMessage) (json.RawMessage, error) {
	composition, err := parsePrimeCompositionConfig(config)
	if err != nil {
		return nil, err
	}
	files, loadErr := h.loadCreativeResourceFiles(ctx, workspaceID, resourceID, 0)
	if loadErr != nil {
		return nil, errors.New("market pack component source files are unavailable")
	}
	if validateErr := validatePrimeCompositionFiles(*composition, files); validateErr != nil {
		return nil, validateErr
	}
	if validateErr := h.validatePrimeCompositionSourceImages(ctx, workspaceID, *composition, files); validateErr != nil {
		return nil, validateErr
	}
	validation := compilePrimeCompositionValidation(*composition)
	var qrValidation marketPackQRValidation
	var policyErr error
	if composition.QRMode == "static" {
		qrValidation, policyErr = h.validateStaticPrimeQRConfig(ctx, workspaceID, files)
	} else {
		qrValidation, policyErr = validatePrimeCompositionQRPolicy(*composition, config)
	}
	if policyErr != nil {
		return nil, policyErr
	}
	var merged map[string]any
	if unmarshalErr := json.Unmarshal(config, &merged); unmarshalErr != nil || merged == nil {
		return nil, errors.New("market pack config is invalid")
	}
	merged["qr_validation"] = qrValidation
	merged["prime_composition_validation"] = validation
	merged["prime_layout_contract"] = map[string]any{
		"guide_policy": "hard_regions_compiled_from_prime_composition",
		"layouts":      validation.Layouts,
	}
	return json.Marshal(merged)
}

func decodeQRCodeFromReader(reader io.Reader) (string, error) {
	img, _, err := image.Decode(io.LimitReader(reader, maxMarketPackTemplateBytes))
	if err != nil {
		return "", err
	}
	return decodeQRCodeImage(img)
}

func decodeQRCodeImage(img image.Image) (string, error) {
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
	delete(value, "prime_composition_validation")
	if _, componentized := value["prime_composition"]; componentized {
		delete(value, "prime_layout_contract")
	}
	stripped, err := json.Marshal(value)
	if err != nil {
		return config
	}
	return stripped
}
