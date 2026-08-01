package handler

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

const approvedAdaKamiQR = "https://www.adakami.id/termsandconditions"

func TestValidateMarketPackQRPolicyAcceptsApprovedCanonicalTemplates(t *testing.T) {
	policy, err := parseMarketPackQRPolicy(json.RawMessage(`{
		"qr_payload":"https://www.adakami.id/termsandconditions",
		"qr_canonical_payload":"https://www.adakami.id/termsandconditions",
		"qr_approval_status":"approved",
		"qr_allowed_domains":["www.adakami.id"]
	}`))
	if err != nil {
		t.Fatalf("parse policy: %v", err)
	}

	result, err := validateMarketPackQRPolicy(policy, []marketPackTemplateQR{
		{Role: "prime_square", Filename: "11-01.png", Payload: approvedAdaKamiQR},
		{Role: "prime_landscape", Filename: "191-01.png", Payload: approvedAdaKamiQR},
		{Role: "prime_portrait", Filename: "45-01.png", Payload: approvedAdaKamiQR},
	})
	if err != nil {
		t.Fatalf("validate policy: %v", err)
	}
	if result.Status != "passed" {
		t.Fatalf("status = %q, want passed", result.Status)
	}
	if result.ApprovedPayload != approvedAdaKamiQR {
		t.Fatalf("approved payload = %q", result.ApprovedPayload)
	}
	if len(result.Templates) != 3 {
		t.Fatalf("templates = %d, want 3", len(result.Templates))
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

func TestValidateMarketPackQRPolicyRejectsTemplateMismatch(t *testing.T) {
	policy, err := parseMarketPackQRPolicy(json.RawMessage(`{
		"qr_payload":"https://www.adakami.id/termsandconditions",
		"qr_canonical_payload":"https://www.adakami.id/termsandconditions",
		"qr_approval_status":"approved",
		"qr_allowed_domains":["www.adakami.id"]
	}`))
	if err != nil {
		t.Fatalf("parse policy: %v", err)
	}

	_, err = validateMarketPackQRPolicy(policy, []marketPackTemplateQR{
		{Role: "prime_square", Filename: "11-01.png", Payload: approvedAdaKamiQR},
		{Role: "prime_landscape", Filename: "191-01.png", Payload: "https://adakami.id/promo/pembayaran-awal"},
		{Role: "prime_portrait", Filename: "45-01.png", Payload: approvedAdaKamiQR},
	})
	if err == nil || !strings.Contains(err.Error(), "prime_landscape") {
		t.Fatalf("error = %v, want prime_landscape mismatch", err)
	}
}

func TestValidateMarketPackQRPolicyRequiresEveryPrimeTemplate(t *testing.T) {
	policy, err := parseMarketPackQRPolicy(json.RawMessage(`{
		"qr_payload":"https://www.adakami.id/termsandconditions",
		"qr_canonical_payload":"https://www.adakami.id/termsandconditions",
		"qr_approval_status":"approved",
		"qr_allowed_domains":["www.adakami.id"]
	}`))
	if err != nil {
		t.Fatalf("parse policy: %v", err)
	}

	_, err = validateMarketPackQRPolicy(policy, []marketPackTemplateQR{
		{Role: "prime_square", Filename: "11-01.png", Payload: approvedAdaKamiQR},
		{Role: "prime_portrait", Filename: "45-01.png", Payload: approvedAdaKamiQR},
	})
	if err == nil || !strings.Contains(err.Error(), "prime_landscape") {
		t.Fatalf("error = %v, want missing prime_landscape", err)
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

func TestDecodeConfiguredPrimeTemplates(t *testing.T) {
	directory := os.Getenv("MULTICA_PRIME_TEMPLATE_DIR")
	if directory == "" {
		t.Skip("MULTICA_PRIME_TEMPLATE_DIR is not configured")
	}
	for _, filename := range []string{"11-01.png", "191-01.png", "45-01.png"} {
		t.Run(filename, func(t *testing.T) {
			file, err := os.Open(filepath.Join(directory, filename))
			if err != nil {
				t.Fatalf("open template: %v", err)
			}
			defer file.Close()
			payload, err := decodeQRCodeFromReader(file)
			if err != nil {
				t.Fatalf("decode QR: %v", err)
			}
			if payload != approvedAdaKamiQR {
				t.Fatalf("payload = %q, want %q", payload, approvedAdaKamiQR)
			}
		})
	}
}
