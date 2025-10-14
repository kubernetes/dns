// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package internal

import (
	"sync"
	"time"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

type TickFunc func()

type Ticker struct {
	ticker *time.Ticker

	tickSpeedMu sync.Mutex
	tickSpeed   time.Duration

	interval Range[time.Duration]

	tickFunc TickFunc

	stopChan chan struct{}
	stopped  bool
}

func NewTicker(tickFunc TickFunc, interval Range[time.Duration]) *Ticker {
	ticker := &Ticker{
		ticker:    time.NewTicker(interval.Max),
		tickSpeed: interval.Max,
		interval:  interval,
		tickFunc:  tickFunc,
		stopChan:  make(chan struct{}),
	}

	go func() {
		for {
			select {
			case <-ticker.ticker.C:
				tickFunc()
			case <-ticker.stopChan:
				return
			}
		}
	}()

	return ticker
}

func (t *Ticker) CanIncreaseSpeed() {
	t.tickSpeedMu.Lock()
	defer t.tickSpeedMu.Unlock()

	oldTickSpeed := t.tickSpeed
	t.tickSpeed = t.interval.Clamp(t.tickSpeed / 2)

	if oldTickSpeed == t.tickSpeed {
		return
	}

	log.Debug("telemetry: increasing flush speed to an interval of %s", t.tickSpeed)
	t.ticker.Reset(t.tickSpeed)
}

func (t *Ticker) CanDecreaseSpeed() {
	t.tickSpeedMu.Lock()
	defer t.tickSpeedMu.Unlock()

	oldTickSpeed := t.tickSpeed
	t.tickSpeed = t.interval.Clamp(t.tickSpeed * 2)

	if oldTickSpeed == t.tickSpeed {
		return
	}

	log.Debug("telemetry: decreasing flush speed to an interval of %s", t.tickSpeed)
	t.ticker.Reset(t.tickSpeed)
}

func (t *Ticker) Stop() {
	if t.stopped {
		return
	}
	t.ticker.Stop()
	t.stopChan <- struct{}{}
	close(t.stopChan)
	t.stopped = true
}
