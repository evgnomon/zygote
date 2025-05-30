package utils

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// GetYesNoInput prompts the user for a yes/no response and returns a boolean
func GetYesNoInput(prompt string) bool {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("%s (yes/no): ", prompt)
	for {
		scanner.Scan()
		input := strings.ToLower(strings.TrimSpace(scanner.Text()))

		switch input {
		case "yes", "y":
			return true
		case "no", "n":
			return false
		default:
			fmt.Printf("Invalid input; %s (yes/no): ", prompt)
		}
	}
}
