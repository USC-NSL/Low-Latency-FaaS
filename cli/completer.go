
package cli

import (
	prompt "github.com/c-bata/go-prompt"
)

func Complete(in prompt.Document) []prompt.Suggest {
	if in.TextBeforeCursor() == "" {
		return []prompt.Suggest{}
	}

	s := []prompt.Suggest{
		{Text: "pods", Description: "List all pods."},
		{Text: "deps", Description: "List all deployments."},
		{Text: "nodes", Description: "List all worker nodes."},
		{Text: "add", Description: "Add a function without any associated traffic."},
		{Text: "update", Description: "Update the traffic volume of a function."},
	}

	// |FilterHasPrefix| checks whether the completion.Text begins with sub.
	return prompt.FilterHasPrefix(s, in.GetWordBeforeCursor(), true)
}
