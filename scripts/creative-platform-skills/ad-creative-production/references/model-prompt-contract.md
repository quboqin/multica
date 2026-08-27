# GPT Image Model Prompt Contract

This file is the only executable source for standard-production model prompt shape. Agent instructions, initialization scripts, task
metadata, and Variant briefs must not copy or fork this template.

## Boundary

The prompt contains only instructions that can change pixels. Do not include order, item, Variant, revision, task, attachment, file,
hash, request, upload, registration, retry, timeout, status, JSON, CLI, audit, or lineage instructions. Those remain in the workflow
outside the model call.

GPT Image renders all approved headline, benefit, amount, table, supporting copy, and CTA text into the unbranded base. Do not reserve
blank text boxes for later code rendering and do not ask the platform to typeset, patch, or overlay business copy. The only later
deterministic overlay is official Prime.

Aim for 900-1800 characters plus the approved copy. Dense repayment tables or App UI replacement may use up to about 2800 characters;
the hard limit is 3600. State each constraint once.

## Fixed sections

Replace every placeholder. Send exactly these sections in this order:

```text
TASK
Create one unbranded {canvas_lock} ad base. Redesign the source identity while preserving the approved business mechanism.

INPUT ROLES
{input_roles}

LOCKED DESIGN DNA
{design_dna_invariants}

EDITABLE LAYOUT
{layout_plan_for_this_size}

APPROVED COPY
Render every approved string and frozen repayment row exactly once: {approved_copy_and_table}

PRIME SUPPORT
Input 2 is the current-size official Prime visual context. Keep its future component areas clear of business content and support its
actual light, dark, or mixed foreground with a calm low-detail background of sufficient contrast. Prime is visual context only: never
draw or copy its logo, QR, store badge, legal text, regulatory mark, template wording, or component geometry.

ACCEPTANCE
All approved copy and content groups are present and legible; the requested canvas is preserved; DesignDNA remains recognizable; no
competitor identity, extra claim, duplicate copy, Prime element, story frame, phone screenshot, long poster, scrolling page, 9:16, or
9:19 canvas is present.
```

The canvas locks are:

- `1080x1080 locked 1:1 square`;
- `1200x628 locked 1.91:1 landscape`;
- `800x1000 locked 4:5 portrait, only 1.25 times as high as wide`.

## Input roles

Input 1 is always the downloaded candidate source and is used only for business structure, reading order, and permitted visual
anchors. It is never the editable square base for landscape or portrait.

Input 2 is always the current-size, current-revision official Prime visual context.

Input 3 exists only when `creative_contract.app_ui_replacement.selected=true`: it is the selected first-party App UI for visible
phone-screen content only. Preserve the phone, hand, perspective, reflections, lighting, occlusion, and scene; remove competitor UI
identity and do not draw the selected UI outside the screen.

Input 4 is optional and may be the selected primary image for DesignDNA/identity consistency only. It must not donate layout, crop,
copy placement, Prime pixels, or canvas proportions. Missing Input 4 does not block `family_consistent` generation. Under
`strict_identity`, the declared identity anchor must be present, but that is an identity dependency rather than a square-image
dependency.

For bounded visual rework, Input 1 is instead the failed same-size unbranded base; Input 2 remains current-size Prime context. Keep the
same DesignDNA, approved copy, and business facts and edit only the failed acceptance targets.

## Compilation

Compile the prompt from `creative_intent`, `design_dna`, the current `layout_plan`, approved copy, and semantic Input roles. Do not paste
the JSON or field names into the prompt. If `prime_support.background_polarity=adaptive`, describe sufficient visual separation from
Input 2 without inventing a fixed light or dark background. If a structured polarity is supplied, express that polarity once.

Run `validate_copy_snapshot.py --require-prime-guard --require-redesign-guard --require-concise-prompt --explain` before the model call.
Repair only the reported rule. The validator checks approved financial values, this section contract, prompt length, and exclusion of
workflow protocol from model-facing text.
