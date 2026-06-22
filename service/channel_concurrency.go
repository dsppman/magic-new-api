package service

import (
	"sync"
	"sync/atomic"
)

// channelInFlight tracks the number of in-flight relay attempts per channel id.
// It is a process-local gauge used for per-channel concurrency limiting; in a
// multi-instance deployment the effective limit applies per instance.
var channelInFlight sync.Map // map[int]*int64

func channelInFlightCounter(channelId int) *int64 {
	// Load-first avoids allocating a throwaway *int64 on the common hit path,
	// since LoadOrStore would evaluate new(int64) on every call.
	if v, ok := channelInFlight.Load(channelId); ok {
		return v.(*int64)
	}
	v, _ := channelInFlight.LoadOrStore(channelId, new(int64))
	return v.(*int64)
}

// AcquireChannelConcurrencySlot reserves one in-flight slot for the channel.
//
// When limit <= 0 the channel is treated as unlimited: the call always succeeds
// and the returned release is a no-op. Otherwise it atomically reserves a slot
// and returns ok=false (holding no slot) when the channel is already at its
// limit. The returned release frees the slot exactly once and is safe to call
// from a defer; callers MUST invoke it when the attempt finishes.
func AcquireChannelConcurrencySlot(channelId int, limit int) (release func(), ok bool) {
	if limit <= 0 {
		return func() {}, true
	}
	counter := channelInFlightCounter(channelId)
	if atomic.AddInt64(counter, 1) > int64(limit) {
		atomic.AddInt64(counter, -1)
		return nil, false
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			atomic.AddInt64(counter, -1)
		})
	}, true
}

// ChannelInFlight returns the current in-flight count for a channel.
func ChannelInFlight(channelId int) int64 {
	return atomic.LoadInt64(channelInFlightCounter(channelId))
}
