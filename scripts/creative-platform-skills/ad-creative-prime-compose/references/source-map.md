# Ad Creative Prime Composition Source Map

| Contract | Source |
| --- | --- |
| The Skill invokes `multica creative order prime-compose <order-id> --variant <variant-id> --output json` as its only execution command, with a 5 minute default HTTP timeout for the long-running backend composition request | `references/run_prime_compose.py` |
| The backend owns deterministic image composition, frozen QR-free model-integrated promotion, Prime template validation, output assets, and composition evidence | `server/internal/handler/creative_prime_backend.go`, `server/internal/handler/creative_market_pack_templates.go`, `server/internal/creative/primecompose` |
| `primecompose` remains the pixel-composition implementation and is not reimplemented in this Skill | `server/internal/creative/primecompose/image_prime_compose.py`, `server/internal/creative/primecompose/runner.go` |
| Before deterministic alpha composition, the composer independently selects an adequate approved template for each delivery size from actual visible-component polarity, relative-luminance contrast, and texture evidence; model-integrated mode instead uses its one frozen QR-free family and never overlays a second copy | `server/internal/creative/primecompose/image_prime_compose.py:select_template_for_size`, `server/internal/handler/creative_prime_backend.go:registerCreativeOrderVariantModelIntegratedPrime` |
| The package fails closed when any expected size has no adequate approved template; different sizes may select different families and no partial output is published | `server/internal/creative/primecompose/image_prime_compose.py:compose_manifest`, `server/internal/handler/creative_prime_backend.go` |
| Deterministic output remains an unbranded normalized base; model-integrated output uses the sole frozen QR-free full template before the same QC handoff | `scripts/creative-platform-skills/ad-creative-production/SKILL.md`, `server/internal/handler/creative_domain.go:frozenCreativePrimeCompositionContract` |
| The backend-composed result is the image inspected by visual QC and the source for the existing bounded rework handoff | `server/internal/handler/creative_domain.go`, `scripts/creative-platform-skills/ad-creative-qc/SKILL.md` |

## JSON Boundary

The wrapper accepts only the JSON output of the backend command. A result is
successful when the top-level response, or its `result` object, contains one of
these explicit completion signals:

- `success: true`
- `completed: true`
- `status: completed`, `complete`, `success`, or `succeeded`

The wrapper preserves the decoded response on stdout for the calling Agent and
returns a non-zero exit code for command failures, malformed JSON, or missing
completion signals.
