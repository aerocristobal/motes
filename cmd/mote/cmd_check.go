package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/security"
)

var checkCmd = &cobra.Command{
	Use:   "check <id> <index>",
	Short: "Toggle an acceptance criterion",
	Long:  "Toggle a 1-based acceptance criterion. Use --all to mark all criteria met.",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runCheck,
}

var checkAll bool

func init() {
	checkCmd.Flags().BoolVar(&checkAll, "all", false, "Mark all acceptance criteria met")
	rootCmd.AddCommand(checkCmd)
}

func runCheck(cmd *cobra.Command, args []string) error {
	moteID := args[0]
	if err := security.ValidateMoteID(moteID); err != nil {
		return fmt.Errorf("invalid mote ID: %w", err)
	}

	root, err := findMemoryRoot()
	if err != nil {
		return err
	}
	mm := core.NewMoteManager(root)

	m, err := mm.Read(moteID)
	if err != nil {
		return fmt.Errorf("read mote: %w", err)
	}

	if len(m.Acceptance) == 0 {
		return fmt.Errorf("mote %s has no acceptance criteria", moteID)
	}

	// Ensure AcceptanceMet is the right length
	for len(m.AcceptanceMet) < len(m.Acceptance) {
		m.AcceptanceMet = append(m.AcceptanceMet, false)
	}

	if checkAll {
		for i := range m.AcceptanceMet {
			m.AcceptanceMet[i] = true
		}
		if err := mm.Update(moteID, core.UpdateOpts{
			AcceptanceMet: m.AcceptanceMet,
		}); err != nil {
			return err
		}
		fmt.Printf("Marked all %d criteria met for %s\n", len(m.Acceptance), moteID)
		return nil
	}

	if len(args) < 2 {
		return fmt.Errorf("index required (1-%d) or use --all", len(m.Acceptance))
	}

	idx, err := strconv.Atoi(args[1])
	if err != nil || idx < 1 || idx > len(m.Acceptance) {
		return fmt.Errorf("index must be 1-%d", len(m.Acceptance))
	}

	m.AcceptanceMet[idx-1] = !m.AcceptanceMet[idx-1]
	if err := mm.Update(moteID, core.UpdateOpts{
		AcceptanceMet: m.AcceptanceMet,
	}); err != nil {
		return err
	}

	status := "met"
	if !m.AcceptanceMet[idx-1] {
		status = "unmet"
	}
	fmt.Printf("Criterion %d %s: %s\n", idx, status, m.Acceptance[idx-1])
	return nil
}
