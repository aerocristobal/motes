// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"bytes"
	"strings"
	"text/template"
	"time"

	"motes/internal/core"
)

// PromptBuilder constructs prompts for dream cycle invocations.
type PromptBuilder struct {
	batchTmpl    *template.Template
	reconTmpl    *template.Template
	reconLensTmpl *template.Template
	lensTmpls    map[string]*template.Template
	reader       func(string) (*core.Mote, error)
}

// KnownLenses is the set of recognized lens identifiers.
// Entries are added as lens prompt implementations are completed (ML-2 stories).
var KnownLenses = map[string]bool{
	"structural":        true,
	"survivorship_bias": true,
	"feedback_loops":    true,
	"inversion":         true,
	"first_principles":  true,
	"probabilistic":     true,
	"confirmation_bias": true,
	"opportunity_cost":  true,
	"occams_razor":      true,
}

// NewPromptBuilder creates a prompt builder with mote reader function.
func NewPromptBuilder(reader func(string) (*core.Mote, error)) *PromptBuilder {
	funcMap := template.FuncMap{
		"joinTags":   func(tags []string) string { return strings.Join(tags, ", ") },
		"formatTime": formatTime,
		"hasTask": func(tasks []string, target string) bool {
			for _, t := range tasks {
				if t == target {
					return true
				}
			}
			return false
		},
	}
	pb := &PromptBuilder{reader: reader}
	pb.batchTmpl = template.Must(template.New("batch").Funcs(funcMap).Parse(batchPromptTmpl))
	pb.reconTmpl = template.Must(template.New("recon").Funcs(funcMap).Parse(reconPromptTmpl))
	pb.reconLensTmpl = template.Must(template.New("reconLens").Funcs(funcMap).Parse(reconLensPromptTmpl))
	pb.lensTmpls = make(map[string]*template.Template)
	for name, src := range map[string]string{
		"structural":        structuralLensPrompt,
		"survivorship_bias": survivorshipBiasLensPrompt,
		"feedback_loops":    feedbackLoopsLensPrompt,
		"confirmation_bias": confirmationBiasLensPrompt,
		"inversion":         inversionLensPrompt,
		"probabilistic":     probabilisticLensPrompt,
		"first_principles":  firstPrinciplesLensPrompt,
		"opportunity_cost":  opportunityCostLensPrompt,
		"occams_razor":      occamsRazorLensPrompt,
	} {
		pb.lensTmpls[name] = template.Must(template.New(name).Funcs(funcMap).Parse(src))
	}
	return pb
}

type batchPromptData struct {
	LucidLog string
	Phase    string
	Cluster  string
	Motes    []*core.Mote
	Tasks    []string
}

// BuildBatchPrompt generates the prompt for a single batch.
// lens is the cognitive lens to apply; empty string uses the legacy all-in-one prompt.
func (pb *PromptBuilder) BuildBatchPrompt(batch Batch, ll *LucidLog, lens string) string {
	var motes []*core.Mote
	for _, id := range batch.MoteIDs {
		m, err := pb.reader(id)
		if err != nil {
			continue
		}
		motes = append(motes, m)
	}

	data := batchPromptData{
		LucidLog: ll.Serialize(),
		Phase:    batch.Phase,
		Cluster:  batch.PrimaryCluster,
		Motes:    motes,
		Tasks:    batch.Tasks,
	}

	tmpl := pb.batchTmpl
	if lens != "" {
		if lt := pb.lensTemplate(lens); lt != nil {
			tmpl = lt
		}
		// If no template registered yet for this lens, fall back to all-in-one.
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return ""
	}
	return buf.String()
}

// lensTemplate returns the parsed template for the named lens, or nil if not yet implemented.
// Nil causes BuildBatchPrompt to fall back to the legacy all-in-one template.
func (pb *PromptBuilder) lensTemplate(lens string) *template.Template {
	if t, ok := pb.lensTmpls[lens]; ok {
		return t
	}
	return nil
}

type reconPromptData struct {
	LucidLog string
	Lenses   []string
}

// BuildReconciliationPrompt generates the reconciliation prompt from the lucid log.
// When lenses are provided (lens mode), includes cross-lens synthesis instructions.
func (pb *PromptBuilder) BuildReconciliationPrompt(ll *LucidLog, lenses ...string) string {
	data := reconPromptData{LucidLog: ll.Serialize(), Lenses: lenses}
	tmpl := pb.reconTmpl
	if len(lenses) > 0 {
		tmpl = pb.reconLensTmpl
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return ""
	}
	return buf.String()
}

func formatTime(t *time.Time) string {
	if t == nil {
		return "never"
	}
	return t.Format("2006-01-02")
}

var batchPromptTmpl = `You are performing dream cycle maintenance on a mote nebula.

## Lucid Log
{{.LucidLog}}

## Batch ({{.Phase}}: {{.Cluster}})
{{range .Motes}}
### {{.ID}} — {{.Title}}
Type: {{.Type}} | Origin: {{.Origin}} | Weight: {{printf "%.2f" .Weight}} | Tags: {{joinTags .Tags}}
Last accessed: {{formatTime .LastAccessed}} | Access count: {{.AccessCount}}

{{.Body}}
---
{{end}}

## Tasks
{{range .Tasks}}- {{.}}
{{end}}
{{if hasTask .Tasks "content_link_review"}}
## Content Similarity Context
Some motes in this batch were paired by BM25 content similarity — they share distinctive vocabulary
but have no explicit links or shared tags. For each "content_link_review" task, evaluate whether the
conceptual overlap warrants a permanent "relates_to" link. Only promote pairs with genuine thematic
connection, not incidental vocabulary overlap.

When two or more motes describe concepts where one concept causally enables, amplifies, or counteracts
another, express this as a link_suggestion using a typed link:
- link_type: "reinforces"  — A increases or amplifies B (reinforcing/positive loop leg)
- link_type: "counteracts" — A reduces or stabilizes B (balancing/negative loop leg)
- link_type: "delays"      — A causes B but with a significant time lag

If the linked motes complete a cycle (A→B→A), record the loop in lucid_log_updates.observed_patterns:
  pattern_id: "loop_reinforcing_<cluster>" or "loop_balancing_<cluster>"
  description: "Reinforcing loop: [what amplifies]" or "Balancing loop: [what stabilizes]"
  mote_ids: all motes in the cycle
  strength: 0.7–0.9 if causal language is explicit in the body; 0.4–0.6 if inferred

Only flag loops grounded in the motes' body text — do not infer loops from titles alone.
{{end}}
{{if hasTask .Tasks "merge_review"}}
## Merge Review Context
Some motes in this batch were flagged as highly similar. For "merge_review" tasks,
evaluate whether 3+ motes are truly redundant and should be merged into one.
If yes, produce a merge_suggestion vision:
- type: "merge_suggestion", action: "merge"
- source_motes: ALL mote IDs to merge
- tags: union of all tags (deduplicated)
- rationale: FULL body text of the new merged mote (first line = title, rest = body)
- severity: "medium"
Only merge truly redundant content. Different perspectives on the same topic should stay separate.
{{end}}
{{if hasTask .Tasks "action_extraction"}}
## Action Extraction Context
For "action_extraction" tasks, extract a single prescriptive action sentence from each lesson or
decision mote's body. The action should be an imperative sentence (max 120 chars) that a developer
can apply directly — e.g., "Check response body for error field even on 2xx status codes."
Produce an action_extraction vision:
- type: "action_extraction", action: "add_action"
- source_motes: [the mote ID]
- rationale: the extracted action sentence (imperative, max 120 chars)
- severity: "low"
Skip motes whose body does not contain a clear prescriptive statement.
{{end}}

## Survivorship Bias Guard
For any lesson, decision, or context mote, check whether the body presents conclusions based
only on visible outcomes (successes, survivors, returning cases) while ignoring the unobserved
failures or non-survivors.

Indicators:
- "Successful X always Y" or "the winning approach was Z" without base-rate or failure data
- Lessons derived from a single positive outcome without noting failure conditions
- Advice extrapolated from a selective sample (only successful projects, only cases that completed)
- Missing counterfactual: "those who did X succeeded" without evidence on those who did X and failed

When survivorship bias is detected, record in lucid_log_updates.interrupts:
  severity: "medium" (use "high" if the claim is strongly stated as universal)
  description: "Survivorship bias: [specific claim] — missing evidence from non-surviving cases"
  mote_id: the affected mote's ID

When two or more motes in this batch both show survivorship bias on the same topic, also produce
a link_suggestion vision with link_type: "survivorship_risk" connecting them — so the pattern
surfaces in reconciliation and becomes navigable in the graph.
Only flag cases grounded in the body text; do not flag motes that explicitly acknowledge their limitations.

IMPORTANT: Respond with ONLY a single JSON object, no other text. Do not wrap in markdown code fences.

Required format:
{"visions": [{"type": "link_suggestion", "action": "add_link", "source_motes": ["id1"], "target_motes": ["id2"], "link_type": "relates_to", "rationale": "why", "severity": "medium"}], "lucid_log_updates": {"observed_patterns": [{"pattern_id": "p1", "description": "what", "mote_ids": ["id1"], "strength": 0.8}], "tensions": [{"tension_id": "t1", "description": "what", "mote_ids": ["id1"]}], "visions_summary": [{"type": "link_suggestion", "mote_ids": ["id1"], "batch": 1}], "interrupts": [{"severity": "medium", "description": "...", "mote_id": "id1"}], "strata_health": []}}

Your ENTIRE response must be this JSON object. No text before or after.

Vision types: link_suggestion, contradiction, tag_refinement, staleness, compression, signal, merge_suggestion, action_extraction.
Vision actions: add_link, remove, split_tag, deprecate, compress, add_signal, merge, add_action.
If no findings for a category, use an empty array [].
`

var reconPromptTmpl = `You are performing reconciliation across dream cycle batches.

## Lucid Log (accumulated across all batches)
{{.LucidLog}}

Review the accumulated patterns, tensions, and vision summaries. Produce a final consolidated list of visions that resolves conflicts, removes duplicates, and prioritizes high-value changes.

When multiple visions target the same motes or address the same issue:
- MERGE them into a single vision with combined source_motes and target_motes lists
- Synthesize rationales from all contributing visions
- Use the highest severity among the merged visions
- Note the number of independent batches that suggested this vision in the rationale

When synthesizing survivorship bias patterns from interrupts:
- If 2+ interrupts share a common mote tag or topic, produce a link_suggestion vision
  connecting the affected motes with link_type: "survivorship_risk" at medium severity
- In the rationale: name the missing class of evidence (failure cases, non-survivors, control group)
  and note what complementary documentation would correct the bias
- Flag knowledge clusters where survivorship bias is concentrated for priority human review

When synthesizing loop patterns from observed_patterns:
- Merge partial loop patterns from different batches that share mote IDs into complete cycles
- Distinguish reinforcing loops (virtuous or vicious) from balancing loops (stabilizing or oscillating)
- For reinforcing loops: note whether the loop is beneficial (virtuous cycle) or pathological (vicious cycle)
- For any mote that appears in 2+ loops, flag it as a leverage point in that vision's rationale
- Do not suppress loop annotations — surface them for reviewer awareness even at medium confidence

After deduplication, apply second-order thinking to the consolidated vision list:
- Would the combined effect of these visions over-concentrate weight on a small number of motes (i.e., the same mote appears as a target in 3+ visions)?
- Does any deprecation vision orphan motes that other visions in this list are linking to?
- Does any merge vision create a hub that itself becomes a target in 3+ other visions?
If any of these apply, note the concern in that vision's rationale field so reviewers can decide. Do not suppress visions on these grounds.

IMPORTANT: Respond with ONLY a single JSON object, no other text. Do not wrap in markdown code fences.

Required format:
{"visions": [{"type": "link_suggestion", "action": "add_link", "source_motes": ["id1"], "target_motes": ["id2"], "link_type": "relates_to", "rationale": "why", "severity": "medium"}]}

Vision types: link_suggestion, contradiction, tag_refinement, staleness, compression, signal, merge_suggestion, action_extraction.
If no visions, respond with: {"visions": []}
`

// reconLensPromptTmpl is the reconciliation prompt used when lens mode was active for the batch phase.
// It includes cross-lens synthesis instructions in addition to the standard deduplication pass.
var reconLensPromptTmpl = `You are performing reconciliation across dream cycle batches.
This reconciliation synthesizes lens-mode results from {{len .Lenses}} analytical lenses: {{range $i, $l := .Lenses}}{{if $i}}, {{end}}{{$l}}{{end}}.

## Lens Mode Synthesis Context
Each lens ran independently with a distinct analytical focus. Visions tagged with cross_lens_agreement
were found independently by multiple lenses — treat cross-lens corroboration as a stronger epistemic
signal than single-lens findings. A vision seen by two analytically distinct lenses (e.g., survivorship
bias AND inversion both flagging the same mote) warrants higher priority than a vision seen only once.

When evaluating cross-lens visions:
- Elevate severity when 2+ lenses converge on the same target mote
- In the rationale, note which lenses agreed and why that convergence is meaningful
- Do not suppress single-lens findings — they may represent lens-specific insight not visible to other lenses

## Lucid Log (accumulated across all batches)
{{.LucidLog}}

Review the accumulated patterns, tensions, and vision summaries. Produce a final consolidated list of visions that resolves conflicts, removes duplicates, and prioritizes high-value changes.

When multiple visions target the same motes or address the same issue:
- MERGE them into a single vision with combined source_motes and target_motes lists
- Synthesize rationales from all contributing visions
- Use the highest severity among the merged visions
- Note the number of independent lenses/batches that suggested this vision in the rationale

When synthesizing survivorship bias patterns from interrupts:
- If 2+ interrupts share a common mote tag or topic, produce a link_suggestion vision
  connecting the affected motes with link_type: "survivorship_risk" at medium severity
- In the rationale: name the missing class of evidence and note what complementary documentation would correct the bias

When synthesizing loop patterns from observed_patterns:
- Merge partial loop patterns from different batches that share mote IDs into complete cycles
- For any mote that appears in 2+ loops, flag it as a leverage point in that vision's rationale

After deduplication, apply second-order thinking:
- Would the combined effect over-concentrate weight on a small number of motes?
- Does any deprecation vision orphan motes that other visions are linking to?
If any of these apply, note the concern in the vision's rationale field. Do not suppress visions.

IMPORTANT: Respond with ONLY a single JSON object, no other text. Do not wrap in markdown code fences.

Required format:
{"visions": [{"type": "link_suggestion", "action": "add_link", "source_motes": ["id1"], "target_motes": ["id2"], "link_type": "relates_to", "rationale": "why", "severity": "medium"}]}

Vision types: link_suggestion, contradiction, tag_refinement, staleness, compression, signal, merge_suggestion, action_extraction.
If no visions, respond with: {"visions": []}
`

// lensPromptHeader is the shared mote display block used by all lens templates.
const lensPromptHeader = `
## Lucid Log
{{.LucidLog}}

## Batch ({{.Phase}}: {{.Cluster}})
{{range .Motes}}
### {{.ID}} — {{.Title}}
Type: {{.Type}} | Origin: {{.Origin}} | Weight: {{printf "%.2f" .Weight}} | Tags: {{joinTags .Tags}}
Last accessed: {{formatTime .LastAccessed}} | Access count: {{.AccessCount}}

{{.Body}}
---
{{end}}`

// lensPromptFooter is the shared JSON format instruction used by all lens templates.
const lensPromptFooter = `
IMPORTANT: Respond with ONLY a single JSON object, no other text. Do not wrap in markdown code fences.

Required format:
{"visions": [{"type": "link_suggestion", "action": "add_link", "source_motes": ["id1"], "target_motes": ["id2"], "link_type": "relates_to", "rationale": "why", "severity": "medium"}], "lucid_log_updates": {"observed_patterns": [{"pattern_id": "p1", "description": "what", "mote_ids": ["id1"], "strength": 0.8}], "tensions": [], "visions_summary": [], "interrupts": [{"severity": "medium", "description": "...", "mote_id": "id1"}], "strata_health": []}}

Vision types: link_suggestion, contradiction, tag_refinement, staleness, compression, signal, merge_suggestion, action_extraction.
Vision actions: add_link, remove, split_tag, deprecate, compress, add_signal, merge, add_action.
If no findings for a category, use an empty array [].
`

// ML-2.1: Structural lens — graph hygiene (merge, action extraction, compression, contradiction, tag refinement).
var structuralLensPrompt = `You are performing dream cycle maintenance on a mote nebula using the **Structural** lens.
Focus exclusively on graph hygiene: merge candidates, action extraction, compression, contradictions, and tag refinement. Do not apply cognitive model analysis.
` + lensPromptHeader + `
## Structural Analysis

### Merge Detection
Evaluate whether 3+ motes in this batch are truly redundant (same concept, same level of abstraction). Only merge truly redundant content — different perspectives stay separate.
Produce a merge_suggestion:
- type: "merge_suggestion", action: "merge"
- source_motes: ALL mote IDs to merge
- rationale: FULL body text of the merged mote (first line = title, rest = body)
- severity: "medium"

### Action Extraction
For each lesson or decision mote, extract one prescriptive imperative sentence (max 120 chars) that a developer can apply directly. Skip motes without a clear prescriptive statement.
Produce an action_extraction vision:
- type: "action_extraction", action: "add_action"
- source_motes: [mote ID]
- rationale: the extracted action sentence
- severity: "low"

### Compression
Flag motes with excessive length or redundant phrasing that can be condensed without losing facts.
Produce a compression vision:
- type: "compression", action: "compress"
- source_motes: [mote ID]
- rationale: the compressed body text
- severity: "low"

### Contradiction Detection
Identify motes whose bodies make contradictory claims or recommendations.
- type: "contradiction", action: "add_link"
- source_motes: [conflicting mote IDs]
- rationale: exactly what contradicts what
- severity: "medium"

### Tag Refinement
Identify motes with missing, incorrect, or redundant tags.
- type: "tag_refinement", action: "split_tag"
- source_motes: [mote ID]
- rationale: current tags and recommended changes
- severity: "low"
` + lensPromptFooter

// ML-2.2: Survivorship Bias lens — focused on detecting missing failure data and non-survivor evidence.
var survivorshipBiasLensPrompt = `You are performing dream cycle maintenance on a mote nebula using the **Survivorship Bias** lens.
Focus exclusively on detecting missing failure data, non-survivor evidence, and base-rate omissions. Ask: what would a researcher who studied the failures see that is absent from these motes?
` + lensPromptHeader + `
## Survivorship Bias Analysis

For every lesson, decision, and context mote, actively search for what is missing — the unseen failures, the counterfactuals, the base rates.

Detection patterns (treat each as a trigger for investigation):
- "Successful X always Y" — without evidence on unsuccessful X
- Lessons derived from a single positive outcome — without failure-mode data
- Advice citing only survivors (successful founders, winning projects, completed studies)
- Missing counterfactual: no evidence on those who tried the same approach and failed
- Sample restriction: conclusions drawn from a filtered sample (only cases that returned, only completed projects)

For each detected case, record in lucid_log_updates.interrupts:
  severity: "medium" (use "high" if the claim is stated as universal or prescriptive)
  description: "Survivorship bias: [specific claim] — missing evidence from [what class of cases]"
  mote_id: the affected mote's ID

When two or more motes in this batch show survivorship bias on the same topic, produce a link_suggestion:
- link_type: "survivorship_risk"
- source_motes + target_motes: the affected motes
- rationale: what common evidence class is missing across both motes
- severity: "medium"

Only flag cases grounded in the body text. Do not flag motes that explicitly acknowledge their limitations.
` + lensPromptFooter

// ML-2.3: Feedback Loops lens — traces causal chains, identifies reinforcing and balancing cycles.
var feedbackLoopsLensPrompt = `You are performing dream cycle maintenance on a mote nebula using the **Feedback Loops** lens.
Focus exclusively on tracing causal chains between motes and identifying reinforcing and balancing cycles. Do not apply survivorship bias or structural analysis.
` + lensPromptHeader + `
## Feedback Loop Analysis

Actively trace causal chains. For each mote, ask: "What does this cause or enable? What does it suppress or reduce? What does it depend on?"

Detection patterns:
- Reinforcing loops: A amplifies B which amplifies A (virtuous cycles: growth, learning spirals; vicious cycles: debt traps, quality spirals)
- Balancing loops: A reduces B which reduces pressure on A (stabilizing: self-correcting; oscillating: overshoot-and-correct)
- Delayed causation: A causes B but with significant time lag that obscures the relationship

When a partial chain is visible (A→B documented, B→C implied but not linked):
- Propose the completing link with a link_suggestion
- link_type: "reinforces" (A amplifies B), "counteracts" (A suppresses B), or "delays" (A causes B with lag)

When a complete loop is identified, record in lucid_log_updates.observed_patterns:
  pattern_id: "loop_reinforcing_<cluster>" or "loop_balancing_<cluster>"
  description: "Reinforcing loop: [what amplifies what]" or "Balancing loop: [what stabilizes what]"
  mote_ids: all motes in the cycle
  strength: 0.7–0.9 if causal language is explicit; 0.4–0.6 if inferred

Leverage point identification:
- A mote appearing in 2+ loops is a leverage point — note this in the vision rationale

Only flag loops grounded in the motes' body text. Do not infer loops from titles alone.
` + lensPromptFooter

// ML-2.7: Confirmation Bias lens — detects selective evidence marshaling.
var confirmationBiasLensPrompt = `You are performing dream cycle maintenance on a mote nebula using the **Confirmation Bias** lens.
Focus exclusively on detecting selective evidence marshaling: cases where a mote cites only confirming evidence while discounting or ignoring contradicting evidence. This is distinct from survivorship bias (missing failure data) and inversion (untested assumptions).
` + lensPromptHeader + `
## Confirmation Bias Analysis

For each lesson, decision, and context mote, investigate: does this mote present a conclusion whose supporting evidence was selectively gathered?

Detection patterns:
- All cited evidence points in one direction — no counter-evidence acknowledged
- Alternatives evaluated but only weaknesses of rejected options documented (not weaknesses of chosen option)
- Sources all agree with the conclusion — no dissenting expert or study referenced
- Disconfirming evidence dismissed without engaging its force ("despite X, we chose Y")
- Post-hoc rationalization: decision described before evidence, then evidence marshaled to support it

When a contradicting mote exists in the knowledge graph but is not linked:
- Produce a link_suggestion with link_type: "contradiction"
- source_motes: the biased mote, target_motes: the contradicting mote
- rationale: what exactly contradicts what
- severity: "medium"

When a mote shows one-sided evidence without a contradicting mote to link to:
- Produce a signal vision with action: "add_signal"
- source_motes: [the mote ID]
- rationale: "One-sided evidence: [what is missing] — recommend documenting counter-evidence or acknowledging limitations"
- severity: "low" (use "medium" if the conclusion is prescriptive)

Do not flag motes that explicitly acknowledge counter-evidence, limitations, or alternative views.
` + lensPromptFooter

// ML-2.4: Inversion lens — surfaces fragile assumptions, overconfident claims, and knowledge that would be most damaging if wrong.
var inversionLensPrompt = `You are performing dream cycle maintenance on a mote nebula using the **Inversion** lens.
Apply inversion thinking: for each mote, ask "What would have to be true for this to be wrong?" Surface fragile assumptions, overconfident universal claims, and knowledge that would be most damaging if incorrect.
This is distinct from survivorship bias (missing failure data) — inversion targets unvalidated extrapolation and untested assumptions.
` + lensPromptHeader + `
## Inversion Analysis

For every lesson, decision, and context mote, invert the claim and test the inversion:
- "This approach always works" → "Under what conditions would it fail?"
- "X causes Y" → "When does X NOT cause Y? What confounds the relationship?"
- "We should do Z" → "What are the strongest arguments against Z?"

Detection patterns:
- Universal claims ("always", "never", "every", "the only way") without documented boundary conditions
- Prescriptive advice without explicit failure conditions or exit criteria
- Confident conclusions that rest on a single undocumented assumption
- Knowledge clusters where all motes point toward the same conclusion (monoculture of evidence)

For each fragile assumption identified, produce a link_suggestion:
- link_type: "assumption_risk"
- source_motes: [the mote with the claim]
- rationale: "Fragile assumption: [what assumption] — if false, impact is [what]; untested because [why]"
- severity: "medium" (use "high" if a high-stakes decision rests on it)

When a knowledge cluster (3+ motes on the same topic) lacks opposing evidence, produce a signal:
- type: "signal", action: "add_signal"
- source_motes: [the motes in the cluster]
- rationale: "Evidence monoculture: all motes support [conclusion] — no dissenting data documented"
- severity: "low"

Do not flag motes that explicitly document failure conditions, alternatives considered, or exit criteria.
` + lensPromptFooter

// ML-2.6: Probabilistic Thinking lens — flags miscalibrated confidence, missing base rates, and overconfident claims.
var probabilisticLensPrompt = `You are performing dream cycle maintenance on a mote nebula using the **Probabilistic Thinking** lens.
Focus on calibration: flag lessons and decisions that claim certainty without empirical grounding or base-rate data. This is distinct from survivorship bias (missing failure examples) — probabilistic thinking targets miscalibrated confidence regardless of whether failures are visible.
` + lensPromptHeader + `
## Probabilistic Thinking Analysis

For every lesson, decision, and context mote, evaluate the calibration of its claims:
- Does the confidence level match the evidence quality?
- Are base rates or sample sizes provided for statistical claims?
- Are probabilities expressed as ranges rather than binary outcomes?

Detection patterns:
- "This always works" / "this never fails" — without frequency data
- Binary framing (success/failure) without probability range or confidence interval
- "Studies show X" without n, methodology, or confidence level
- Point estimates presented as certainties ("the conversion rate is 12%")
- Extrapolation from small samples stated as general truth

For each miscalibrated claim, produce a signal:
- type: "signal", action: "add_signal"
- source_motes: [the mote ID]
- rationale: "Overconfident claim: [what is stated as certain] — [what base-rate or confidence context is missing]"
- severity: "low" (use "medium" if a decision depends on this claim)

When a contradicting mote with relevant counter-evidence exists and is not linked:
- Produce a link_suggestion connecting the overconfident mote to the counter-evidence mote
- link_type: "relates_to"
- rationale: note how the counter-evidence revises the confidence level
- severity: "low"

Do not flag motes that appropriately qualify claims with frequency language, sample sizes, confidence ranges, or explicit caveats ("in our limited testing", "one data point", "approximately").
` + lensPromptFooter

// ML-2.5: First Principles lens — identifies derived conclusions vs grounded fundamentals, surfaces over-compressed compounds.
var firstPrinciplesLensPrompt = `You are performing dream cycle maintenance on a mote nebula using the **First Principles** lens.
Break knowledge down to its grounded fundamentals. Identify motes that are derived conclusions resting on unstated axioms, or compound motes that conflate multiple distinct concepts. This is the inverse of structural lens (which finds duplicates) — first principles finds over-compressed compounds.
` + lensPromptHeader + `
## First Principles Analysis

For every mote, ask: "Is this knowledge grounded in direct evidence, or is it derived from an assumption that should be made explicit?"

Detection patterns:
- Compound motes that conflate two or more distinct concepts that would be clearer as separate motes
- Analogy-based reasoning stated as principle ("we do X because Company Y does X") without first-principles grounding
- Derived conclusions presented as axioms without documenting the reasoning chain
- References to a concept that is never defined or grounded elsewhere in the batch

For over-compressed compound motes, produce a decompose_suggestion vision:
- type: "decompose_suggestion" (note: this is NOT merge_suggestion — structural lens owns merges)
- action: "split"  (use the closest available action — record "split" even if not in standard list)
- source_motes: [the compound mote ID]
- rationale: "Compound concept: [what should be split] — Concept A: [description] | Concept B: [description]"
- severity: "low" (use "medium" if the conflation causes confusion in linked motes)

For analogy-based reasoning without grounding:
- type: "signal", action: "add_signal"
- source_motes: [the mote ID]
- rationale: "Analogy without grounding: [the analogy used] — first-principles basis is undocumented"
- severity: "low"

For missing foundational concepts (referenced but undefined):
- type: "signal", action: "add_signal"
- source_motes: [the mote referencing the concept]
- rationale: "Missing foundation: [concept name] is referenced but never defined in the graph"
- severity: "low"

Do not flag motes that contain one well-grounded concept with a documented reasoning chain.
` + lensPromptFooter

// ML-2.8: Opportunity Cost lens — surfaces knowledge absences, underrepresented topics, and systematically avoided questions.
var opportunityCostLensPrompt = `You are performing dream cycle maintenance on a mote nebula using the **Opportunity Cost** lens.
Focus on what is absent: knowledge gaps, underrepresented topics, and decisions documented without alternatives. This lens produces add_signal visions for human review — it does NOT create new motes. All visions use rationale with a "knowledge_gap:" prefix.
` + lensPromptHeader + `
## Opportunity Cost Analysis

Examine what is NOT in the batch that SHOULD be. For every decision and lesson mote, ask: "What would a thorough reviewer expect to find here that is missing?"

Detection patterns:
- Decision motes that record the chosen option but no alternatives considered
- Lesson motes that describe what happened but not what was tried and failed
- Topics mentioned repeatedly across motes but lacking any associated lesson mote
- Decisions made on time pressure without documented opportunity cost evaluation

For each identified gap, produce a signal vision:
- type: "signal", action: "add_signal"
- source_motes: [the mote most closely related to the gap]
- rationale: "knowledge_gap: [what specific knowledge is missing and why it matters]"
- severity: "low" (use "medium" if the gap is around a high-stakes decision)

Examples of rationale text:
- "knowledge_gap: alternatives not documented — decision mote records only the chosen path"
- "knowledge_gap: topic lacks lesson motes — [topic] appears in N context motes but no lessons"
- "knowledge_gap: no failure-mode documentation — implementation mote lacks known failure conditions"

Do not produce signals for decisions that document both the chosen option and at least two alternatives. Do not create net-new motes or suggest content to be written — only flag the absence for human attention.
` + lensPromptFooter

// ML-2.9: Occam's Razor lens — detects unnecessary complexity: over-linked motes, redundant splits, and collapsible abstractions.
var occamsRazorLensPrompt = `You are performing dream cycle maintenance on a mote nebula using the **Occam's Razor** lens.
Prefer the simpler explanation. Find unnecessary complexity: over-linked motes, abstraction layers that obscure more than they clarify, and concept splits that fragment rather than illuminate. This operates at the semantic layer — it complements the deterministic mote doctor (which catches metric-based complexity) by catching meaning-level over-engineering.
This is distinct from structural lens (which finds duplicate content for merging) — Occam's Razor finds over-complex abstractions to collapse.
` + lensPromptHeader + `
## Occam's Razor Analysis

For each mote, ask: "Is this concept earning its complexity? Or does simpler reasoning explain the same thing?"

Detection patterns:
- Motes with 5+ links where the link chain is longer than the reasoning requires
- Abstraction layers that add indirection without adding clarity (wrapper concepts)
- Concept splits that represent the same underlying idea at different labels rather than different content
- Long mote bodies that could be expressed in one sentence without losing meaning
- Chains of 3+ derived concepts where each step adds minimal new information

For over-complex abstractions that could be collapsed, produce a decompose_suggestion:
- type: "decompose_suggestion", action: "compress"  (collapse, not split)
- source_motes: [the over-complex mote ID]
- rationale: "Unnecessary abstraction: [what layer could be collapsed] — simpler alternative: [what to replace it with]"
- severity: "low" (use "medium" if the complexity actively misleads linked motes)

Do not flag motes that the deterministic doctor command would already catch (high link counts, long chains) — focus on semantic complexity that metrics cannot detect. Do not flag motes with appropriate complexity matching their conceptual scope.
` + lensPromptFooter
