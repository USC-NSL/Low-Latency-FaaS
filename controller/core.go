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
		activeSG := ""
		idleSG := ""
		for _, sg := range c.sGroups {
			if sg.IsSched() {
				activeSG += fmt.Sprintf("<%d> ", sg.ID())
				sumLoad += sg.GetPktLoad()
			} else {
				idleSG += fmt.Sprintf("%d ", sg.ID())
			}
		}
		info += "A: " + activeSG + "| I: " + idleSG
	}

	info += fmt.Sprintf("] (Load=%d)", sumLoad)
	return info
}

func (c *Core) ID() int {
	return c.coreID
}

// Add a new SGroup to be managed this |core|. The SGroup may either
// be active or idle. Note: do not add duplicate SGroups to a Core.
func (c *Core) addSGroup(sgroup *SGroup) {
	for _, sg := range c.sGroups {
		if sg.ID() == sgroup.ID() {
			glog.Errorf("SGroup[%d] is duplicate on Core[%d]", sgroup.ID(), c.coreID)
			return
		}
	}

	// |sgroup| does not present on Core |c|.
	c.sGroups = append(c.sGroups, sgroup)
}

// Removes an existing SGroup from this |core|. The SGroup is temporarily not
// managed by any CPU cores.
func (c *Core) removeSGroup(sgroup *SGroup) {
	// Do not remove a SGroup that is not managed by this core.
	if sgroup.GetCoreID() != c.coreID {
		glog.Errorf("SGroup[%d] is not managed by Core[%d]", sgroup.ID(), c.coreID)
		return
	}

	for i, sg := range c.sGroups {
		if sg.ID() == sgroup.ID() {
			c.sGroups = append(c.sGroups[:i], c.sGroups[i+1:]...)
			return
		}
	}

	glog.Errorf("SGroup[%d] is not found on Core[%d]", sgroup.ID(), c.coreID)
}
