# Creative Intent Contract

This is the canonical planning contract for candidate Variants. The Planner writes lifecycle fields at the `variant-put` top level and
visual fields inside each Variant brief; downstream workers consume them without reconstructing intent from prose.

## Candidate envelope

Each Order Item has four or five candidate Variants named `C01` through `C05`. Every candidate starts with:

```json
{
  "candidate_state": "candidate",
  "primary_size": "1200x628",
  "selection_rank": null,
  "creative_contract": {
    "contract_version": 2,
    "creative_intent": {},
    "design_dna": {},
    "layout_plans": {}
  }
}
```

`primary_size` is selected from the frozen delivery sizes for that concept. It is not always square:

- choose `1200x628` for horizontal comparison, wide environment, or left-right product/benefit relationships;
- choose `800x1000` for a vertical person, phone, stacked journey, or strong top-to-bottom reading order;
- choose `1080x1080` for balanced, radial, modular, or aspect-neutral concepts.

Only `primary_size` is generated while `candidate_state=candidate`. Candidate selection atomically promotes exactly three candidates
to `selected` with ranks 1 through 3 and moves the rest to `reserve`. Reserve candidates and their main images remain traceable but do
not participate in delivery aggregation. A selected candidate reuses its completed main image and expands to the full frozen delivery
size set.

## CreativeIntent

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

- `consistency_mode`: `family_consistent` by default, or `strict_identity` when the same person/product/object identity is essential;
- `subject_system`: subject type, identity anchor, pose/action family, relative scale, and subject-background relationship;
- `visual_system`: palette roles, contrast character, material, lighting, depth, image medium, and texture budget;
- `typography_system`: hierarchy, weight character, alignment behavior, line-count intent, and numeric/table treatment. GPT Image renders
  the approved text; this field does not authorize platform code-rendered typography;
- `motif_system`: shapes, icons, separators, cards, and recurring decorative language;
- `spatial_signature`: stable reading order and relationships among headline, benefit, subject, amount/table, CTA, and Prime support;
- `invariants`: features that must remain recognizably the same in every size;
- `adaptable_features`: features that may reflow, crop, simplify, or change scale without becoming another concept.

For `family_consistent`, each size can be generated independently from the same source reference and DesignDNA. A selected primary
image is an optional consistency reference, never a layout source or availability prerequisite. For `strict_identity`, use an explicit
approved identity anchor or selected-primary reference; that identity dependency must be declared and must never be disguised as a
hard dependency on the square delivery image.

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

## Candidate comparison

Every candidate must differ in hypothesis and at least two high-salience DesignDNA dimensions. Before main-image generation, record
an adaptability forecast for all three sizes. After at least three usable main-size Prime images exist, the independent candidate-selection
review scores each image on approved-copy readability, visual appeal, hypothesis clarity, differentiation, Prime integration, and
three-size adaptability. The reviewer selects exactly three; the Planner and producer must not update candidates one by one to mimic
an atomic selection. The platform may reject terminally unusable candidates and continue with the remaining candidates; fewer than three
usable candidates cannot enter comparison.
