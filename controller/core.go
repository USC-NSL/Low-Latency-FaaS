package controller

import (
	"fmt"

	glog "github.com/golang/glog"
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
			sumLoad += sg.GetPktLoad()
		}
	}

	info += fmt.Sprintf("] (Load=%d)", sumLoad)
	return info
}

func (c *Core) ID() int {
	return c.coreID
}

// Attach a SGroup |sgroup| on the core.
func (c *Core) attachSGroup(sgroup *SGroup) {
	c.sGroups = append(c.sGroups, sgroup)
}

// Detach a SGroup |sgroup| on the core.
func (c *Core) detachSGroup(sgroup *SGroup) {
	for i, sg := range c.sGroups {
		if sg.ID() == sgroup.ID() {
			glog.Infof("SGroup[%d]", sgroup.ID())
			c.sGroups = append(c.sGroups[:i], c.sGroups[i+1:]...)
			return
		}
	}

	glog.Errorf("SGroup[%d]", sgroup.ID())
}
