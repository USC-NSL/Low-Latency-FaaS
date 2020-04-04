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
		{Text: "nodes", Description: "List all nodes."},
		{Text: "workers", Description: "List all workers."},
		{Text: "add", Description: "Create a NF instance on specific node."},
		{Text: "connect [up] [down]", Description: "Connects two logical NFs"},
		{Text: "rm", Description: "Delete a NF instance on specific node."},
		{Text: "kubectl", Description: "Control kubernetes clusters."},
		{Text: "quit", Description: "Clean up and quit the controller."},
	}

	// |FilterHasPrefix| checks whether the completion.Text begins with sub.
	return prompt.FilterHasPrefix(s, in.GetWordBeforeCursor(), true)
}
