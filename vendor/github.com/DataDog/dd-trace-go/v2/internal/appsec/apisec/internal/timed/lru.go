// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package timed

import (
	"fmt"
	"math"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/DataDog/dd-trace-go/v2/internal/appsec/apisec/internal/config"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

// capacity is the maximum number of items that may be temporarily present in a
// [LRU]. An eviction triggers once [config.MaxItemCount] is reached, however the
// implementation is based on Copy-Update-Replace semantics, so during a table
// rebuild, the old table may contrinue to receive items for a short while.
const capacity = 2 * config.MaxItemCount

// LRU is a specialized open-addressing-hash-table-based implementation of a
// specialized LRU cache, using Copy-Update-Replace semantics to operate in a
// completely lock-less manner.
type LRU struct {
	// table is the pointer to the current backing hash table
	table atomic.Pointer[table]
	// clock is used to determine the current timestamp when making
	// changes
	clock biasedClock
	// intervalSeconds is the amount of time in seconds that an entry is
	// considered live for.
	intervalSeconds uint32
	// zeroKey is a key that is used to replace 0 in the set. This key and 0 are
	// effectively the same item. This allows us to gracefully handle 0 in our
	// use-case without having to half the hash-space (to 63 bits) so we can use
	// one bit as an empty discriminator. The value is chosen at random when the
	// set is created, so that different instances will merge 0 with a different
	// key.
	zeroKey uint64
	// rebuilding is a flag to indicate whether the table is being rebuilt as
	// part of an eviction request.
	rebuilding atomic.Bool
}

// NewLRU initializes a new, empty [LRU] with the given interval and clock
// function. A warning will be logged if it is set below 1 second. Panics if
// the interval is more than [math.MaxUint32] seconds, as this value cannot be
// used internally.
//
// Note: timestamps are stored at second resolution, so the interval will be
// rounded down to the nearest second.
func NewLRU(interval time.Duration) *LRU {
	if interval < time.Second {
		log.Debug("NewLRU: interval is less than one second; this should not be attempted in production (value: %s)", interval)
	}
	if interval > time.Second*math.MaxUint32 {
		panic(fmt.Errorf("NewLRU: interval must be <= %s, but was %s", time.Second*math.MaxUint32, interval))
	}

	intervalSeconds := uint32(interval.Seconds())
	set := &LRU{
		clock:           newBiasedClock(intervalSeconds),
		intervalSeconds: intervalSeconds,
		zeroKey:         rand.Uint64(),
	}

	// That value cannot be zero...
	for set.zeroKey == 0 {
		set.zeroKey = rand.Uint64()
	}

	set.table.Store(&table{})

	return set
}

// Hit determines whether the given key should be kept or dropped based on the
// last time it was sampled. If the table grows larger than [config.MaxItemCount], the
// [LRU.rebuild] method is called in a separate goroutine to begin the
// eviction process. Until this has completed, all updates to the [LRU] are
// effectively dropped, as they happen on the soon-to-be-replaced table.
//
// Note: in order to run completely lock-less, [LRU] cannot store the 0 key in
// the table, as a 0 key is used as a sentinel value to identify free entries.
// To avoid this pitfall, [LRU.zeroKey] is used as a substitute for 0, meaning
// 0 and [LRU.zeroKey] are treated as the same key. This is not an issue in
// common use, as given a uniform distribution of keys this only happens 1 in
// 2^64-1 times.
func (m *LRU) Hit(key uint64) bool {
	if key == 0 {
		// The 0 key is used as a way to imply a slot is empty; so we cannot store
		// it in the table. To address this, when passed a 0 key, we will use the
		// [Set.zeroKey] as a substitute.
		key = m.zeroKey
	}

	now := m.clock.Now()
	threshold := now - m.intervalSeconds

	var (
		table = m.table.Load()
		entry *entry
	)
	for {
		var exists bool
		entry, exists = table.FindEntry(key)
		if exists {
			// The entry already exists, so we can proceed...
			break
		}

		// We're adding a new entry to the table, so we need to:
		// 1. Ensure we have capacity (possibly trigger an eviction rebuild)
		// 2. Claim the slot (or look for another slot if it's already claimed)
		newCount := table.count.Add(1)
		if newCount > config.MaxItemCount && m.rebuilding.CompareAndSwap(false, true) {
			// We're already holding the maximium number of items, so we will rebuild
			// in order to perform an eviction pass. Updates made in the meantime will
			// be lost.
			go m.rebuild(table, threshold)
		}
		if newCount > capacity {
			// We don't have space to add any new item, so we'll ignore this and
			// decide to DROP it (we may otherwise cause a surge of inconditional
			// keep decisions, that is not desirable). This only happens in the most
			// dire of circumstances (a table rebuild did not complete fast enough
			// to make up free space).
			table.count.Add(-1)
			return false
		}

		if entry.Key.CompareAndSwap(0, key) {
			// We have successfully claimed the slot, so now we can proceed to set it
			// up. If we fail to swap, another goroutine has sampled this slot just
			// before this one, so we can DROP the sample.
			return entry.Data.CompareAndSwap(0, newEntryData(now, now))
		}

		if entry.Key.Load() == key {
			// Another goroutine has concurrently claimed this slot for this key, and
			// since very little time has passed since then, so we can DROP this
			// sample... This is extremely unlikely to happen (and nearly impossible
			// to reliably cover in unit tests).
			return false
		}

		// Another goroutine has concurrently claimed this slot for another key...
		// We will try to find another slot then...
		table.count.Add(-1)
	}

	// We have found an existing entry, so we can proceed to update it...
	curData := entry.Data.Load()
	if curData.SampleTime() <= threshold {
		// We sampled this a while back (or this is the first time), so we may keep
		// this sample!

		// Store the value ahead of the for loop so we don't have to do the bit
		// shifts over and over again (even though they're cheap to do).
		nowEntryData := newEntryData(now, now)
		for !entry.Data.CompareAndSwap(curData, nowEntryData) {
			// Another goroutine has already changed it...
			curData = entry.Data.Load()
			if curData.LastAccessKept() {
				// The concurrent update was a KEEP (as is indicated by the fact its
				// atime and stime are equal), so this one is necessarily a DROP.
				return false
			}

			if curData.SampleTime() >= now {
				// The concurrent update was made in our future, and it somehow was not
				// a KEEP, so we'll make a KEEP decision here, but avoid rolling back
				// the [entryData.AccessTime] back.
				return true
			}

			// The concurrent update was a DROP, and our clock is ahead of theirs, so
			// we'll try again...
		}

		// We successfully swapped at this point, so we have our KEEP decision!
		return true
	}

	newData := curData.WithAccessTime(now)
	for curData.AccessTime() < now {
		if entry.Data.CompareAndSwap(curData, newData) {
			// We are done here!
			break
		}
		// Another goroutine has updated the access time... We'll try again...
		curData = entry.Data.Load()
	}
	return false
}

// rebuild runs in a separate goroutine, and creates a pruned copy of the
// provided [table] with old and expired entries removed. It will keep at most
// [config.MaxItemCount]*2/3 items in the new table. Once the rebuild is complete, it
// replaces the [LRU.table] with the copy.
func (m *LRU) rebuild(oldTable *table, threshold uint32) {
	// Since Go has a GC, we can "just" replace the current [Set.table] with a
	// trimmed down copy, and let the GC take care of reclaiming the old one, once
	// it is no longer in use by any reader.
	m.table.Store(oldTable.PrunedCopy(threshold))
	m.rebuilding.Store(false)
}
