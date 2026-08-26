---
name: multica-ad-creative-prime-compose
description: Run the backend-owned deterministic Prime composition for a completed creative base. Use after the model has registered the unbranded generated and normalized base for an order variant, or after a bounded visual rework has produced a replacement base. This Skill only invokes `multica creative order prime-compose` and validates its JSON completion result; it does not edit model prompts or implement pixel composition.
allowed-tools: Bash(multica *), Bash(python *)
---

# Ad Creative Prime Composition

Run the backend Prime composition command after the output Agent has registered
the required unbranded base assets for the order variant.

## Procedure

1. Keep the production model prompt, approved copy, source structure, visual
   inheritance, size order, and Prime visual-context rules unchanged.
2. Confirm that the current order variant has completed generated and
   normalized unbranded base assets before invoking this Skill.
3. Run the bundled wrapper:

   ```text
   python references/run_prime_compose.py --order-id <order-id> --variant <variant-id> --output json
   ```

4. Treat a non-zero exit, invalid JSON, or a JSON result without an explicit
   success/completed signal as a composition failure. Do not register a
   primed or delivered asset from such a result.
5. After a successful result, continue with the backend-owned process-asset,
   primed-asset, and QC handoff recorded by the command response.

The backend evaluates every approved template family against the same frozen
unbranded bases before composition. If the highest-contrast family produces an
inconspicuous official component or a dominant bright patch and another
approved family is readable, it reselects that family and composes again in the
same request. This never asks the model to regenerate a base or recreate QR,
Logo, legal, or other Prime pixels. The evidence records the candidate scores
and whether a visual-adequacy reselect occurred.

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
