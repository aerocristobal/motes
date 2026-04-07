// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"motes/internal/compliance"
)

var complianceCmd = &cobra.Command{
	Use:   "compliance",
	Short: "Compliance and security reporting",
}

var complianceExportCmd = &cobra.Command{
	Use:   "export --format=oscal-cdef",
	Short: "Export an OSCAL Component Definition documenting security controls",
	RunE:  runComplianceExport,
}

var (
	complianceFormat string
	complianceOutput string
)

func init() {
	complianceExportCmd.Flags().StringVar(&complianceFormat, "format", "", "Export format (required: oscal-cdef)")
	complianceExportCmd.MarkFlagRequired("format")
	complianceExportCmd.Flags().StringVar(&complianceOutput, "output", "", "Output file (default: stdout)")

	complianceCmd.AddCommand(complianceExportCmd)
	rootCmd.AddCommand(complianceCmd)
}

func runComplianceExport(cmd *cobra.Command, args []string) error {
	if strings.ToLower(complianceFormat) != "oscal-cdef" {
		return fmt.Errorf("unsupported format %q: only oscal-cdef is supported", complianceFormat)
	}

	doc, err := compliance.GenerateComponentDefinition()
	if err != nil {
		return fmt.Errorf("generate component definition: %w", err)
	}

	errs := compliance.ValidateComponentDefinition(doc)
	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", strings.Join(errs, "; "))
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	data = append(data, '\n')

	if complianceOutput != "" {
		if err := os.WriteFile(complianceOutput, data, 0644); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Written to %s\n", complianceOutput)
		return nil
	}

	_, err = cmd.OutOrStdout().Write(data)
	return err
}
