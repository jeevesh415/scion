// Package hubsync provides Hub synchronization checks for agent operations.
package hubsync

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ConfirmAction prompts user for Y/n confirmation.
// Returns true if confirmed, false otherwise.
// If autoConfirm is true, returns defaultYes without prompting.
func ConfirmAction(prompt string, defaultYes bool, autoConfirm bool) bool {
	if autoConfirm {
		return defaultYes
	}

	suffix := " (Y/n): "
	if !defaultYes {
		suffix = " (y/N): "
	}

	fmt.Print(prompt + suffix)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		// On error, return the default
		return defaultYes
	}

	input = strings.TrimSpace(strings.ToLower(input))

	// Empty input returns the default
	if input == "" {
		return defaultYes
	}

	return input == "y" || input == "yes"
}

// ShowSyncPlan displays what will be synced and asks for confirmation.
// Returns true if the user confirms, false otherwise.
func ShowSyncPlan(result *SyncResult, autoConfirm bool) bool {
	if result.IsInSync() {
		return true // Nothing to sync
	}

	fmt.Println()
	fmt.Println("Hub Agent Sync Required")
	fmt.Println("=======================")

	if len(result.ToRegister) > 0 {
		fmt.Println("Agents to register on Hub:")
		for _, name := range result.ToRegister {
			fmt.Printf("  + %s\n", name)
		}
	}

	if len(result.ToRemove) > 0 {
		fmt.Println("Agents to remove from Hub (not on this host):")
		for _, ref := range result.ToRemove {
			fmt.Printf("  - %s\n", ref.Name)
		}
	}

	// Show pending agents for visibility (they don't require action)
	if len(result.Pending) > 0 {
		fmt.Println()
		fmt.Println("Agents pending on Hub (awaiting start):")
		for _, ref := range result.Pending {
			fmt.Printf("  ~ %s\n", ref.Name)
		}
	}

	fmt.Println()
	return ConfirmAction("Proceed with sync?", true, autoConfirm)
}

// ShowRegistrationPrompt displays the grove registration prompt.
// Returns true if the user confirms, false otherwise.
func ShowRegistrationPrompt(groveName string, autoConfirm bool) bool {
	fmt.Println()
	fmt.Printf("Grove '%s' is not registered with the Hub.\n", groveName)
	return ConfirmAction("Register grove with Hub?", true, autoConfirm)
}

// ShowInitRegistrationPrompt displays the post-init registration prompt.
// Returns true if the user confirms, false otherwise.
func ShowInitRegistrationPrompt(autoConfirm bool) bool {
	return ConfirmAction("Grove initialized. Register with Hub?", true, autoConfirm)
}

// GroveChoice represents the user's choice when matching groves exist.
type GroveChoice int

const (
	// GroveChoiceCancel means the user cancelled the operation.
	GroveChoiceCancel GroveChoice = iota
	// GroveChoiceLink means the user chose to link to an existing grove.
	GroveChoiceLink
	// GroveChoiceRegisterNew means the user chose to register a new grove.
	GroveChoiceRegisterNew
)

// GroveMatch holds information about a matching grove for display.
type GroveMatch struct {
	ID        string
	Name      string
	GitRemote string
}

// ShowMatchingGrovesPrompt displays matching groves and asks the user to choose.
// Returns the choice and the selected grove ID if linking.
func ShowMatchingGrovesPrompt(groveName string, matches []GroveMatch, autoConfirm bool) (GroveChoice, string) {
	fmt.Println()
	fmt.Printf("Found %d existing grove(s) with the name '%s' on the Hub:\n", len(matches), groveName)
	fmt.Println()

	for i, m := range matches {
		if m.GitRemote != "" {
			fmt.Printf("  [%d] %s (ID: %s, remote: %s)\n", i+1, m.Name, m.ID, m.GitRemote)
		} else {
			fmt.Printf("  [%d] %s (ID: %s)\n", i+1, m.Name, m.ID)
		}
	}
	fmt.Printf("  [%d] Register as a new grove (duplicate name)\n", len(matches)+1)
	fmt.Println()

	if autoConfirm {
		// Auto-confirm defaults to linking to the first match
		fmt.Printf("Auto-linking to: %s (ID: %s)\n", matches[0].Name, matches[0].ID)
		return GroveChoiceLink, matches[0].ID
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Enter choice (or 'c' to cancel): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return GroveChoiceCancel, ""
		}

		input = strings.TrimSpace(strings.ToLower(input))
		if input == "c" || input == "cancel" {
			return GroveChoiceCancel, ""
		}

		choice := 0
		if _, err := fmt.Sscanf(input, "%d", &choice); err != nil {
			fmt.Println("Invalid choice. Please enter a number.")
			continue
		}

		if choice < 1 || choice > len(matches)+1 {
			fmt.Printf("Invalid choice. Please enter 1-%d.\n", len(matches)+1)
			continue
		}

		if choice == len(matches)+1 {
			return GroveChoiceRegisterNew, ""
		}

		return GroveChoiceLink, matches[choice-1].ID
	}
}
