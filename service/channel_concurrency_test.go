package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcquireChannelConcurrencySlotUnlimited(t *testing.T) {
	// limit <= 0 means unlimited: every acquire succeeds and the in-flight gauge
	// is never touched, so no slot can ever leak.
	for _, limit := range []int{0, -1} {
		const channelId = 9001
		release, ok := AcquireChannelConcurrencySlot(channelId, limit)
		require.True(t, ok)
		require.NotNil(t, release)
		assert.Equal(t, int64(0), ChannelInFlight(channelId))
		release()
		assert.Equal(t, int64(0), ChannelInFlight(channelId))
	}
}

func TestAcquireChannelConcurrencySlotBoundary(t *testing.T) {
	const (
		channelId = 9002
		limit     = 2
	)

	r1, ok := AcquireChannelConcurrencySlot(channelId, limit)
	require.True(t, ok)
	assert.Equal(t, int64(1), ChannelInFlight(channelId))

	r2, ok := AcquireChannelConcurrencySlot(channelId, limit)
	require.True(t, ok)
	assert.Equal(t, int64(2), ChannelInFlight(channelId))

	// At capacity: the third acquire is rejected and holds no slot.
	r3, ok := AcquireChannelConcurrencySlot(channelId, limit)
	require.False(t, ok)
	assert.Nil(t, r3)
	assert.Equal(t, int64(2), ChannelInFlight(channelId), "rejected acquire must not increment the gauge")

	// Freeing a slot lets a new request in.
	r1()
	assert.Equal(t, int64(1), ChannelInFlight(channelId))

	r4, ok := AcquireChannelConcurrencySlot(channelId, limit)
	require.True(t, ok)
	assert.Equal(t, int64(2), ChannelInFlight(channelId))

	r2()
	r4()
	assert.Equal(t, int64(0), ChannelInFlight(channelId))
}

func TestAcquireChannelConcurrencySlotReleaseIsIdempotent(t *testing.T) {
	const (
		channelId = 9003
		limit     = 1
	)

	release, ok := AcquireChannelConcurrencySlot(channelId, limit)
	require.True(t, ok)
	assert.Equal(t, int64(1), ChannelInFlight(channelId))

	// A double release must free exactly one slot, never driving the gauge negative.
	release()
	release()
	assert.Equal(t, int64(0), ChannelInFlight(channelId))

	// The freed slot is reusable.
	_, ok = AcquireChannelConcurrencySlot(channelId, limit)
	require.True(t, ok)
	assert.Equal(t, int64(1), ChannelInFlight(channelId))
}
