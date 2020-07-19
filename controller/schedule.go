package controller

import (
	"sort"
	"time"

	glog "github.com/golang/glog"
)

// This is the place to implement CPU scheduling.
// NF chains are scheduled periodically based on real-time monitoring.

// Go routine that runs on each worker to rebalance traffic loads among
// available CPU cores. Select a scheduling algorithm by changing the
// per-worker scheduling function here.
func (w *Worker) ScheduleLoop() {
	for {
		select {
		case <-w.schedOp:
			w.wg.Done()
			return
		default:
			w.noPackingScheduleOnce()
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// Returns an idle CPU core |core| in |w.cores|. The CPU core does not
// run any NF chains. This function is only called by other per-worker
// functions. So, no lock as other functions must lock first.
func (w *Worker) getIdleCore() *Core {
	for _, core := range w.cores {
		if len(core.sGroups) == 0 {
			return core
		}
	}

	return nil
}

// The per-worker NF thread scheduler.
// This function monitors traffic loads for all deployed SGroups.
// It packs SGroups into a minimum number of CPU cores.
// Algorithm: Best Fit Decreasing.
func (w *Worker) scheduleOnce() {
	// Stops all updates on Worker |w| temporally.
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	sort.Sort(sort.Reverse(w.sgroups))

	coreID := 0
	load := 80

	for _, sg := range w.sgroups {
		if !sg.IsReady() {
			// Skips if |sg| is not ready for scheduling.
			continue
		} else if !sg.IsActive() {
			// Detaches |sg| if it is still being scheduled.
			if sg.IsSched() {
				if sg.GetCoreID() != 1 {
					if err := sg.attachSGroup(1); err != nil {
						glog.Errorf("Failed to attach SGroup[%d] to Core #1. %v", sg.ID(), err)
						continue
					}

					// |sg| should be attached successfully.
					if sg.GetCoreID() != 1 || !sg.IsSched() {
						glog.Errorf("SGroup[%d] was Attached to Core #1 but not running on it!", sg.ID())
					}
				}

				if err := sg.detachSGroup(); err != nil {
					glog.Errorf("Failed to detach SGroup[%d]. %v", sg.ID(), err)
					continue
				}

				// |sg| should be detached successfully.
				if sg.IsSched() {
					glog.Errorf("SGroup[%d] was Detached but still running!", sg.ID())
				}
			}

			continue
		}

		sgLoad := sg.GetPktLoad()

		if load+sgLoad < 80 {
			load = load + sgLoad
		} else if coreID < len(w.cores) {
			coreID += 1
			load = sgLoad
		} else {
			glog.Errorf("Worker[%s] runs out of cores", w.name)
			break
		}

		// Enforce scheduling.
		// Note: be careful about deadlocks.
		if err := sg.attachSGroup(coreID); err != nil {
			glog.Errorf("Failed to attach SGroup[%d] to Core[%d]", sg.ID(), coreID)
		}
	}
}

// Algorithm:
// Best-Fit Decreasing for non-overloaded SGroups.
// Rescheduling for overloaded SGroups.
func (w *Worker) advancedFitScheduleOnce() {
	// Stops all updates on Worker |w| temporally.
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	sort.Sort(sort.Reverse(w.sgroups))

	coreID := 0
	load := 80

	for _, sg := range w.sgroups {
		if !sg.IsReady() {
			// Skips if |sg| is not ready for scheduling.
			continue
		} else if !sg.IsActive() {
			// Detaches |sg| if it is still being scheduled.
			if sg.IsSched() {
				if sg.GetCoreID() != 1 {
					if err := sg.attachSGroup(1); err != nil {
						glog.Errorf("Failed to attach SGroup[%d] to Core #1. %v", sg.ID(), err)
						continue
					}

					// |sg| should be attached successfully.
					if sg.GetCoreID() != 1 || !sg.IsSched() {
						glog.Errorf("SGroup[%d] was Attached to Core #1 but not running on it!", sg.ID())
					}
				}

				if err := sg.detachSGroup(); err != nil {
					glog.Errorf("Failed to detach SGroup[%d]. %v", sg.ID(), err)
					continue
				}

				// |sg| should be detached successfully.
				if sg.IsSched() {
					glog.Errorf("SGroup[%d] was Detached but still running!", sg.ID())
				}
			}

			continue
		}

		sgLoad := sg.GetPktLoad()

		if load+sgLoad < 80 {
			load = load + sgLoad
		} else if coreID < len(w.cores) {
			coreID += 1
			load = sgLoad
		} else {
			glog.Errorf("Worker[%s] runs out of cores", w.name)
			break
		}

		// Enforce scheduling.
		// Note: be careful about deadlocks.
		if err := sg.attachSGroup(coreID); err != nil {
			glog.Errorf("Failed to attach SGroup[%d] to Core[%d]", sg.ID(), coreID)
		}
	}
}

func (w *Worker) noPackingScheduleOnce() {
	// Stops all updates on Worker |w| temporally.
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	for _, sg := range w.sgroups {
		if !sg.IsReady() {
			// Skips if |sg| is not ready for scheduling.
			continue
		} else if !sg.IsActive() {
			// Detaches |sg| if it is still being scheduled.
			if sg.IsSched() {
				if sg.GetCoreID() != 1 {
					if err := sg.attachSGroup(1); err != nil {
						glog.Errorf("Failed to attach SGroup[%d] to Core #1. %v", sg.ID(), err)
						continue
					}

					// |sg| should be attached successfully.
					if sg.GetCoreID() != 1 || !sg.IsSched() {
						glog.Errorf("SGroup[%d] was Attached to Core #1 but not running on it!", sg.ID())
					}
				}

				if err := sg.detachSGroup(); err != nil {
					glog.Errorf("Failed to detach SGroup[%d]. %v", sg.ID(), err)
					continue
				}

				// |sg| should be detached successfully.
				if sg.IsSched() {
					glog.Errorf("SGroup[%d] was Detached but still running!", sg.ID())
				}
			}

			continue
		}

		// |sg| is ready and active. Schedule the sg with one idle CPU core.
		if !sg.IsSched() {
			core := w.getIdleCore()
			if core == nil {
				glog.Errorf("Worker[%s] runs out of cores", w.name)
				continue
			}

			// Enforce scheduling.
			// Note: be careful about deadlocks.
			if err := sg.attachSGroup(core.coreID); err != nil {
				glog.Errorf("Failed to attach SGroup[%d] to Core[%d]", sg.ID(), core.coreID)
			}
		}
	}
}
