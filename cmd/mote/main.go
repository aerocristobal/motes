package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "mote",
	Short:   "AI-native context and memory system",
	Long:    "Motes is an AI-native context and memory system. Knowledge is stored as atomic units (motes) linked in two dimensions: dependency links and semantic links.",
	Version: "0.1.0",
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
