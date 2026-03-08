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
{{end}}

IMPORTANT: Respond with ONLY a single JSON object, no other text. Do not wrap in markdown code fences.

Required format:
{"visions": [{"type": "link_suggestion", "action": "add_link", "source_motes": ["id1"], "target_motes": ["id2"], "link_type": "relates_to", "rationale": "why", "severity": "medium"}], "lucid_log_updates": {"observed_patterns": [{"pattern_id": "p1", "description": "what", "mote_ids": ["id1"], "strength": 0.8}], "tensions": [{"tension_id": "t1", "description": "what", "mote_ids": ["id1"]}], "visions_summary": [{"type": "link_suggestion", "mote_ids": ["id1"], "batch": 1}], "interrupts": [], "strata_health": []}}

Your ENTIRE response must be this JSON object. No text before or after.

Vision types: link_suggestion, contradiction, tag_refinement, staleness, compression, signal.
Vision actions: add_link, remove, split_tag, deprecate, compress, add_signal.
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

IMPORTANT: Respond with ONLY a single JSON object, no other text. Do not wrap in markdown code fences.

Required format:
{"visions": [{"type": "link_suggestion", "action": "add_link", "source_motes": ["id1"], "target_motes": ["id2"], "link_type": "relates_to", "rationale": "why", "severity": "medium"}]}

Vision types: link_suggestion, contradiction, tag_refinement, staleness, compression, signal.
If no visions, respond with: {"visions": []}
`
