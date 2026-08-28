---
name: multica-ad-creative-prime-compose
description: Run the backend-owned Prime handoff for a completed creative base. It uses deterministic composition by default, or records a frozen QR-free model-integrated template result without a second overlay. This Skill only invokes `multica creative order prime-compose` and validates its JSON completion result; it does not edit model prompts or implement pixel composition.
allowed-tools: Bash(multica *), Bash(python *)
---

# Ad Creative Prime Composition

Run the backend Prime handoff command after the output Agent has registered the
required generated assets for the order variant. Read the frozen
`brief.prime_composition.mode`: it is `deterministic` unless the platform has
explicitly frozen `model_integrated`.

## Procedure

1. Keep the production model prompt, approved copy, source structure, visual
   inheritance, size order, and Prime visual-context rules unchanged.
2. Confirm that the current order variant has completed generated and
   normalized assets. They are unbranded in `deterministic` mode; in
   `model_integrated` mode they already contain the one frozen QR-free full
   template chosen by the platform.
3. Run the bundled wrapper:

   ```text
   python3 references/run_prime_compose.py --order-id <order-id> --variant <variant-id> --output json
   ```

   Use `--force` only for an explicit deterministic retry of a failed job on
   the same writable staging revision when the platform operator or task
   contract requests it. It does not make a completed generated asset
   writable. A successful result may report `qc_risk`; preserve that evidence
   and let final visual QC assess the actual Prime image.

4. Treat a non-zero exit, invalid JSON, or a JSON result without an explicit
   success/completed signal as a composition failure. Do not register a
   primed or delivered asset from such a result.
5. After a successful result, continue with the backend-owned process-asset,
   primed-asset, and QC handoff recorded by the command response.

`deterministic` evaluates every approved template against each delivery size's
own frozen unbranded base. Selection scope is one delivery size, so square,
landscape, and portrait may use different approved light/dark families when
their actual backgrounds require it. The evidence is based on the visible
component mask, foreground polarity, relative luminance contrast, and texture
under the real glyphs, and records every candidate score for that size.

`model_integrated` is allowed only for the frozen QR-free family validated at
market-pack publication. The backend does not run a second alpha overlay: it
promotes the model result as `primed`, records the exact selected template
attachments plus a backend-owned manifest/result, and sends the actual image
to the same final visual QC. It is not a deterministic legibility guarantee;
unreadable official text or real obstruction remains a blocking visual finding
and uses the existing bounded size-specific rework path.

Composition is fail closed for the package: every expected size must have an
adequate approved template. If any size has none, the request returns a
structured failure for that size and publishes no partial package; it never
falls back to an inadequate family or forces the other sizes to share its
family. In `deterministic` this never asks the model to recreate QR, Logo,
legal, or other Prime pixels. In `model_integrated`, the model receives the
selected QR-free full template as its only official-template input and must
not invent a QR, switch family, or add a second official component. A bounded
repair changes only the failed size and preserves the frozen mode.

## Ownership

- `creative_prime_backend.go` and `primecompose` own all image composition,
  template validation, output registration, and composition evidence.
- `references/run_prime_compose.py` is only a narrow CLI adapter. It must not
  open, transform, inspect, or rewrite image pixels.
- This Skill is not a Prime Agent and must not create a separate Prime task.
- Visual QC must inspect the actual backend-composed image. Only the existing
  bounded visual rework contract may send a failed size back to the output
  Agent.

See [references/source-map.md](references/source-map.md) for the ownership and
handoff contract.
