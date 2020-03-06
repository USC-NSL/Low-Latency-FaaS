
package controller

// The abstraction of CPU core.
// |instances| are the NF instances assigned on the core. Each instance will be assigned to a sGroup.
// |sGroups| is a chain of instances in the core. sGroup is the minimal scheduling unit and will
//           run to completion (without preemption) for a batch of packets when it is scheduled.
type Core struct {
	instances []*Instance
	sGroups []*SGroup
}

func newCore() *Core {
	core := Core{
		instances: make([]*Instance, 0),
		sGroups: make([]*SGroup, 0),
	}
	return &core
}
