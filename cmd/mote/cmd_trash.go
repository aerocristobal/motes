package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/security"
)

var trashCmd = &cobra.Command{
	Use:   "trash",
	Short: "Manage trashed motes",
}

var trashListCmd = &cobra.Command{
	Use:   "list",
	Short: "List trashed motes",
	Args:  cobra.NoArgs,
	RunE:  runTrashList,
}

var trashRestoreCmd = &cobra.Command{
	Use:   "restore <id>",
	Short: "Restore a mote from trash",
	Args:  cobra.ExactArgs(1),
	RunE:  runTrashRestore,
}

var trashPurgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Permanently delete expired trashed motes",
	Args:  cobra.NoArgs,
	RunE:  runTrashPurge,
}

var trashRetentionCmd = &cobra.Command{
	Use:   "retention [days]",
	Short: "Show or set trash retention period",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTrashRetention,
}

var purgeAll bool

func init() {
	trashPurgeCmd.Flags().BoolVar(&purgeAll, "all", false, "Purge all trashed motes regardless of age")
	trashCmd.AddCommand(trashListCmd)
	trashCmd.AddCommand(trashRestoreCmd)
	trashCmd.AddCommand(trashPurgeCmd)
	trashCmd.AddCommand(trashRetentionCmd)
	rootCmd.AddCommand(trashCmd)
}

func runTrashList(cmd *cobra.Command, args []string) error {
	root, err := findMemoryRoot()
	if err != nil {
		return err
	}

	mm := core.NewMoteManager(root)
	cfg, err := core.LoadConfig(root)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	motes, err := mm.ListTrash()
	if err != nil {
		return fmt.Errorf("list trash: %w", err)
	}

	if len(motes) == 0 {
		fmt.Fprintln(os.Stdout, "Trash is empty.")
		return nil
	}

	now := time.Now().UTC()
	retention := time.Duration(cfg.Trash.RetentionDays) * 24 * time.Hour

	fmt.Fprintf(os.Stdout, "%-30s %-30s %-12s %s\n", "ID", "TITLE", "DELETED", "DAYS LEFT")
	for _, m := range motes {
		deletedStr := "unknown"
		daysLeft := "?"
		if m.DeletedAt != nil {
			deletedStr = m.DeletedAt.Format("2006-01-02")
			expiry := m.DeletedAt.Add(retention)
			remaining := expiry.Sub(now).Hours() / 24
			if remaining < 0 {
				daysLeft = "expired"
			} else {
				daysLeft = fmt.Sprintf("%.0f", math.Ceil(remaining))
			}
		}
		title := m.Title
		if len(title) > 28 {
			title = title[:25] + "..."
		}
		fmt.Fprintf(os.Stdout, "%-30s %-30s %-12s %s\n", m.ID, title, deletedStr, daysLeft)
	}
	return nil
}

func runTrashRestore(cmd *cobra.Command, args []string) error {
	moteID := args[0]

	if err := security.ValidateMoteID(moteID); err != nil {
		return fmt.Errorf("invalid mote ID: %w", err)
	}

	root, err := findMemoryRoot()
	if err != nil {
		return err
	}

	mm := core.NewMoteManager(root)
	if err := mm.Restore(moteID); err != nil {
		return fmt.Errorf("restore mote: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Restored %s from trash\n", moteID)
	fmt.Fprintln(os.Stdout, "Note: run 'mote index rebuild' to rebuild link index")
	return nil
}

func runTrashPurge(cmd *cobra.Command, args []string) error {
	root, err := findMemoryRoot()
	if err != nil {
		return err
	}

	mm := core.NewMoteManager(root)
	cfg, err := core.LoadConfig(root)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	purged, err := mm.PurgeTrash(cfg.Trash.RetentionDays, purgeAll)
	if err != nil {
		return fmt.Errorf("purge trash: %w", err)
	}

	if len(purged) == 0 {
		fmt.Fprintln(os.Stdout, "Nothing to purge.")
	} else {
		for _, id := range purged {
			fmt.Fprintf(os.Stdout, "Permanently deleted %s\n", id)
		}
		fmt.Fprintf(os.Stdout, "Purged %d mote(s)\n", len(purged))
	}
	return nil
}

func runTrashRetention(cmd *cobra.Command, args []string) error {
	root, err := findMemoryRoot()
	if err != nil {
		return err
	}

	cfg, err := core.LoadConfig(root)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if len(args) == 0 {
		fmt.Fprintf(os.Stdout, "Trash retention: %d days\n", cfg.Trash.RetentionDays)
		return nil
	}

	days, err := strconv.Atoi(args[0])
	if err != nil || days < 0 {
		return fmt.Errorf("invalid retention days: %s", args[0])
	}

	cfg.Trash.RetentionDays = days
	if err := core.SaveConfig(root, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Trash retention set to %d days\n", days)
	return nil
}
