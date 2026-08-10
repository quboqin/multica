package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"slices"
	"strings"
	"testing"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

const approvedAdaKamiQR = "https://www.adakami.id/termsandconditions"

func componentizedPrimeConfig(qrMode string, qrEnabled bool) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{
		"prime_composition":{
			"schema_version":2,
			"qr_mode":%q,
			"components":[
				{"id":"logo","label":"Logo","kind":"image","enabled":true,"source_role":"prime_logo","backdrop_rule":"low_texture"},
				{"id":"qr","label":"QR","kind":"qr","enabled":%t,"source_role":"prime_qr","backdrop_rule":"light"}
			],
			"layouts":{
				"1080x1080":{"components":{"logo":{"destination_rect":[30,28,312,99]},"qr":{"destination_rect":[983,29,1053,99]}}},
				"1200x628":{"components":{"logo":{"destination_rect":[21,22,214,70]},"terms":{"destination_rect":[946,22,1116,80]},"qr":{"destination_rect":[1120,16,1192,88]}}},
				"800x1000":{"components":{"logo":{"destination_rect":[25,23,234,76]},"terms":{"destination_rect":[540,24,718,82]},"qr":{"destination_rect":[722,18,790,86]}}}
			}
		},
		"qr_payload":%q,
		"qr_canonical_payload":%q,
		"qr_approval_status":"approved",
		"qr_allowed_domains":["www.adakami.id"]
	}`, qrMode, qrEnabled, approvedAdaKamiQR, approvedAdaKamiQR))
}

func TestParsePrimeCompositionRequiresVersion2AndComposition(t *testing.T) {
	if _, err := parsePrimeCompositionConfig(json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "prime_composition is required") {
		t.Fatalf("missing composition error = %v", err)
	}
	v1 := bytes.Replace(componentizedPrimeConfig("none", false), []byte(`"schema_version":2`), []byte(`"schema_version":1`), 1)
	if _, err := parsePrimeCompositionConfig(v1); err == nil || !strings.Contains(err.Error(), "schema_version must be 2") {
		t.Fatalf("v1 error = %v", err)
	}
}

func TestParsePrimeCompositionValidatesImageAndTextSources(t *testing.T) {
	missingImageRole := bytes.Replace(componentizedPrimeConfig("none", false), []byte(`"source_role":"prime_logo"`), []byte(`"source_role":""`), 1)
	if _, err := parsePrimeCompositionConfig(missingImageRole); err == nil || !strings.Contains(err.Error(), "source_role must be prime_logo") {
		t.Fatalf("missing image source role error = %v", err)
	}

	withText := bytes.Replace(
		componentizedPrimeConfig("none", false),
		[]byte(`{"id":"qr","label":"QR","kind":"qr","enabled":false,"source_role":"prime_qr","backdrop_rule":"light"}`),
		[]byte(`{"id":"custom_terms","label":"Terms","kind":"text","enabled":true,"content":"Representative terms","backdrop_rule":"light"}`),
		1,
	)
	withText = bytes.ReplaceAll(withText, []byte(`"qr":{"destination_rect"`), []byte(`"custom_terms":{"destination_rect"`))
	composition, err := parsePrimeCompositionConfig(withText)
	if err != nil {
		t.Fatalf("parse text component: %v", err)
	}
	if err := validatePrimeCompositionFiles(*composition, []creativeResourceFileResponse{{Role: "prime_logo"}}); err != nil {
		t.Fatalf("text component unexpectedly required a file: %v", err)
	}

	missingContent := bytes.Replace(withText, []byte(`"content":"Representative terms"`), []byte(`"content":"  "`), 1)
	if _, err := parsePrimeCompositionConfig(missingContent); err == nil || !strings.Contains(err.Error(), "content is required") {
		t.Fatalf("missing text content error = %v", err)
	}

	standardTermsText := bytes.Replace(
		componentizedPrimeConfig("none", false),
		[]byte(`{"id":"qr","label":"QR","kind":"qr","enabled":false,"source_role":"prime_qr","backdrop_rule":"light"}`),
		[]byte(`{"id":"terms","label":"Terms","kind":"text","enabled":true,"content":"Representative terms"}`),
		1,
	)
	standardTermsText = bytes.ReplaceAll(standardTermsText, []byte(`"qr":{"destination_rect"`), []byte(`"terms":{"destination_rect"`))
	if _, err := parsePrimeCompositionConfig(standardTermsText); err == nil || !strings.Contains(err.Error(), "must use an image source") {
		t.Fatalf("standard text component error = %v", err)
	}
}

func TestParsePrimeCompositionSupportsQRModesAndCompilesHardRegions(t *testing.T) {
	for _, test := range []struct {
		mode         string
		qrEnabled    bool
		wantRegions  int
		wantQRActive bool
	}{
		{mode: "none", qrEnabled: true, wantRegions: 1, wantQRActive: false},
		{mode: "static", qrEnabled: true, wantRegions: 2, wantQRActive: true},
		{mode: "dynamic", qrEnabled: true, wantRegions: 2, wantQRActive: true},
	} {
		t.Run(test.mode, func(t *testing.T) {
			composition, err := parsePrimeCompositionConfig(componentizedPrimeConfig(test.mode, test.qrEnabled))
			if err != nil {
				t.Fatalf("parse composition: %v", err)
			}
			if composition == nil || composition.QRMode != test.mode {
				t.Fatalf("composition = %#v, want mode %q", composition, test.mode)
			}
			validation := compilePrimeCompositionValidation(*composition)
			regions := validation.Layouts["1080x1080"]["hard_regions"].([]map[string]any)
			if len(regions) != test.wantRegions {
				t.Fatalf("hard regions = %d, want %d", len(regions), test.wantRegions)
			}
			qrActive := slices.ContainsFunc(regions, func(region map[string]any) bool { return region["id"] == "qr" })
			if qrActive != test.wantQRActive {
				t.Fatalf("QR active = %t, want %t", qrActive, test.wantQRActive)
			}
		})
	}
}

func TestParsePrimeCompositionRequiresEnabledQRForStaticAndDynamic(t *testing.T) {
	for _, mode := range []string{"static", "dynamic"} {
		t.Run(mode, func(t *testing.T) {
			_, err := parsePrimeCompositionConfig(componentizedPrimeConfig(mode, false))
			if err == nil || !strings.Contains(err.Error(), "enabled qr component") {
				t.Fatalf("error = %v, want enabled qr component", err)
			}
		})
	}
}

func TestPrimeCompositionQRValidationDependsOnMode(t *testing.T) {
	none, err := parsePrimeCompositionConfig(componentizedPrimeConfig("none", true))
	if err != nil {
		t.Fatalf("parse none composition: %v", err)
	}
	noneValidation, err := validatePrimeCompositionQRPolicy(*none, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("validate none without QR policy: %v", err)
	}
	if noneValidation.Mode != "none" || noneValidation.ApprovedPayload != "" {
		t.Fatalf("validation = %#v, want none without approved payload", noneValidation)
	}

	dynamic, err := parsePrimeCompositionConfig(componentizedPrimeConfig("dynamic", true))
	if err != nil {
		t.Fatalf("parse dynamic composition: %v", err)
	}
	if _, err := validatePrimeCompositionQRPolicy(*dynamic, json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "approval status") {
		t.Fatalf("dynamic error = %v, want QR policy failure", err)
	}
	validation, err := validatePrimeCompositionQRPolicy(*dynamic, componentizedPrimeConfig("dynamic", true))
	if err != nil {
		t.Fatalf("validate dynamic: %v", err)
	}
	if validation.ApprovedPayload != approvedAdaKamiQR {
		t.Fatalf("approved payload = %q", validation.ApprovedPayload)
	}
}

func TestValidateStaticPrimeQRTemplateUsesSinglePrimeQR(t *testing.T) {
	template := marketPackTemplateQR{Role: "prime_qr", Payload: approvedAdaKamiQR}
	validation, err := validateStaticPrimeQRTemplate(template)
	if err != nil {
		t.Fatalf("validate static template: %v", err)
	}
	if validation.Mode != "static" || validation.ApprovedPayload != approvedAdaKamiQR || len(validation.Templates) != 1 {
		t.Fatalf("validation = %#v", validation)
	}
	if _, err := validateStaticPrimeQRTemplate(marketPackTemplateQR{Role: "prime_square", Payload: approvedAdaKamiQR}); err == nil || !strings.Contains(err.Error(), "prime_qr") {
		t.Fatalf("wrong role error = %v", err)
	}
	if _, err := validateStaticPrimeQRTemplate(marketPackTemplateQR{Role: "prime_qr"}); err == nil || !strings.Contains(err.Error(), "could not be decoded") {
		t.Fatalf("empty payload error = %v", err)
	}
}

func TestParsePrimeCompositionAcceptsNamespacedCustomComponentIDs(t *testing.T) {
	for _, id := range []string{"custom_1", "custom_2", "custom_terms"} {
		t.Run(id, func(t *testing.T) {
			config := bytes.ReplaceAll(componentizedPrimeConfig("none", false), []byte(`"logo"`), []byte(fmt.Sprintf("%q", id)))
			if _, err := parsePrimeCompositionConfig(config); err != nil {
				t.Fatalf("parse %s: %v", id, err)
			}
		})
	}
	config := bytes.ReplaceAll(componentizedPrimeConfig("none", false), []byte(`"logo"`), []byte(`"custom-invalid"`))
	if _, err := parsePrimeCompositionConfig(config); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("invalid custom id error = %v", err)
	}
}

func TestPrimeCompositionSourceRoleAndFileValidation(t *testing.T) {
	composition, err := parsePrimeCompositionConfig(componentizedPrimeConfig("static", true))
	if err != nil {
		t.Fatalf("parse composition: %v", err)
	}
	files := []creativeResourceFileResponse{
		{Role: "prime_logo"}, {Role: "prime_qr"},
	}
	if err := validatePrimeCompositionFiles(*composition, files); err != nil {
		t.Fatalf("validate files: %v", err)
	}
	if role := primeComponentSourceRole(composition.Components[0], composition.QRMode); role != "prime_logo" {
		t.Fatalf("resolved role = %q, want prime_logo", role)
	}
	if err := validatePrimeCompositionFiles(*composition, files[:1]); err == nil || !strings.Contains(err.Error(), "prime_qr") {
		t.Fatalf("error = %v, want missing prime_qr", err)
	}
	if err := validatePrimeCompositionFiles(*composition, append(files, creativeResourceFileResponse{Role: "prime_qr"})); err == nil || !strings.Contains(err.Error(), "more than one prime_qr") {
		t.Fatalf("error = %v, want duplicate prime_qr", err)
	}
}

func TestPrimeCompositionFileRequirementsDependOnQRMode(t *testing.T) {
	for _, mode := range []string{"none", "dynamic"} {
		t.Run(mode, func(t *testing.T) {
			composition, err := parsePrimeCompositionConfig(componentizedPrimeConfig(mode, true))
			if err != nil {
				t.Fatalf("parse composition: %v", err)
			}
			if err := validatePrimeCompositionFiles(*composition, []creativeResourceFileResponse{{Role: "prime_logo"}}); err != nil {
				t.Fatalf("%s mode unexpectedly required a QR source: %v", mode, err)
			}
		})
	}
}

func TestDecodePrimeSourceImageSizeRejectsDamagedFiles(t *testing.T) {
	if _, err := decodePrimeSourceImageSize(bytes.NewReader([]byte("not an image"))); err == nil {
		t.Fatal("damaged source image unexpectedly decoded")
	}
	canvas := image.NewRGBA(image.Rect(0, 0, 320, 180))
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatalf("encode source image: %v", err)
	}
	dimension, err := decodePrimeSourceImageSize(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("decode source image: %v", err)
	}
	if dimension != [2]int{320, 180} {
		t.Fatalf("dimension = %v, want 320x180", dimension)
	}
}

func TestParsePrimeCompositionRejectsOutOfCanvasPlacement(t *testing.T) {
	config := bytes.Replace(
		componentizedPrimeConfig("none", false),
		[]byte(`"destination_rect":[30,28,312,99]`),
		[]byte(`"destination_rect":[30,28,1200,99]`),
		1,
	)
	_, err := parsePrimeCompositionConfig(config)
	if err == nil || !strings.Contains(err.Error(), "inside 1080x1080") {
		t.Fatalf("error = %v, want canvas bounds failure", err)
	}
}

func TestParseMarketPackQRPolicyRejectsUnapprovedOrUnsafePayloads(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   string
	}{
		{
			name: "approval required",
			config: `{"qr_payload":"https://www.adakami.id/termsandconditions",` +
				`"qr_canonical_payload":"https://www.adakami.id/termsandconditions",` +
				`"qr_approval_status":"pending","qr_allowed_domains":["www.adakami.id"]}`,
			want: "approval status",
		},
		{
			name: "https required",
			config: `{"qr_payload":"http://www.adakami.id/termsandconditions",` +
				`"qr_canonical_payload":"http://www.adakami.id/termsandconditions",` +
				`"qr_approval_status":"approved","qr_allowed_domains":["www.adakami.id"]}`,
			want: "HTTPS",
		},
		{
			name: "canonical mismatch",
			config: `{"qr_payload":"https://adakami.id/promo/pembayaran-awal",` +
				`"qr_canonical_payload":"https://www.adakami.id/termsandconditions",` +
				`"qr_approval_status":"approved","qr_allowed_domains":["www.adakami.id"]}`,
			want: "canonical",
		},
		{
			name: "domain allowlist",
			config: `{"qr_payload":"https://www.adakami.id/termsandconditions",` +
				`"qr_canonical_payload":"https://www.adakami.id/termsandconditions",` +
				`"qr_approval_status":"approved","qr_allowed_domains":["example.com"]}`,
			want: "allowed domain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseMarketPackQRPolicy(json.RawMessage(tt.config))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestDecodeQRCodeFromReaderFindsQRCodeInPrimeCorner(t *testing.T) {
	matrix, err := qrcode.NewQRCodeWriter().EncodeWithoutHint(
		approvedAdaKamiQR,
		gozxing.BarcodeFormat_QR_CODE,
		300,
		300,
	)
	if err != nil {
		t.Fatalf("encode QR: %v", err)
	}
	canvas := image.NewRGBA(image.Rect(0, 0, 1200, 628))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for y := 0; y < matrix.GetHeight(); y++ {
		for x := 0; x < matrix.GetWidth(); x++ {
			pixel := color.White
			if matrix.Get(x, y) {
				pixel = color.Black
			}
			canvas.Set(880+x, 20+y, pixel)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatalf("encode image: %v", err)
	}
	payload, err := decodeQRCodeFromReader(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("decode QR: %v", err)
	}
	if payload != approvedAdaKamiQR {
		t.Fatalf("payload = %q, want %q", payload, approvedAdaKamiQR)
	}
}
