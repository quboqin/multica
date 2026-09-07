# Creative Intent Contract

This is the canonical planning contract for candidate Variants. The Planner writes lifecycle fields at the `variant-put` top level and
visual fields inside each Variant brief; downstream workers consume them without reconstructing intent from prose.

## Candidate envelope

Read N from frozen `input_snapshot.target_variant_count` and K from `candidate_count` (N+2). Historical snapshots without these fields
use N=3 and K=5. New copy-library orders plan exactly K candidates named `C01` through `C{K:02d}`; N=10 uses C01-C12.
Material orders retain four or five candidates and three selected directions. Every candidate starts with:

```json
{
  "candidate_state": "candidate",
  "primary_size": "1080x1080",
  "selection_rank": null,
  "creative_contract": {
    "contract_version": 2,
    "creative_intent": {},
    "design_dna": {},
    "layout_plans": {}
  }
}
```

All candidate previews use `primary_size=1080x1080`. This makes the candidate comparisons consistent; their
`layout_plans` must still forecast native landscape and portrait reflow before selection.

Only `primary_size` is generated while `candidate_state=candidate`. Candidate selection atomically promotes exactly N candidates
to `selected` with ranks 1 through N and places up to two remaining candidates in `reserve` with ranks N+1 and N+2. Reserve candidates and their main images remain traceable but do
not participate in delivery aggregation. A selected candidate reuses its completed main image and expands to the full frozen delivery
size set.

## CreativeIntent

For `source_kind=copy_library`, there is no source reference or source analysis. Derive the concept from the frozen selected copy and
optional visual direction. Empty slots stay absent; an empty repayment list creates no repayment module, regardless of creative type.
With `visual_only=true`, explore visual hypotheses without inventing business claims. Prime remains required. Input roles list actual
assets only, starting with current-size Prime context and adding the selected-primary reference when identity continuity requires it.

`creative_intent` contains:

- `hypothesis_id`, `audience_tension`, `hook`, `message_mechanism`, `desired_response`, and `emotional_tone`;
- `input_roles`: ordered semantic roles for the source reference, per-size Prime context, optional approved App UI, and optional
  selected-primary consistency reference;
- `locked_set`: approved copy, financial facts, required content groups, product truth, legal truth, and identity anchors that cannot
  change;
- `editable_set`: scene, subject treatment, composition, crop, spacing, material, lighting, decorative system, and native per-size
  reflow decisions that may change;
- `change_budget`: the permitted degree of redesign and the maximum number of high-salience changes during a repair;
- `priorities`: ordered goals used to resolve conflicts;
- `acceptance_checks`: observable checks, each with a stable ID and a pass condition.

Input roles are permissions, not just labels. An input may be `editable`, `structure_reference`, `visual_context`, or
`identity_reference`. The same input cannot be both editable and locked. The source reference never authorizes reuse of competitor
identity, copy, brand, UI, QR, legal footer, face, clothing, gesture, props, or background.

## DesignDNA

`design_dna` is the cross-size identity, not a square raster:

- `consistency_mode`: use `strict_identity` whenever the selected direction contains a recurring person, product, or distinctive central object; `family_consistent` is only for abstract or unanchored directions;
- `subject_system`: subject type, explicit identity anchor, pose/action family, relative scale, and subject-background relationship. A recurring person, product, or distinctive central object must have one declared anchor;
- `visual_system`: palette roles, contrast character, material, lighting, depth, image medium, and texture budget;
- `typography_system`: hierarchy, weight character, alignment behavior, line-count intent, and numeric/table treatment. GPT Image renders
  the approved text; this field does not authorize platform code-rendered typography;
- `motif_system`: shapes, icons, separators, cards, and recurring decorative language;
- `spatial_signature`: stable reading order and relationships among headline, benefit, subject, amount/table, CTA, and Prime support;
- `invariants`: features that must remain recognizably the same in every size;
- `adaptable_features`: features that may reflow, crop, simplify, or change scale without becoming another concept.

For `family_consistent`, each size can be generated independently from the same source reference and DesignDNA. A selected primary
image is an optional consistency reference, never a layout source or availability prerequisite. For `strict_identity`, use an explicit
approved identity anchor and the selected-primary reference for every later size; the same declared person, product, or central object
must not be replaced. That identity dependency must be declared and must never be disguised as a hard dependency on the square
delivery image.

## LayoutPlan

`layout_plans` has one entry per frozen delivery size. Each entry contains:

- `size_key`, `aspect_role`, `is_primary`, `reading_order`, `content_groups`, and `native_reflow`;
- `subject_frame`, `copy_density`, `table_strategy`, `crop_tolerance`, and `fallback_simplifications`;
- `prime_support`: `strategy`, component-scoped support regions, texture budget, and `background_polarity`. Use `adaptive` when the
  approved family has not yet been selected; never assume that official text always needs a light background;
- `acceptance_checks`: size-specific observable conditions, including all required copy and every frozen repayment row;
- `adaptability_risks`: anticipated landscape, portrait, crop, density, App UI, and Prime-support failure modes.

LayoutPlan describes semantic relationships. Exact protected geometry stays in the frozen `prime_layout_contract` and deterministic
composer evidence; it is not copied into the model prompt.

When frozen approved copy contains a nonempty `benefit`, every LayoutPlan must include it as a readable text content group and an
acceptance check. Icons, phones, and step cards may reinforce the benefit but cannot replace the text.

## Candidate comparison

Every candidate must differ in hypothesis and at least two high-salience DesignDNA dimensions. Before main-image generation, record
an adaptability forecast for all three sizes. After all planned candidates settle and at least N usable main-size Prime images exist, the independent candidate-selection
review scores each image on approved-copy readability, visual appeal, hypothesis clarity, differentiation, Prime integration, and
three-size adaptability. The reviewer selects exactly N; the Planner and producer must not update candidates one by one to mimic
an atomic selection. The platform may reject terminally unusable candidates and continue with the remaining candidates; fewer than N
usable candidates cannot enter comparison.
