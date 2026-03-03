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

Respond with JSON: {"visions": [...], "lucid_log_updates": {"observed_patterns": [...], "tensions": [...], "visions_summary": [...], "interrupts": [...], "strata_health": [...]}}

Each vision should have: type (link_suggestion|contradiction|tag_refinement|staleness|compression|signal), action, source_motes, target_motes (optional), link_type (optional), rationale, severity (low|medium|high), tags (optional).
`

var reconPromptTmpl = `You are performing reconciliation across dream cycle batches.

## Lucid Log (accumulated across all batches)
{{.LucidLog}}

Review the accumulated patterns, tensions, and vision summaries. Produce a final consolidated list of visions that resolves conflicts, removes duplicates, and prioritizes high-value changes.

Respond with JSON: {"visions": [...]}

Each vision should have: type, action, source_motes, target_motes (optional), link_type (optional), rationale, severity (low|medium|high), tags (optional).
`
