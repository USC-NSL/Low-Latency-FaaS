package controller

import (
	"fmt"
)

// The abstraction of CPU core.
// |sGroups| contains all sgroups managed by this core.
// Each SGroup is a minimal scheduling unit and is run-to-completion.
type Core struct {
	sGroups []*SGroup
}

func newCore() *Core {
	core := Core{
		sGroups: make([]*SGroup, 0),
	}
	return &core
}

func (c *Core) String() string {
	info := ""
	if len(c.sGroups) == 0 {
		return "Empty"
	}
	for _, sg := range c.sGroups {
		info += fmt.Sprintf("<%s> ", sg)
	}
	return info
}

// Attach a SGroup |sg| on the core.
func (c *Core) attachSGroup(sg *SGroup) {
	c.sGroups = append(c.sGroups, sg)
}

// Detach sGroup with |groupId| on the core.
func (c *Core) detachSGroup(groupId int) *SGroup {
	for i, sg := range c.sGroups {
		if sg.ID() == groupId {
			c.sGroups = append(c.sGroups[:i], c.sGroups[i+1:]...)
			return sg
		}
	}
	return nil
}
