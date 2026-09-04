# GPT Image Model Prompt Contract

This file is the only executable source for standard-production model prompt shape. Agent instructions, initialization scripts, task
metadata, and Variant briefs must not copy or fork this template.

## Boundary

The prompt contains only instructions that can change pixels. Do not include order, item, Variant, revision, task, attachment, file,
hash, request, upload, registration, retry, timeout, status, JSON, CLI, audit, or lineage instructions. Those remain in the workflow
outside the model call.

GPT Image renders all approved headline, benefit, amount, table, supporting copy, and CTA text. A nonempty approved benefit must be
visible, legible text in the image; icons, phone forms, and step cards may reinforce it but never replace it. Do not reserve blank text boxes for
later code rendering and do not ask the platform to typeset, patch, or overlay business copy. The frozen
`brief.prime_composition.mode` decides the Prime boundary: `deterministic` produces an unbranded base for the later official overlay;
`model_integrated` produces the complete final image with the QR-free official template from Input 2. Never infer this choice from
market name, a previous task, or a template filename.

Aim for 900-1800 characters plus the approved copy. Dense repayment tables or App UI replacement may use up to about 2800 characters;
the hard limit is 3600. State each constraint once.

## Fixed sections

Replace every placeholder. Send exactly these sections in this order:

```text
TASK
{prime_task_instruction} {canvas_lock} ad image. Redesign the source identity while preserving the approved business mechanism.

INPUT ROLES
{input_roles}

LOCKED DESIGN DNA
{design_dna_invariants}

EDITABLE LAYOUT
{layout_plan_for_this_size}

APPROVED COPY
Render every approved string and frozen repayment row exactly once: {approved_copy_and_table}

PRIME SUPPORT
Input 2 is the current-size official Prime visual context. {prime_support_instruction}

ACCEPTANCE
All approved copy and content groups are present and legible; the requested canvas is preserved; DesignDNA remains recognizable; every
declared identity anchor remains the same person, product, or core object across delivery sizes and is never replaced; no competitor
identity, extra claim, duplicate copy, story frame, phone screenshot, long poster, scrolling page, 9:16, or 9:19 canvas is present.
{prime_acceptance_instruction}
```

The canvas locks are:

- `1080x1080 locked 1:1 square`;
- `1200x628 locked 1.91:1 landscape`;
- `800x1000 locked 4:5 portrait, only 1.25 times as high as wide`.

## Input roles

Input 1 is always the downloaded candidate source and is used only for business structure, reading order, and permitted visual
anchors. It is never the editable square base for landscape or portrait.

Input 2 is always the current-size, current-revision official Prime visual context. In `deterministic` mode it is a rendered visual
context for a later overlay. In `model_integrated` mode it is the full frozen, QR-free template selected by
`brief.prime_composition.template_family_id`; use only the matching current-size attachment in
`brief.prime_composition.template_sources`.

Input 3 exists only when `creative_contract.app_ui_replacement.selected=true`: it is the selected first-party App UI for visible
phone-screen content only. Preserve the phone, hand, perspective, reflections, lighting, occlusion, and scene; remove competitor UI
identity and do not draw the selected UI outside the screen.

Input 4 is the selected primary image for DesignDNA/identity consistency only. When `design_dna.subject_system` declares a person,
product, or core-object identity anchor, Input 4 is required for every later delivery size: keep the same declared identity anchor and
do not replace that person, product, or object. It must not donate layout, crop, copy placement, Prime pixels, or canvas proportions.
Input 4 may be omitted only for an unanchored `family_consistent` direction. This is an identity dependency, never a square-image
layout dependency.

For bounded visual rework, Input 1 is the failed same-size generated base in `deterministic` mode, or the failed same-size
model-integrated image in `model_integrated` mode. Input 2 remains current-size Prime context. Keep the same DesignDNA, approved copy,
and business facts and edit only the failed acceptance targets.

## Compilation

Compile the prompt from `creative_intent`, `design_dna`, the current `layout_plan`, approved copy, and semantic Input roles. Do not paste
the JSON or field names into the prompt. For every declared identity anchor, compile one explicit sentence that it remains the same
person, product, or core object in every size and must not be replaced. For `deterministic`, set `{prime_task_instruction}` to `Create one unbranded`, explain that
Input 2 is context only, keep future component areas clear, and state that no Prime pixel is drawn. For `model_integrated`, set it to
`Create one model-integrated image using the QR-free full official Prime template from Input 2`, require the visible official text,
logo, color, and approximate placement to remain faithful to Input 2, prohibit inventing any QR or additional official component, and
keep business content clear of the template areas. The backend will not add a second overlay in this mode. If
`prime_support.background_polarity=adaptive`, describe sufficient visual separation from Input 2 without inventing a fixed light or
dark background. If a structured polarity is supplied, express that polarity once.

Run `validate_copy_snapshot.py --require-prime-guard --require-redesign-guard --require-identity-guard --require-concise-prompt --explain` before the model call.
Repair only the reported rule. The validator checks approved financial values, this section contract, prompt length, and exclusion of
workflow protocol from model-facing text.
