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
		{Text: "add [nodeName] [funcType1] [funcType2] ...", Description: "Create a sGroup on a node by a list of NFs."},
		{Text: "rm [nodeName] [groupId]", Description: "Remove a free sGroup on a node."},
		{Text: "attach [nodeName] [groupId] [coreId]", Description: "Attach a sGroup to a core."},
		{Text: "detach [nodeName] [groupId] [coreId]", Description: "Detach a sGroup from a core."},
		{Text: "kubectl rm [deploymentName]", Description: "Destroy a deployment in kubernetes by its name."},
		{Text: "flow [srcIp] [srcPort] [dstIp] [dstPort] [protocol]", Description: "Simulate a flow coming to the system."},
		{Text: "deploy [user] [nf]", Description: "Adds a logical NF to |user|'s' NF DAG"},
		{Text: "connect [user] [up] [down]", Description: "Connects two logical NFs"},
		{Text: "kubectl", Description: "Control kubernetes clusters."},
		{Text: "quit", Description: "Clean up and quit the controller."},
	}

	// |FilterHasPrefix| checks whether the completion.Text begins with sub.
	return prompt.FilterHasPrefix(s, in.GetWordBeforeCursor(), true)
}
