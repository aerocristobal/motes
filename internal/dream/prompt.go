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
	batchTmpl *template.Template
	reconTmpl *template.Template
	reader    func(string) (*core.Mote, error)
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
func (pb *PromptBuilder) BuildBatchPrompt(batch Batch, ll *LucidLog) string {
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

	var buf bytes.Buffer
	if err := pb.batchTmpl.Execute(&buf, data); err != nil {
		return ""
	}
	return buf.String()
}

// BuildReconciliationPrompt generates the reconciliation prompt from the lucid log.
func (pb *PromptBuilder) BuildReconciliationPrompt(ll *LucidLog) string {
	var buf bytes.Buffer
	if err := pb.reconTmpl.Execute(&buf, map[string]string{
		"LucidLog": ll.Serialize(),
	}); err != nil {
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
