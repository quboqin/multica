# Ad Creative Prime Composition Source Map

| Contract | Source |
| --- | --- |
| The Skill invokes `multica creative order prime-compose <order-id> --variant <variant-id> --output json` as its only execution command | `references/run_prime_compose.py` |
| The backend owns deterministic image composition, Prime template validation, output assets, and composition evidence | `server/internal/handler/creative_prime_backend.go`, `server/internal/creative/primecompose` |
| `primecompose` remains the pixel-composition implementation and is not reimplemented in this Skill | `server/internal/creative/primecompose/image_prime_compose.py`, `server/internal/creative/primecompose/runner.go` |
| The model output remains an unbranded, normalized base; the Skill does not change prompt text, approved copy, layout, or Prime visual-context semantics | `scripts/creative-platform-skills/ad-creative-production/SKILL.md` |
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
