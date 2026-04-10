> **ARCHIVED** — All ML-1 through ML-5 stories are complete as of v0.4.7. This document is preserved for historical context. See `docs/maintenance.md` and `docs/configuration.md` for current lens mode documentation.

---

# Epic Backlog: Mental Model Lens Runs (ML Series)

## Context

The dream cycle currently supports `self_consistency_runs: N`, which runs the same batch prompt N times in parallel and uses majority voting to filter noise. Analysis of the quality ledger shows 3x-sonnet delivers no meaningful confidence gain over 1x-sonnet (~65% vs ~65%) at 2.5x the cost. The voting mechanism rewards redundancy, not insight.

**This backlog replaces the voting mechanism with lens mode**: N parallel runs where each uses a focused mental model lens instead of an identical prompt. The reconciliation pass (Opus) synthesizes across lenses rather than voting away disagreements — complementary findings replace redundant ones.

**Theoretical grounding:** Charlie Munger's "latticework of mental models" holds that complex problems cannot be understood through a single analytical lens. His practical "Rule of 3 Models" applies directly to lens selection: choose one psychological lens (e.g., confirmation bias, survivorship bias), one economic or causal lens (e.g., opportunity cost, feedback loops), and one structural or mathematical lens (e.g., first principles, probabilistic thinking). Triangulation across these three categories surfaces blind spots that any single framework would miss — and maps directly to how dream cycle lenses should be configured per project.

---

## Design Decisions (Resolved)

### D1: Voting Mechanism — Replace, Don't Patch

`VoteVisions()` in `voting.go` is built for same-job agreement. It cannot meaningfully vote across lenses by design. **Recommendation: Lens mode replaces voting mode.** Both options co-exist in config but are mutually exclusive. `self_consistency_runs > 1` continues to work for users who want redundancy-based voting; `lens_mode: [...]` activates lens runs instead.

### D2: Confidence Scoring — Cross-Lens Agreement Is a Stronger Signal

The current 20% agreement weight scores a vision higher when multiple identical runs agreed on it. In lens mode, cross-lens agreement (two different mental model lenses independently flagging the same mote) is a meaningfully stronger signal — multiple analytical frameworks converging on the same problem. **Recommendation: Introduce a `cross_lens_agreement` confidence component that replaces the `agreement` component in lens mode.** Single-lens visions retain their base confidence; cross-lens matches receive a boost.

### D3: Structural Tasks — Dedicated Baseline Lens

Merge detection, action extraction, and compression currently run in every batch prompt. These are housekeeping tasks with no mental model angle. **Recommendation: Create a `structural` lens as the first lens in every lens run.** When `lens_mode` is configured, the structural lens always runs regardless of which cognitive lenses are active. This avoids duplicating housekeeping instructions into every cognitive lens prompt.

### D4: Config Shape

```yaml
dream:
  batching:
    lens_mode:
      enabled: true
      lenses: ["structural", "survivorship_bias", "feedback_loops", "inversion"]
    self_consistency_runs: 1  # ignored when lens_mode.enabled is true
```

Lens names are string identifiers mapped to prompt variants in code. This allows the user to configure which lenses run per project. The structural lens does not count against the cognitive lens count — if 3 cognitive lenses are configured, 4 runs occur per batch.

### D5: Backward Compatibility

Existing `self_consistency_runs` config continues to work. When `lens_mode.enabled: true`, `self_consistency_runs` is silently ignored and the quality ledger logs the config as `"Nx-lens"` (e.g., `"3x-lens"`).

---

## Questions and Decisions Required from Product Owner

> **Q1 — Lens count flexibility**: Should lens mode support any number of lenses (2, 3, 5), or enforce exactly 3 for parity with the existing 3x billing model? **Recommendation: Flexible** — list-based config is more powerful and no harder to implement. Projects with different knowledge profiles benefit from different lens counts.

> **A1** - Agree with recommendation. Lens count should be flexible and configurable. The lens count should default at three on initial configuration.

> **Q2 — Structural lens scope**: Should merge detection and action extraction live in the structural lens only, or should each cognitive lens also extract actions from findings in its own domain? E.g., should the survivorship bias lens also extract actions like "Add failure case data for X"? **Recommendation: Structural lens only** — cognitive lenses output cognitive findings; action extraction from those findings can be a downstream step.

> **A2** - Agree with recommendation. Cognitive lenses output cognitive findings

> **Q3 — Lens applicability when nothing found**: If a lens finds nothing applicable in a batch (e.g., no feedback loops detected), should it return an empty result set or a brief "no findings" explanation for the reconciliation context? **Recommendation: Empty set** — reconciliation tokens are wasted on "nothing found" prose.

> **A3** - Agree with recommendation. Retuen an empty result.

> **Q4 — Which cognitive lenses for v1?** Applying Munger's Rule of 3, the recommended v1 set is: one psychological lens (**survivorship bias**, already implemented in prompts), one causal/economic lens (**feedback loops**, already implemented in prompts), and one critical thinking lens (**inversion**, new but well-defined). This produces a genuinely triangulated analysis at no additional cost versus 3x-sonnet.

> **A4** - Agree with recommendation. Implement inversion.

> **Q5 — New mental models in scope for this series?** Inventory review identifies four models ready for full stories beyond the original plan (Inversion, First Principles, Probabilistic Thinking, Confirmation Bias upgrade). **Recommendation: Include all four** — they are analytically distinct and well-grounded in the inventory. See Epic ML-2 for readiness assessment.

> **A5** - Agree with recommendation. Include all four

> **Q6 — Quality ledger migration**: The `--quality` display currently shows `VotingConfig` as `"3x-sonnet"`. Lens mode needs a new label format (e.g., `"3x-lens[surv,floop,inv]"`). **Recommendation: No migration** — old rows display as-is; new rows use the new label format.

> **A6** - Agree with recommendation. No migration.

---

## Mental Model Inventory: Lens Readiness

| Model | Source | Lens Candidate | Current State | Readiness |
|-------|--------|---------------|---------------|-----------|
| **Survivorship Bias** | Inventory #11 | Yes | Prompt guard in `prompt.go:155-174`; recon synthesis `prompt.go:201-206` | **Ready** — extract and deepen existing prompt |
| **Feedback Loops** | Inventory #14 | Yes | Prompt guard in `prompt.go:109-129`; recon synthesis `prompt.go:208-213` | **Ready** — extract and deepen existing prompt |
| **Inversion** | Inventory #2 | Yes | Not implemented | **Ready for stories** — well-defined; "what assumptions, if wrong, are most damaging?" is a clear prompt frame |
| **First Principles** | Inventory #1 | Yes | Not implemented | **Ready for stories** — "which motes are derived conclusions that could be decomposed to fundamentals?" distinct from Occam's Razor |
| **Probabilistic Thinking / Base Rates** | Inventory #20 | Yes | Not implemented | **Ready for stories** — "which lessons claim certainty without base-rate data?" well-defined detection pattern |
| **Confirmation Bias** | Inventory #10 | Yes | ML-6 stub only | **Upgrade to full story** — inventory provides concrete detection heuristics |
| **Opportunity Cost** | Inventory #6 | Yes | Stats command only; no LLM lens | **Needs design** — "what knowledge gaps exist?" needs prompt definition; `knowledge_gap` vision type undefined |
| **Occam's Razor** | Inventory #4 | Yes | Doctor command (deterministic thresholds) | **Needs design** — LLM lens differs from deterministic; overlap risk with structural lens merge detection |
| **Second-Order Effects** | Inventory #3, #15 | Partial | Dry-run preview in `--review`; recon check `prompt.go:215-219` | **Deferred** — deterministic scoring preview covers mechanical cascades; unclear additive value |
| **Incentives** | Inventory #9 | Stub | Not implemented | **Future** — high conceptual value; hard to operationalize without clearer detection heuristic |
| **Anchoring Bias** | Inventory #12 | Stub | Not implemented | **Future** — early motes anchoring later ones; needs scoping |
| **Pareto Principle** | Inventory #8 | Stub | Not implemented | **Future** — graph analysis angle (which 20% of motes generate 80% of retrieval value); needs scoping |
| **Hanlon's Razor** | Inventory #13 | Stub | Not implemented | **Future** — applies specifically to incident/post-mortem motes; too narrow for general lens |
| **Dominant Mote Awareness** | MM-1 | No | PreScanner `prescanner.go:802-862` — deterministic | **Not a lens candidate** — deterministic detection more reliable |
| **Sunk Cost** | Inventory #7, MM-5 | No | PreScanner + session-end — deterministic | **Not a lens candidate** — deterministic handles this |
| **Stocks and Flows** | Inventory #16, MM-2 | No | Stats + Doctor — fully implemented | **Not a lens candidate** — already implemented |
| **Compounding** | Inventory #21 | No | Covered by feedback loops (reinforcing loops = compounding) | **Overlap** — skip |
| **Loss Aversion** | Inventory #23 | No | Overlaps with survivorship + confirmation | **Overlap** — skip |
| **Hyperbolic Discounting** | Inventory #25 | No | Covered by sunk cost (deterministic) | **Overlap** — skip |
| **Network Effects** | Inventory #17 | No | Not applicable to knowledge graph maintenance | **Skip** |
| **Creative Destruction** | Inventory #19 | No | Not applicable | **Skip** |
| **Social Proof** | Inventory #24 | No | Not applicable | **Skip** |
| **Margin of Safety** | Inventory #18 | No | Too domain-specific | **Skip** |
| **Circle of Competence** | Inventory #5 | No | Too meta; hard to operationalize in prompt | **Skip** |
| **Regression to the Mean** | Inventory #22 | No | Too narrow; requires quantitative data | **Skip** |

**Structural lens** (always-on baseline): merge detection, action extraction, compression, contradiction detection, tag refinement. Extracted from current all-in-one `batchPromptTmpl`. Not a cognitive model — infrastructure.

---

## Epic ML-1: Lens Mode Architecture

**Design Note — Complementarity Over Redundancy:**
Knowledge graph maintenance benefits more from multiple analytical perspectives examining the same content than from the same perspective repeated for noise reduction. This epic establishes the infrastructure that lets lens runs operate in parallel and feed their distinct findings to the reconciliation stage.

**Dependencies:** None. This is the foundation for all subsequent ML epics.

---

### Story ML-1.1 — Lens Mode Config and Validation

**As a** user configuring a dream cycle
**I want to** specify a list of named lenses instead of a repetition count
**So that** each parallel run analyzes motes through a different mental model

**Acceptance Criteria:**
```gherkin
Given dream.batching.lens_mode.enabled is true
And dream.batching.lens_mode.lenses contains ["structural", "survivorship_bias"]
When the dream cycle initializes
Then it schedules 2 parallel runs per batch, one per named lens
And self_consistency_runs is ignored

Given dream.batching.lens_mode.enabled is false
When the dream cycle initializes
Then it uses self_consistency_runs behavior unchanged

Given lens_mode.lenses contains an unrecognized name
When the dream cycle initializes
Then it returns a config validation error naming the unknown lens
```

**Critical file:** `internal/core/config.go`
**Status: Ready for user stories**

---

### Story ML-1.2 — Lens-Aware Prompt Builder

**As a** batch orchestrator
**I want to** receive a lens name when building a batch prompt
**So that** each parallel run gets a focused prompt variant instead of the all-in-one prompt

**Acceptance Criteria:**
```gherkin
Given a batch and a lens name "survivorship_bias"
When BuildBatchPrompt(batch, lucidLog, lens) is called
Then it returns the survivorship bias focused prompt variant
And the returned prompt does not contain feedback loop or merge detection sections

Given lens name is "structural"
When BuildBatchPrompt is called
Then it returns the structural prompt (merge, compress, action extraction, contradiction)

Given lens name is "" (empty, legacy mode)
When BuildBatchPrompt is called
Then it returns the existing all-in-one prompt unchanged
```

**Critical file:** `internal/dream/prompt.go` — `BuildBatchPrompt()`
**Status: Ready for user stories**

---

### Story ML-1.3 — Tagged Union Merge (Replace VoteVisions in Lens Mode)

**As a** batch orchestrator in lens mode
**I want to** collect all lens results into a tagged union rather than voting them
**So that** unique findings from each lens are preserved rather than filtered out

**Acceptance Criteria:**
```gherkin
Given 3 lens runs complete for a batch
And survivorship_bias lens produces 2 visions
And feedback_loops lens produces 1 vision
And structural lens produces 3 visions
When MergeLensResults() is called
Then all 6 visions are preserved in the result set
And each vision carries a LensSource field identifying which lens produced it

Given survivorship_bias lens and feedback_loops lens both produce a vision for the same source mote
When MergeLensResults() is called
Then both visions are preserved
And a cross_lens_match flag is set on both
And the CrossLensAgreement field is populated with the lens names that agreed
```

**Critical files:** `internal/dream/voting.go` (new function alongside `VoteVisions`), `internal/dream/types.go` (Vision struct extension)
**Status: Ready for user stories**

---

### Story ML-1.4 — Quality Ledger Lens Label

**As a** user reviewing dream quality history
**I want to** see which lenses were used for a given cycle
**So that** I can compare lens configurations over time

**Acceptance Criteria:**
```gherkin
Given lens_mode ran with lenses ["structural", "survivorship_bias", "feedback_loops"]
When the quality entry is written
Then VotingConfig label is "3x-lens[struct,surv,floop]" (abbreviated lens names)

Given legacy self_consistency_runs: 3
When the quality entry is written
Then VotingConfig label is "3x-sonnet" (unchanged)

Given mote dream --quality is run
Then lens-mode rows display their lens config in the Config column
```

**Critical file:** `internal/dream/quality.go` — `VotingConfigLabel()`
**Status: Ready for user stories**

---

## Epic ML-2: Lens Prompt Library

**Design Note — Focused Prompts, Sharper Findings:**
The current all-in-one batch prompt asks Claude to simultaneously detect survivorship bias, feedback loops, merge candidates, contradictions, and compression opportunities. Each mental model lens receives a prompt scoped to its single analytical frame, reducing cognitive load on the model and producing higher-signal findings per lens. Lens prompts do not duplicate structural analysis (merge, compress, extract) — that lives in the structural lens only.

**Dependencies:** ML-1 (ML-1.2 — lens-aware prompt builder must exist first)

---

### Story ML-2.1 — Structural Lens Prompt

**As a** dream cycle running lens mode
**I want a** structural lens prompt that handles housekeeping analysis
**So that** merge detection, action extraction, compression, and contradiction detection run reliably regardless of which cognitive lenses are active

**Scope:** Extract from existing `batchPromptTmpl` in `prompt.go`: merge review, action extraction, compression, contradiction detection, tag refinement. Remove all cognitive model guards. This is a refactoring of existing content, not new prompt writing.

**Critical file:** `internal/dream/prompt.go`
**Status: Ready for user stories** — content exists, extraction is mechanical

---

### Story ML-2.2 — Survivorship Bias Lens Prompt

**As a** dream cycle running lens mode
**I want a** survivorship bias focused prompt variant
**So that** one dedicated run examines all motes for missing failure data, non-survivor evidence, and base-rate omissions

**Scope:** Deepen and focus the existing survivorship bias guard from `prompt.go:155-174`. Remove all non-survivorship sections. The lens should actively look for what is *absent* — failure cases, counterfactuals, base rates — not just what is present. It should be more aggressive about flagging suspicion than the current embedded guard, which is diluted by running alongside other detection tasks.

Key detection patterns (from inventory):
- "Successful X always Y" without data on unsuccessful X
- Lessons derived from a single positive outcome
- Advice citing only survivors (successful founders, winning projects)
- Missing counterfactual: no evidence on those who tried the same approach and failed

**Critical file:** `internal/dream/prompt.go`
**Status: Ready for user stories** — existing prompt is a strong starting point

---

### Story ML-2.3 — Feedback Loop Lens Prompt

**As a** dream cycle running lens mode
**I want a** feedback loop focused prompt variant
**So that** one dedicated run traces causal chains between motes and identifies reinforcing and balancing cycles

**Scope:** Deepen and focus the existing feedback loop detection from `prompt.go:109-129`. The dedicated lens should more aggressively identify partial chains (A→B, B→C exists but C→A is undocumented) and propose the completing link. Leverage point identification — motes appearing in 2+ loops — should be prominent, as these are the highest-intervention-value nodes in the graph.

Key detection patterns (from inventory):
- Reinforcing loops: A amplifies B which amplifies A (virtuous or vicious cycles)
- Balancing loops: A reduces B which reduces pressure on A (stabilizing or oscillating)
- Delayed causation: A causes B but with significant lag that obscures the relationship

**Critical file:** `internal/dream/prompt.go`
**Status: Ready for user stories** — existing prompt is a strong starting point

---

### Story ML-2.4 — Inversion Lens Prompt

**As a** dream cycle running lens mode
**I want an** inversion focused prompt variant
**So that** one dedicated run surfaces fragile assumptions, overconfident claims, and knowledge that would be most damaging if wrong

**Scope:** New lens, not currently implemented. The inversion model (Munger: "Invert, always invert") approaches a problem backwards — instead of asking "what do we know?", it asks "what assumptions in our current knowledge, if wrong, would cause the most harm?" For a knowledge graph, this means:

- Identify motes containing strong claims presented as settled that have not been tested against failure conditions
- Flag lessons that are stated as universal principles but rest on limited evidence
- Surface decisions documented as successful that lack documented alternatives or exit criteria
- Identify knowledge clusters where all motes point toward the same conclusion (a failure to invert)

This lens is analytically distinct from survivorship bias (which looks for missing failure data) — inversion looks for untested assumptions regardless of whether failures are documented.

**Output vision types:** `survivorship_risk` (reusing for fragile-assumption warnings), `link_suggestion` with `link_type: "assumption_risk"` (new), `add_signal` for knowledge clusters lacking opposing evidence.

**Decision resolved (Q7):** `assumption_risk` is a **new link type**. Survivorship risk = missing data; assumption risk = unvalidated extrapolation. Semantically distinct — using the same type would obscure the analytical difference in reconciliation.

**Output vision types:** `link_suggestion` with `link_type: "assumption_risk"`, `add_signal` for clusters lacking opposing evidence.

**Acceptance Criteria:**
```gherkin
Given a batch with motes containing strong universal claims
When the inversion lens analyzes the batch
Then it produces visions of type link_suggestion with link_type: "assumption_risk"
  for motes whose claims rest on untested assumptions
And it produces add_signal visions for knowledge clusters
  where all motes point toward the same conclusion
And it does not flag motes that document alternatives or exit criteria

Given a mote states a lesson as a universal principle
And the mote has no documented failure conditions or counterexamples
When the inversion lens analyzes it
Then a vision is produced identifying the untested assumption
And the vision rationale distinguishes assumption_risk from survivorship_risk

Given a mote documents both assumptions and explicit alternatives considered
When the inversion lens analyzes it
Then no assumption_risk vision is produced for that mote
```

**Status: Ready for stories**

---

### Story ML-2.5 — First Principles Lens Prompt

**As a** dream cycle running lens mode
**I want a** first principles focused prompt variant
**So that** one dedicated run identifies motes that are derived conclusions rather than grounded fundamentals, and surfaces opportunities to decompose compound concepts

**Scope:** New lens. First Principles thinking (Aristotle, Musk) challenges assumptions by reducing claims to their most fundamental, irreducible truths and then rebuilding. For a knowledge graph, this means:

- Identify motes that state conclusions derived from other concepts without documenting the reasoning chain
- Find compound motes that conflate two or three distinct concepts which could be decomposed into separate, more fundamental motes
- Surface motes that cite analogies ("we do X because Company Y does X") rather than grounding the reasoning in first principles
- Identify missing foundational motes — referenced concepts that are assumed to be understood but never defined in the graph

This is distinct from Occam's Razor (which simplifies) — First Principles decomposes and grounds. The structural lens handles merge candidates (too many motes); First Principles handles decomposition candidates (motes that are too compound or poorly grounded).

**Decision resolved (Q8):** New `decompose_suggestion` vision type. Inverse of `merge_suggestion` — structural lens finds duplicates (merge); first principles finds over-compressed compounds (decompose). Clear boundary: structural = duplicate content, first principles = over-compressed semantics.

**Acceptance Criteria:**
```gherkin
Given a batch containing compound motes that conflate distinct concepts
When the first principles lens analyzes the batch
Then it produces visions of type decompose_suggestion for over-compressed motes
And each decompose_suggestion vision identifies the distinct concepts to separate
And it does not produce merge_suggestion visions (structural lens owns that)

Given a mote cites analogy-based reasoning ("we do X because Company Y does X")
When the first principles lens analyzes it
Then a decompose_suggestion or add_signal vision is produced
And the rationale notes missing first-principles grounding

Given a mote references a concept that is never defined elsewhere in the graph
When the first principles lens analyzes the batch
Then an add_signal vision is produced flagging the missing foundational mote

Given a mote contains only one well-grounded concept with documented reasoning chain
When the first principles lens analyzes it
Then no decompose_suggestion vision is produced for that mote
```

---

### Story ML-2.6 — Probabilistic Thinking / Base Rates Lens Prompt

**As a** dream cycle running lens mode
**I want a** probabilistic thinking focused prompt variant
**So that** one dedicated run flags lessons and decisions that claim certainty without empirical grounding or base-rate data

**Scope:** New lens. Probabilistic thinking (base-rate reasoning) corrects for overconfidence and the "inside view" — treating each situation as unique rather than anchoring to historical frequencies. For a knowledge graph, this means:

- Identify lessons stating universal outcomes ("this approach always works," "this pattern never fails") without supporting data on frequency or sample size
- Flag decisions that treated their outcome as more certain than the evidence warranted
- Surface motes that should reference base rates but don't (software project timelines, migration success rates, technology adoption curves)
- Identify knowledge clusters where all documented outcomes are binary (success/failure) with no probability distributions or confidence levels

This is distinct from survivorship bias (which focuses on missing failure examples) — probabilistic thinking focuses on miscalibrated confidence regardless of whether failures are visible.

**Output vision types:** `add_signal` for missing base-rate context, `link_suggestion` to connect overconfident claims to existing counter-evidence motes.

**Status: Ready for user stories** — detection patterns are well-defined

---

### Story ML-2.7 — Confirmation Bias Lens Prompt

**As a** dream cycle running lens mode
**I want a** confirmation bias focused prompt variant
**So that** one dedicated run detects when motes selectively cite confirming evidence while discounting or ignoring contradicting evidence

**Scope:** Upgrade from ML-6 stub. Confirmation bias (from inventory) is described as "one of the most pervasive cognitive biases in decision-making." For a knowledge graph, this means:

- Identify lessons that draw conclusions from evidence that all points in one direction without acknowledging contradictions
- Flag decision motes where alternatives were evaluated but only weaknesses of rejected options were documented (not weaknesses of the chosen option)
- Surface motes that reference external sources or examples only from those that agree with the conclusion
- Identify when a mote's stated lesson directly contradicts another mote that is not linked as a contradiction — the two should be connected so the tension is visible

This differs from survivorship bias (missing failure data) and inversion (untested assumptions) — confirmation bias specifically detects selective evidence marshaling.

**Output vision types:** `link_suggestion` with `link_type: "contradiction"` for unlinked contradicting motes, `add_signal` for one-sided evidence patterns.

**Status: Ready for user stories**

---

### Story ML-2.8 — Opportunity Cost Lens Prompt *(Needs Design)*

**As a** dream cycle running lens mode
**I want an** opportunity cost focused prompt variant
**So that** one dedicated run examines what knowledge is absent, underrepresented, or systematically avoided in the graph

**Scope:** Decisions or events documented with outcomes but not alternatives; topics appearing in many motes but lacking lessons; knowledge that would complement what exists but is missing.

**Decision resolved (Q9):** Lens flags absences for human review only. Uses `add_signal` with `knowledge_gap:` prefix in rationale. No net-new mote creation — reconciliation evaluates absence patterns from what is present.

**Note:** Implement after v1 lenses (ML-2.1–2.4, ML-2.7) are validated in production.

**Acceptance Criteria:**
```gherkin
Given a batch with motes documenting a decision
And the decision mote records the chosen option but no alternatives
When the opportunity cost lens analyzes the batch
Then an add_signal vision is produced noting the absence of alternatives
And the vision type is add_signal (not a new-mote creation request)
And the rationale contains "knowledge_gap: alternatives not documented"

Given a topic that appears across many motes but lacks associated lessons
When the opportunity cost lens identifies the pattern
Then an add_signal vision is produced for the most-related mote
  requesting a lesson be authored
And the rationale contains "knowledge_gap: topic lacks lesson motes"

Given a decision mote that documents both the chosen option and at least two alternatives considered
When the opportunity cost lens analyzes it
Then no opportunity cost vision is produced for that mote
```

---

### Story ML-2.9 — Occam's Razor Lens Prompt *(Needs Design)*

**As a** dream cycle running lens mode
**I want an** Occam's Razor focused prompt variant
**So that** one dedicated run looks for unnecessary complexity: over-linked motes, redundant concept splits, and abstraction layers that could be collapsed

**Scope:** Complements the deterministic complexity thresholds in `mote doctor` but operates semantically. Is this concept modeled at the right level of abstraction? Is this link chain longer than the underlying reasoning requires?

**Design resolved:** LLM lens adds semantic complexity detection that deterministic thresholds miss — specifically, link chains longer than the underlying reasoning requires, and abstraction layers that could be collapsed. Uses `decompose_suggestion` vision type (introduced in ML-2.5). Clear boundary: structural lens = duplicate content (merge); Occam's Razor lens = over-complex abstraction (decompose to simplify). Never duplicates doctor's deterministic findings.

**Note:** Implement after v1 lenses validated. Shares `decompose_suggestion` type with ML-2.5.

**Acceptance Criteria:**
```gherkin
Given a batch with a mote that is linked to 5+ other motes
And the link chain is longer than the underlying reasoning requires
When the Occam's Razor lens analyzes the batch
Then a decompose_suggestion vision is produced for the over-linked mote
And the vision rationale explains what abstraction could be collapsed

Given a mote that the deterministic doctor command already flagged for complexity
When the Occam's Razor lens analyzes the same batch
Then the lens produces no duplicate vision for the deterministic finding
And focuses only on semantic complexity not caught by deterministic thresholds

Given a mote with appropriate link complexity matching its conceptual scope
When the Occam's Razor lens analyzes it
Then no decompose_suggestion vision is produced for that mote

Given lens name is "occams_razor"
When BuildBatchPrompt is called
Then the returned prompt does not produce merge_suggestion visions (structural lens owns that)
```

---

### Story ML-2.10 — Second-Order Impact Lens Prompt *(Deferred)*

**⚠ Deferred — needs scoping:**
The existing second-order impact check in `prompt.go:215-219` and `dream --review` (dry-run scoring) already handle mechanical consequence of applying visions. It is unclear whether an LLM lens adds analytical value beyond what the deterministic scoring preview already provides. Do not build until a concrete use case distinguishes LLM-based cascade analysis from the existing dry-run preview.

---

## Epic ML-3: Reconciliation Enhancement for Lens Context

**Design Note — Opus as Lens Synthesizer:**
In lens mode, the reconciliation prompt receives a richer input: visions tagged by their source lens, cross-lens matches flagged, and lens-specific lucid log sections. Opus reconciliation should reason about which lenses agreed and what that means, rather than treating all visions as equivalent. A vision flagged by both survivorship bias and confirmation bias lenses for the same mote is a strong signal — two cognitively distinct analyses converged on the same knowledge problem.

**Dependencies:** ML-1.3 (tagged union must exist), ML-2 (lens prompts must exist)

---

### Story ML-3.1 — Update Reconciliation Prompt for Lens Context

**As a** reconciliation pass in lens mode
**I want to** receive lens-tagged visions and reason across lens perspectives
**So that** cross-lens agreement is surfaced as a higher-confidence signal and lens-specific findings are evaluated in their analytical context

**Acceptance Criteria:**
```gherkin
Given visions arrive tagged with source lens
When the reconciliation prompt is built
Then the prompt includes a lens context section listing which lenses ran
And visions with cross_lens_match set are grouped and highlighted
And the reconciliation instruction notes that cross-lens agreement warrants higher priority

Given only one lens flagged a mote
When reconciliation evaluates it
Then it evaluates the vision on its own merits without penalizing for lack of cross-lens agreement
```

**Critical file:** `internal/dream/prompt.go` — `reconPromptTmpl`
**Status: Ready for user stories** (after ML-1.3)

---

## Epic ML-4: Lens Confidence Scoring

**Design Note:**
The 20% agreement weight in `confidence.go` maps `Vision.Agreement` (fraction of voting runs that agreed) to a confidence component. In lens mode, `Agreement` is replaced by `CrossLensAgreement` — a different and stronger signal. A vision agreed upon by two analytically distinct lenses carries more epistemic weight than a vision agreed upon by two identical runs.

**Dependencies:** ML-1.3 (CrossLensAgreement field on Vision)

---

### Story ML-4.1 — Cross-Lens Agreement Confidence Component

**As a** confidence scorer in lens mode
**I want to** use cross-lens agreement instead of voting agreement
**So that** visions corroborated by multiple mental model lenses receive a meaningful confidence boost

**Acceptance Criteria:**
```gherkin
Given a vision with CrossLensAgreement = ["survivorship_bias", "inversion"]
When ScoreConfidence() runs in lens mode
Then the agreement component returns a high value (e.g., 0.85)
And the overall confidence reflects the cross-lens corroboration

Given a vision produced by only one lens with CrossLensAgreement empty
When ScoreConfidence() runs in lens mode
Then the agreement component returns a neutral value (0.5)
And the vision is not penalized for being lens-specific

Given lens mode is not active (legacy mode)
When ScoreConfidence() runs
Then behavior is unchanged
```

**Critical file:** `internal/dream/confidence.go` — `scoreAgreement()`
**Status: Ready for user stories**

---

## Epic ML-5: Lens Quality Observability

**Design Note:**
The `dream --quality` and `dream --compare` commands need to surface per-lens metrics so users can evaluate which lenses are finding the most actionable visions and whether the lens configuration is delivering value relative to cost.

**Dependencies:** ML-1.4 (quality ledger label), ML-1.3 (vision lens tags)

---

### Story ML-5.1 — Per-Lens Vision Counts in Quality Ledger

**As a** user reviewing dream quality
**I want to** see how many visions each lens produced and how many were applied
**So that** I can tune which lenses to run and identify underperforming lenses

**Decision resolved (Q10):** Per-lens breakdown via `--lens` flag only. The main table stays compact — inline per-lens columns would overflow standard terminals.

**Acceptance Criteria:**
```gherkin
Given a lens mode cycle completed with lenses ["structural", "survivorship_bias", "inversion"]
When mote dream --quality is run without --lens
Then the table shows total visions per row only
And no per-lens columns are added to the default view

Given mote dream --quality --lens is run
Then an additional section shows per-lens vision counts and apply rates for each row
And lenses that produced 0 findings are flagged with a warning indicator

Given a legacy self_consistency_runs row exists in the quality ledger
When mote dream --quality --lens is run
Then the legacy row shows "N/A" in the per-lens section
```

**Critical files:** `internal/dream/quality.go`, `cmd/mote/cmd_dream.go`
**Status: Ready for user stories**

---

### Story ML-5.2 — Vision Provenance in Dream Review

**As a** user reviewing deferred visions
**I want to** see which lens produced each vision
**So that** I can apply appropriate skepticism (e.g., opportunity cost lens visions require more scrutiny than survivorship bias visions, which are more operationally grounded)

**Acceptance Criteria:**
```gherkin
Given a deferred vision was produced by the inversion lens
When mote dream --review shows the vision
Then the lens source is displayed alongside the vision

Given a deferred vision has CrossLensAgreement set
When mote dream --review shows the vision
Then the cross-lens agreement is highlighted (e.g., "[2 lenses agreed: surv, conf-bias]")
```

**Critical file:** `cmd/mote/cmd_dream.go` — review display logic
**Status: Ready for user stories**

---

## Epic ML-6: Future Mental Models *(Stubs — Not Ready for Stories)*

The following models from the inventory are candidates for future lens implementation. Each requires a design session to define detection heuristics before prompting is possible.

### Incentives (Inventory #9)
**Rationale:** Munger called this "the most important principle in all of social science." For a knowledge graph: Do documented decisions reveal implicit incentive structures that contradict stated goals? Are there recurring decision patterns that suggest misaligned incentives between teams? High conceptual value; hard to operationalize without a concrete detection heuristic for prose motes. Requires defining what "incentive signal" looks like in a lesson or decision mote.

### Anchoring Bias (Inventory #12)
**Rationale:** Early documented lessons or initial architectural decisions may be anchoring later decisions without explicit re-evaluation. Detection would look for motes that reference early decisions as settled rather than questioning their continued validity. Needs scoping: how to distinguish a well-grounded foundational decision from one that is anchoring inappropriately.

### Pareto Principle / 80-20 (Inventory #8)
**Rationale:** Graph analysis lens: which 20% of motes generate 80% of retrieval value? Are there highly-connected hub motes being under-maintained? Are there peripheral motes consuming documentation effort disproportionate to their retrieval frequency? This could surface a `prioritize` or `deprioritize` vision type. Needs scoping against existing scoring and stats commands to identify what is not already covered.

### Hanlon's Razor (Inventory #13)
**Rationale:** Applies specifically to incident post-mortems and failure analysis motes. Are failures being attributed to individual incompetence or malice rather than systemic issues? A dedicated lens could flag motes that assign blame without documenting the system conditions that made the failure possible. Narrow scope — may not justify a full batch pass for most project graphs. Better as a targeted prescanner candidate for motes tagged `failure`, `incident`, or `revert`.

---

## Implementation Order

```
ML-1 (Architecture) ──────────────────────────────────────────────┐
                    │                                             │
                    ▼                                             ▼
         ML-2.1 + ML-2.2 + ML-2.3 + ML-2.7      ML-3.1 (can run parallel with ML-2)
         (structural, survivorship, feedback,
          confirmation bias — all ready)
                    │
                    ▼
         ML-2.4 + ML-2.6 (inversion, probabilistic) ── after ML-2.1-2.3 validated
                    │
                    ▼
         ML-4.1 (confidence scoring)
         ML-5.2 (vision provenance, quick win)
                    │
                    ▼
         ML-5.1 (quality observability)
         ML-2.5 + ML-2.8 + ML-2.9 (first principles, opportunity cost, occam's razor)
```

ML-2.10 (Second-Order Impact) and ML-6 stubs are deferred until scoping is complete.

---

## Open Questions Summary

| # | Question | Blocks | Recommendation |
|---|----------|--------|----------------|
| Q1 | Flexible vs. fixed lens count? | ML-1.1 | Flexible — list-based config is more powerful |
| Q2 | Structural lens scope — actions in cognitive lenses too? | ML-2.2–2.7 | No — structural lens only; cognitive lenses output cognitive findings |
| Q3 | Empty lens result — empty set or explanation? | ML-2.x | Empty set — reconciliation tokens wasted on "nothing found" |
| Q4 | Which cognitive lenses for v1? | ML-2 | Survivorship Bias + Feedback Loops + Inversion (Munger's Rule of 3: psychological, causal, critical thinking) |
| Q5 | New mental models in scope? | ML-2.4–2.7 | Yes — Inversion, First Principles, Probabilistic Thinking, Confirmation Bias upgrade all included |
| Q6 | Quality ledger migration for old rows? | ML-1.4 | No migration — old rows display as-is |
| Q7 | Inversion link type — new `assumption_risk` or reuse `survivorship_risk`? | ML-2.4 | **Resolved: New type** — survivorship risk = missing data; assumption risk = unvalidated extrapolation |
| Q8 | First Principles vision type — new `decompose_suggestion` or extend existing? | ML-2.5 | **Resolved: New type** — decomposition is the inverse of merge; structural lens owns merge |
| Q9 | Opportunity Cost lens — should it propose net-new mote creation? | ML-2.8 | **Resolved: No** — `add_signal` with `knowledge_gap:` rationale prefix; no mote creation |
| Q10 | ML-5.1 layout — inline per-lens columns vs. `--lens` flag? | ML-5.1 | **Resolved: `--lens` flag** — table is already wide; default stays compact |
