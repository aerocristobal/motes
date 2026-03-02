package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/format"
)

var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Display a mote's content and links",
	Args:  cobra.ExactArgs(1),
	RunE:  runShow,
}

func init() {
	rootCmd.AddCommand(showCmd)
}

func runShow(cmd *cobra.Command, args []string) error {
	root := mustFindRoot()
	mm := core.NewMoteManager(root)

	m, err := mm.Read(args[0])
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "mote not found: %s\n", args[0])
			os.Exit(1)
		}
		return err
	}

	fmt.Println(format.Header(m.ID))
	fmt.Println(format.Field("type", m.Type))
	fmt.Println(format.Field("status", m.Status))
	fmt.Println(format.Field("title", m.Title))
	fmt.Println(format.Field("tags", format.TagList(m.Tags)))
	fmt.Println(format.Field("weight", fmt.Sprintf("%.2f", m.Weight)))
	fmt.Println(format.Field("origin", m.Origin))
	fmt.Println(format.Field("created_at", m.CreatedAt.Format(time.RFC3339)))
	if m.LastAccessed != nil {
		fmt.Println(format.Field("last_accessed", m.LastAccessed.Format(time.RFC3339)))
	} else {
		fmt.Println(format.Field("last_accessed", "(never)"))
	}
	fmt.Println(format.Field("access_count", fmt.Sprintf("%d", m.AccessCount)))

	if hasAnyLinks(m) {
		fmt.Println("\n--- links ---")
		printLinks(mm, "depends_on", m.DependsOn)
		printLinks(mm, "blocks", m.Blocks)
		printLinks(mm, "relates_to", m.RelatesTo)
		printLinks(mm, "builds_on", m.BuildsOn)
		printLinks(mm, "contradicts", m.Contradicts)
		printLinks(mm, "supersedes", m.Supersedes)
		printLinks(mm, "caused_by", m.CausedBy)
		printLinks(mm, "informed_by", m.InformedBy)
	}

	if m.Body != "" {
		fmt.Println("\n--- body ---")
		fmt.Print(m.Body)
		if m.Body[len(m.Body)-1] != '\n' {
			fmt.Println()
		}
	}

	_ = mm.AppendAccessBatch(m.ID)
	return nil
}

func hasAnyLinks(m *core.Mote) bool {
	return len(m.DependsOn)+len(m.Blocks)+len(m.RelatesTo)+
		len(m.BuildsOn)+len(m.Contradicts)+len(m.Supersedes)+
		len(m.CausedBy)+len(m.InformedBy) > 0
}

func printLinks(mm *core.MoteManager, label string, ids []string) {
	for _, id := range ids {
		linked, err := mm.Read(id)
		if err == nil {
			fmt.Println(format.Field(label, id+" ("+linked.Title+")"))
		} else {
			fmt.Println(format.Field(label, id))
		}
	}
}
