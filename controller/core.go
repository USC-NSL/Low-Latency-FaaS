package controller

import (
	"fmt"
)

// The abstraction of CPU core.
// |sGroups| contains all sgroups managed by this core.
// Each SGroup is a minimal scheduling unit and is run-to-completion.
type Core struct {
	coreID  int
	sGroups SGroupSlice
}

func NewCore(coreID int) *Core {
	core := Core{
		coreID:  coreID,
		sGroups: make([]*SGroup, 0),
	}
	return &core
}

func (c *Core) String() string {
	info := fmt.Sprintf("Core[%d] [", c.coreID)

	sumLoad := 0
	if len(c.sGroups) == 0 {
		info += fmt.Sprintf("Empty")
	} else {
		for _, sg := range c.sGroups {
			info += fmt.Sprintf("<%d> ", sg.ID())
			sumLoad += sg.GetLoad()
		}
	}

	info += fmt.Sprintf("] (Load=%d)", sumLoad)
	return info
}

func (c *Core) ID() int {
	return c.coreID
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
