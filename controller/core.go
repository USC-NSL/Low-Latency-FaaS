package controller

import (
	"fmt"
)

// The abstraction of CPU core.
// |sGroups| is a chain of instances in the core. sGroup is the minimal scheduling unit and will
//           run to completion (without preemption) for a batch of packets when it is scheduled.
type Core struct {
	sGroups   []*SGroup
}

func newCore() *Core {
	core := Core{
		sGroups:   make([]*SGroup, 0),
	}
	return &core
}

func (c *Core) String() string {
	info := ""
	if len(c.sGroups) == 0 {
		return "Empty"
	}
	for _, sGroup := range c.sGroups {
		info += fmt.Sprintf("<%s> ", sGroup)
	}
	return info
}
