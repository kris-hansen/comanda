package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/processor"
	"github.com/spf13/cobra"
)

var loopCmd = &cobra.Command{
	Use:   "loop",
	Short: "Manage agentic loop state",
	Long: `Manage the state of long-running agentic loops.

Commands:
  resume <loop-name>   Resume a paused or failed loop
  status               List all loop states
  status <loop-name>   Show detailed status for a specific loop
  cancel <loop-name>   Cancel a loop and delete its state
  clean                Remove completed and failed loop states`,
}

var loopResumeCmd = &cobra.Command{
	Use:   "resume <loop-name>",
	Short: "Resume a paused or failed loop",
	Long:  `Resume a paused or failed loop from its last saved state.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		loopName := args[0]

		// Get loop state directory
		stateDir, err := config.GetLoopStateDir()
		if err != nil {
			return fmt.Errorf("failed to get loop state directory: %w", err)
		}

		// Load state
		stateManager := processor.NewLoopStateManager(stateDir)
		state, err := stateManager.LoadState(loopName)
		if err != nil {
			return fmt.Errorf("failed to load loop state: %w", err)
		}

		// Check status
		if state.Status == "completed" {
			return fmt.Errorf("loop '%s' is already completed", loopName)
		}

		log.Printf("Resuming loop '%s' from iteration %d/%d", loopName, state.Iteration, state.MaxIterations)
		log.Printf("Status: %s, Last updated: %s", state.Status, formatDuration(time.Since(state.LastUpdateTime)))

		// TODO: Actually resume the loop - this requires loading and executing the workflow
		// For now, we just show the state information
		log.Printf("\nTo resume this loop, you need to re-run the workflow file that created it:")
		if state.WorkflowFile != "" {
			log.Printf("  comanda process %s", state.WorkflowFile)
		} else {
			log.Printf("  (workflow file not recorded in state)")
		}

		return nil
	},
}

var loopStatusCmd = &cobra.Command{
	Use:   "status [loop-name]",
	Short: "Show loop status",
	Long:  `Show status of all loops, or detailed status for a specific loop.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get loop state directory
		stateDir, err := config.GetLoopStateDir()
		if err != nil {
			return fmt.Errorf("failed to get loop state directory: %w", err)
		}

		stateManager := processor.NewLoopStateManager(stateDir)

		if len(args) == 0 {
			// List all loops
			states, err := stateManager.ListStates()
			if err != nil {
				return fmt.Errorf("failed to list loop states: %w", err)
			}

			if len(states) == 0 {
				log.Println("No saved loop states found")
				return nil
			}

			// Print table
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "LOOP NAME\tSTATUS\tITERATION\tELAPSED\tLAST UPDATED")
			for _, state := range states {
				elapsed := state.LastUpdateTime.Sub(state.StartTime)
				lastUpdate := time.Since(state.LastUpdateTime)
				fmt.Fprintf(w, "%s\t%s\t%d/%d\t%s\t%s\n",
					state.LoopName,
					state.Status,
					state.Iteration,
					state.MaxIterations,
					formatDuration(elapsed),
					formatDuration(lastUpdate)+" ago",
				)
			}
			w.Flush()

			return nil
		}

		// Show detailed status for specific loop
		loopName := args[0]
		state, err := stateManager.LoadState(loopName)
		if err != nil {
			return fmt.Errorf("failed to load loop state: %w", err)
		}

		// Print detailed status
		log.Printf("Loop: %s", state.LoopName)
		log.Printf("Status: %s", state.Status)
		log.Printf("Iteration: %d/%d", state.Iteration, state.MaxIterations)
		log.Printf("Started: %s", state.StartTime.Format(time.RFC3339))
		log.Printf("Last updated: %s (%s ago)", state.LastUpdateTime.Format(time.RFC3339), formatDuration(time.Since(state.LastUpdateTime)))
		log.Printf("Elapsed time: %s", formatDuration(state.LastUpdateTime.Sub(state.StartTime)))

		if state.WorkflowFile != "" {
			log.Printf("Workflow file: %s", state.WorkflowFile)
		}

		if state.ExitCondition != "" {
			log.Printf("Exit condition: %s", state.ExitCondition)
			if state.ExitPattern != "" {
				log.Printf("Exit pattern: %s", state.ExitPattern)
			}
		}

		if len(state.Variables) > 0 {
			log.Printf("\nVariables:")
			for key, value := range state.Variables {
				// Truncate long values
				displayValue := value
				if len(displayValue) > 100 {
					displayValue = displayValue[:97] + "..."
				}
				log.Printf("  %s = %s", key, displayValue)
			}
		}

		if len(state.QualityGateResults) > 0 {
			log.Printf("\nQuality Gate Results (last iteration):")
			for _, result := range state.QualityGateResults {
				status := "✓ PASS"
				if !result.Passed {
					status = "✗ FAIL"
				}
				log.Printf("  %s %s (attempts: %d, duration: %s)",
					status,
					result.GateName,
					result.Attempts,
					result.Duration,
				)
				if !result.Passed && result.Message != "" {
					log.Printf("    Message: %s", result.Message)
				}
			}
		}

		if len(state.History) > 0 {
			log.Printf("\nRecent iterations (last 5):")
			start := len(state.History) - 5
			if start < 0 {
				start = 0
			}
			for _, iter := range state.History[start:] {
				log.Printf("  Iteration %d (%s):", iter.Index, iter.Timestamp.Format("15:04:05"))
				// Truncate output
				output := strings.TrimSpace(iter.Output)
				if len(output) > 200 {
					output = output[:197] + "..."
				}
				log.Printf("    %s", output)
			}
		}

		return nil
	},
}

var loopCancelCmd = &cobra.Command{
	Use:   "cancel <loop-name>",
	Short: "Cancel a loop and delete its state",
	Long:  `Cancel a running or paused loop and delete its saved state.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		loopName := args[0]

		// Get loop state directory
		stateDir, err := config.GetLoopStateDir()
		if err != nil {
			return fmt.Errorf("failed to get loop state directory: %w", err)
		}

		// Delete state
		stateManager := processor.NewLoopStateManager(stateDir)
		if err := stateManager.DeleteState(loopName); err != nil {
			return fmt.Errorf("failed to delete loop state: %w", err)
		}

		log.Printf("Loop '%s' cancelled and state deleted", loopName)
		return nil
	},
}

var loopCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove completed and failed loop states",
	Long:  `Remove all completed and failed loop states, keeping only running and paused loops.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get loop state directory
		stateDir, err := config.GetLoopStateDir()
		if err != nil {
			return fmt.Errorf("failed to get loop state directory: %w", err)
		}

		stateManager := processor.NewLoopStateManager(stateDir)

		// List all states
		states, err := stateManager.ListStates()
		if err != nil {
			return fmt.Errorf("failed to list loop states: %w", err)
		}

		// Delete completed and failed states
		deleted := 0
		for _, state := range states {
			if state.Status == "completed" || state.Status == "failed" {
				if err := stateManager.DeleteState(state.LoopName); err != nil {
					log.Printf("Warning: failed to delete state for loop '%s': %v", state.LoopName, err)
				} else {
					log.Printf("Deleted %s loop: %s", state.Status, state.LoopName)
					deleted++
				}
			}
		}

		if deleted == 0 {
			log.Println("No completed or failed loop states to clean")
		} else {
			log.Printf("Cleaned %d loop state(s)", deleted)
		}

		return nil
	},
}

func init() {
	// Add subcommands
	loopCmd.AddCommand(loopResumeCmd)
	loopCmd.AddCommand(loopStatusCmd)
	loopCmd.AddCommand(loopCancelCmd)
	loopCmd.AddCommand(loopCleanCmd)

	// Register with root command
	rootCmd.AddCommand(loopCmd)
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd %dh", days, hours)
}
