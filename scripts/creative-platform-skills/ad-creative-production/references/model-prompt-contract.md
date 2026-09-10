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
{prime_task_instruction} {canvas_lock} ad image. {source_or_original_creation_instruction}

INPUT ROLES
{input_roles}

LOCKED DESIGN DNA
{design_dna_invariants}

EDITABLE LAYOUT
{layout_plan_for_this_size}

APPROVED COPY
Render every approved string and frozen repayment row exactly once: {approved_copy_and_table}

PRIME SUPPORT
{prime_input_label} is the current-size official Prime visual context. {prime_support_instruction}
When an approved headline is present and the official logo is above the content, place the entire headline group below the official logo with a clear visible gap. If space is tight, reduce or reposition decorative elements first; never move business text into the logo area.
{component_background_support}

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

The logo-clearance sentence in PRIME SUPPORT is fixed template content, not an optional summary. It applies to the full headline and benefit group, including the top of every letter. In deterministic mode, omitting logo pixels from the generated base does not make that reserved space available. Resolve any LayoutPlan wording such as "upper headline" to the business area below the actual logo. Keep the background full-bleed; simplify, shrink or move decorative motifs before compressing the gap or the approved text. For visual-only copy, the conditional sentence must not cause a headline to be invented; for a template without an upper logo, preserve the actual component placement.

## Input roles

### Material orders

For material orders, Input 1 is the downloaded candidate source and is used only for business structure, reading order, and permitted visual
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

### Copy library orders

For `source_kind=copy_library`, no candidate source is supplied. Input 1 is current-size official Prime context; subsequent inputs are
only the declared primary-image identity references. Number input labels by their actual order and use those same labels throughout
TASK, INPUT ROLES, and PRIME SUPPORT. Do not synthesize a source image or copy competitor-structure instructions into this branch.
Set the creation instruction to create an original visual identity from the selected copy. APPROVED COPY includes only selected strings
and the selected repayment columns in their frozen order; explicitly prohibit filling omitted slots or columns. The sole display source is repayment_plan_selections.values plus repayment_plan_labels filtered by repayment_plan_columns. Raw repayment_plan_entries are audit-only and cannot donate missing columns, numbers, or labels. Currency prefixes come from the frozen display values; never hardcode Rp for another market. With `visual_only=true`, state that no business copy, claims, numbers,
repayment table, or CTA may be added. Official Prime content keeps its existing composition contract.

Compile the copy-library LayoutPlan as natural-language reading order and content grouping. For dense copy, reduce decoration and
adjust subject emphasis; describe native landscape or portrait reflow instead of carrying over a square layout. Do not add coordinates,
percentage boxes, font-size thresholds, or instructions to shrink the entire image. Preserve all selected copy. Business content stays
clear of the real Prime context while the background remains full-bleed; the platform overlays the official template unchanged.

### Bounded visual rework

For bounded visual rework, Input 1 is the failed same-size generated base in `deterministic` mode, or the failed same-size
model-integrated image in `model_integrated` mode. Input 2 remains current-size Prime context. Keep the same DesignDNA, approved copy,
and business facts and edit only the failed acceptance targets. For local reflow, describe one concrete overlap and where the complete module should move in relation to the visible template; preserve the rest. For contrast-only repair, edit only the quiet surrounding background according to the actual template color. Never change template family between this reference and final composition.

Only the explicitly assigned background_expansion fallback uses references/background-expansion.md instead of the full-design TASK: extend the surrounding background without generating or editing text. It is not a default generation method. Masks guide the model; the protected body is restored deterministically after its response.

## Compilation

Compile the prompt from `creative_intent`, `design_dna`, the current `layout_plan`, approved copy, and semantic Input roles. Do not paste
the JSON or field names into the prompt. For material orders, set the creation instruction to `Redesign the source identity while
preserving the approved business mechanism.` For copy-library orders, use the original-creation instruction defined above.
For every declared identity anchor, compile one explicit sentence that it remains the same
person, product, or core object in every size and must not be replaced. For `deterministic`, set `{prime_task_instruction}` to `Create one unbranded`, explain that
the actual Prime input is context only, keep future component areas clear, and state that no Prime pixel is drawn. For `model_integrated`, set it to
`Create one model-integrated image using the QR-free full official Prime template from {prime_input_label}`, require the visible official text,
logo, color, and approximate placement to remain faithful to that Prime input, prohibit inventing any QR or additional official component, and
keep business content clear of the template areas. The backend will not add a second overlay in this mode. If
`prime_support.background_polarity=adaptive`, describe sufficient visual separation from Input 2 without inventing a fixed light or
dark background. If a structured polarity is supplied, express that polarity once.

Resolve `{component_background_support}` from the current-size Prime image and LayoutPlan before the first generation. Describe
the upper logo and lower fine text/store components separately: actual lettering color, the contrasting scene surface behind it,
and distracting texture to remove. Dark lettering needs lighter support; light lettering needs darker support; colored marks also
need separation from similar background hues and tones. When the components differ, resolve support locally. A transparent template
or the viewer's display background is not evidence that the whole ad should be light or dark. Do not invent measured contrast.

Use a compact visual instruction, adapted to the actual template, for example: "Behind the dark green upper logo, keep a pale,
muted sky with no foliage. Behind the dark lower store lettering, use a softly lit, even tabletop with no glare or grain crossing
the letters. Blend these quiet surfaces naturally into the scene; keep the smallest official lettering easy to distinguish at final
size." This is an example for dark lettering, not a universal palette. For light lettering, use a darker quiet surface instead.
Avoid highlights, object edges and light/dark transitions through fine lettering; place gradients outside each component and keep
their support visually even. Preserve full-bleed scene continuity rather than adding a generic strip, sticker, outline, shadow or
opaque box. Do not recolor or redraw official components to fix the background. In deterministic mode, generate only the supporting
background and business content; the actual Prime pixels still come from the unchanged overlay.

Keep this component-specific instruction concise (usually two sentences). Replace vague or duplicate Prime/background descriptions;
trim decorative prose before approved copy if needed to stay within the existing prompt limit. Treat readability and no obstruction
as separate acceptance goals. This guidance adds no numeric target or exact-phrase generation gate and does not replace final QC.

Run `validate_copy_snapshot.py --require-prime-guard --require-redesign-guard --require-identity-guard --require-concise-prompt --explain` before the model call.
Repair only the reported rule. The validator checks approved financial values, this section contract, prompt length, and exclusion of
workflow protocol from model-facing text.

The Logo placement, spacing, and decoration priority are visual guidance. Equivalent natural wording is allowed; these three phrases are not generation validation gates.
