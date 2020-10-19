package controller

import (
	"fmt"
	"os"
	"sync"
	"time"

	glog "github.com/golang/glog"
)

const (
	kMeasurementDurationMS = 500
)

type snapshot struct {
	ts      time.Time
	coreCnt int64
	pktRate int64
}

type FaaSLogger struct {
	ctl           *FaaSController
	snapshotStore []*snapshot
	// The current log info
	logOn        bool
	logIndex     int
	logStartTime time.Time
	// Key metrics
	testDuration time.Duration
	avgCoreUsage float64
	maxCoreUsage int64
	idleCounter  int
	state        int
	sgMutex      sync.Mutex
}

func NewFaaSLogger(c *FaaSController) *FaaSLogger {
	return &FaaSLogger{
		ctl:           c,
		snapshotStore: make([]*snapshot, 0),
		logOn:         false,
		logIndex:      0,
		logStartTime:  time.Now(),
		testDuration:  0,
		avgCoreUsage:  float64(0),
		maxCoreUsage:  int64(0),
		idleCounter:   0,
		state:         0,
	}
}

func (l *FaaSLogger) RunFaaSLogger() {
	l.sgMutex.Lock()
	l.state = 0
	l.sgMutex.Unlock()

	for {
		s := l.ctl.getSnapshotSummary()

		if !l.logOn {
			if s.coreCnt > 0 || s.pktRate > 0 {
				glog.Infof("Start logging at faas_%d.log", l.logIndex)

				l.logStartTime = time.Now()
				l.logOn = true
				// Reset the snapshot store.
				l.snapshotStore = nil
				l.avgCoreUsage = 0
				l.maxCoreUsage = 0
				l.idleCounter = 0
			}
		}

		if l.logOn {
			l.snapshotStore = append(l.snapshotStore, s)

			if s.coreCnt == 0 && s.pktRate == 0 {
				l.idleCounter += 1
			} else {
				l.idleCounter = 0
			}
			if l.idleCounter >= 5 { // End one measurement period.
				l.snapshotStore = l.snapshotStore[:len(l.snapshotStore)-5]
				l.ComputeKeyMetrics()
				l.WriteResultsToFile()

				glog.Infof("Stop logging at faas_%d.log", l.logIndex)
				l.logIndex += 1
				l.logOn = false
			}
		}

		time.Sleep(kMeasurementDurationMS * time.Millisecond)

		l.sgMutex.Lock()
		if l.state == 1 {
			l.state = 2
			l.sgMutex.Unlock()
			break
		}
		l.sgMutex.Unlock()
	}
}

func (l *FaaSLogger) StopFaaSLogger() {
	l.sgMutex.Lock()
	l.state = 1
	l.sgMutex.Unlock()

	for {
		l.sgMutex.Lock()
		if l.state == 2 {
			l.sgMutex.Unlock()
			break
		}
		l.sgMutex.Unlock()
	}
}

func (l *FaaSLogger) WriteResultsToFile() error {
	logFile := fmt.Sprintf("/tmp/faas_%d.log", l.logIndex)

	f, err := os.Create(logFile)
	if err != nil {
		return fmt.Errorf("Failed to open %s\n", logFile)
	}
	defer f.Close()

	fmt.Fprintf(f, "Test %d\n", l.logIndex)
	fmt.Fprintf(f, "Duration = %d millisecond\n", int64(l.testDuration/time.Millisecond))
	fmt.Fprintf(f, "avg cores = %v\n", l.avgCoreUsage)
	fmt.Fprintf(f, "max cores = %d\n", l.maxCoreUsage)
	fmt.Fprintf(f, "-----------------------\n")
	fmt.Fprintf(f, "------- Timeline ------\n")
	fmt.Fprintf(f, "-----------------------\n")

	start := l.snapshotStore[0].ts
	for _, s := range l.snapshotStore {
		fmt.Fprintf(f, "ts = %d, core = %d, rate: %d\n", int64(s.ts.Sub(start)), s.coreCnt, s.pktRate)
	}
	return nil
}

func (l *FaaSLogger) ComputeKeyMetrics() {
	if !l.logOn || len(l.snapshotStore) < 10 {
		return
	}

	start := l.snapshotStore[0].ts
	end := l.snapshotStore[len(l.snapshotStore)-1].ts
	l.testDuration = end.Sub(start)

	// |sumCoreTime| is the sum of CPU core * time in (core * 100 * millisecond).
	sumCoreTime := int64(0)
	prev := l.snapshotStore[0]
	for _, curr := range l.snapshotStore {
		if curr.coreCnt > l.maxCoreUsage {
			l.maxCoreUsage = curr.coreCnt
		}
		sumCoreTime += int64((curr.ts.Sub(prev.ts))/time.Millisecond) * curr.coreCnt
		prev = curr
	}

	l.avgCoreUsage = float64(sumCoreTime) / float64(l.testDuration/time.Millisecond)
}

// Measurement functions at the FaaSController.
func (c *FaaSController) getSnapshotSummary() *snapshot {
	activeCores := 0
	pktRate := 0
	for _, w := range c.workers {
		c, p := w.getPerWorkerSnapshotSummary()
		activeCores += c
		pktRate += p
	}

	return &snapshot{
		ts:      time.Now(),
		coreCnt: int64(activeCores),
		pktRate: int64(pktRate),
	}
}

// Measurement functions at the Worker.
func (w *Worker) getPerWorkerSnapshotSummary() (int, int) {
	// Stops all updates on Worker |w| temporally.
	w.sgMutex.Lock()
	defer w.sgMutex.Unlock()

	currCores := 0
	currPktRate := 0

	for _, sg := range w.sgroups {
		if sg.IsSched() && sg.IsActive() {
			currCores += 1
			currPktRate += sg.GetPktRate()
		}
	}
	return currCores, currPktRate
}
