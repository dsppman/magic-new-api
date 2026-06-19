package ghostchannel

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateDefaultVertexChannels(t *testing.T) {
	channels, stats, err := Generate(Options{Now: 1781763600})
	require.NoError(t, err)
	require.Len(t, channels, DefaultCount)

	assert.Equal(t, DefaultCount, stats.Count)
	assert.Equal(t, DefaultCount, stats.Enabled)
	assert.Zero(t, stats.AutoDisabled)

	seenNames := make(map[string]struct{}, len(channels))
	seenModelSets := make(map[string]struct{}, len(channels))
	nonZeroQuota := 0
	previousCreatedTime := int64(0)
	createdGaps := make([]int64, 0, len(channels)-1)
	regionSet := knownRegionSet()
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for _, channel := range channels {
		assert.Zero(t, channel.Id)
		assert.Equal(t, constant.ChannelTypeVertexAi, channel.Type)
		assert.Equal(t, "Gemini", channel.Group)
		assert.Greater(t, channel.CreatedTime, previousCreatedTime)
		if previousCreatedTime > 0 {
			createdGaps = append(createdGaps, channel.CreatedTime-previousCreatedTime)
		}
		previousCreatedTime = channel.CreatedTime
		assert.GreaterOrEqual(t, channel.TestTime, channel.CreatedTime)
		assert.LessOrEqual(t, channel.TestTime, int64(1781763600))
		require.NotNil(t, channel.Weight)
		require.NotNil(t, channel.Priority)
		assert.Equal(t, uint(model.GhostChannelMarker), *channel.Weight)
		assert.Equal(t, int64(model.GhostChannelMarker), *channel.Priority)
		assert.Greater(t, channel.ResponseTime, 0)
		assert.GreaterOrEqual(t, channel.UsedQuota, int64(0))
		if channel.UsedQuota > 0 {
			nonZeroQuota++
		}
		assert.Equal(t, DefaultTag, *channel.Tag)
		assert.NotEmpty(t, channel.Name)
		assert.False(t, uuidPattern.MatchString(channel.Name), "channel name should not look like a raw UUID: %s", channel.Name)
		if _, ok := seenNames[channel.Name]; ok {
			t.Fatalf("duplicate channel name: %s", channel.Name)
		}
		seenNames[channel.Name] = struct{}{}

		var key map[string]any
		require.NoError(t, common.Unmarshal([]byte(channel.Key), &key))
		assert.Equal(t, "service_account", key["type"])
		assert.Contains(t, channel.Key, "iam.gserviceaccount.com")
		assert.NotContains(t, channel.Key, "mock-key")

		var other map[string]string
		require.NoError(t, common.Unmarshal([]byte(channel.Other), &other))
		_, ok := regionSet[other["default"]]
		assert.True(t, ok, "unexpected default region %q", other["default"])

		var settings map[string]string
		require.NoError(t, common.Unmarshal([]byte(channel.OtherSettings), &settings))
		assert.Equal(t, "json", settings["vertex_key_type"])
		assert.NotEmpty(t, strings.Split(channel.Models, ","))
		seenModelSets[channel.Models] = struct{}{}
	}
	assert.Greater(t, nonZeroQuota, DefaultCount/2)
	assert.Greater(t, len(seenModelSets), 1)
	assert.Greater(t, distinctInt64Count(createdGaps), 10)
}

func TestGenerateCanRandomizeUsedQuotaAndAutoDisabled(t *testing.T) {
	channels, stats, err := Generate(Options{
		Count:           DefaultCount,
		RandomUsedQuota: true,
		Now:             1781763600,
	})
	require.NoError(t, err)
	require.Len(t, channels, DefaultCount)

	assert.Equal(t, DefaultCount, stats.Enabled+stats.AutoDisabled)
	assert.Greater(t, stats.Enabled, 0)
	assert.Greater(t, stats.AutoDisabled, 0)

	disabled := 0
	quotas := make([]int64, 0, len(channels))
	for _, channel := range channels {
		if channel.Status == common.ChannelStatusAutoDisabled {
			disabled++
		}
		quotas = append(quotas, channel.UsedQuota)
	}
	assert.Equal(t, stats.AutoDisabled, disabled)
	sort.Slice(quotas, func(i, j int) bool { return quotas[i] < quotas[j] })
	assert.Greater(t, quotas[len(quotas)-1], int64(250_000_000))
	assert.Greater(t, quotas[len(quotas)/2], int64(20_000_000))
	assert.Less(t, quotas[0], int64(15_000_000))
}

func TestGenerateCanRandomizeAutoDisabledStatusTimeInRange(t *testing.T) {
	randomAutoDisable := true
	startTime := int64(1781331600)
	endTime := int64(1781763600)
	channels, stats, err := Generate(Options{
		Count:                  DefaultCount,
		RandomAutoDisable:      &randomAutoDisable,
		RandomDisableStartTime: startTime,
		RandomDisableEndTime:   endTime,
		Now:                    endTime,
	})
	require.NoError(t, err)
	require.Len(t, channels, DefaultCount)
	assert.Greater(t, stats.AutoDisabled, 0)

	previousCreatedTime := int64(0)
	disabledIndexes := make([]int, 0, stats.AutoDisabled)
	disabledStatusTimes := make([]int64, 0, stats.AutoDisabled)
	for index, channel := range channels {
		assert.Greater(t, channel.CreatedTime, previousCreatedTime)
		previousCreatedTime = channel.CreatedTime
		if channel.Status != common.ChannelStatusAutoDisabled {
			continue
		}

		var otherInfo map[string]any
		require.NoError(t, common.Unmarshal([]byte(channel.OtherInfo), &otherInfo))
		statusTime, ok := otherInfo["status_time"].(float64)
		require.True(t, ok)
		assert.GreaterOrEqual(t, int64(statusTime), startTime)
		assert.LessOrEqual(t, int64(statusTime), endTime)
		assert.GreaterOrEqual(t, int64(statusTime), channel.CreatedTime)
		disabledIndexes = append(disabledIndexes, index)
		disabledStatusTimes = append(disabledStatusTimes, int64(statusTime))
	}
	assert.Greater(t, len(disabledIndexes), 2)
	assert.False(t, indexesAreConsecutive(disabledIndexes))
	assert.False(t, int64sAreNonDecreasing(disabledStatusTimes))

	sort.Slice(disabledStatusTimes, func(i, j int) bool {
		return disabledStatusTimes[i] < disabledStatusTimes[j]
	})
	statusTimeGaps := make([]int64, 0, len(disabledStatusTimes)-1)
	for i := 1; i < len(disabledStatusTimes); i++ {
		assert.GreaterOrEqual(t, disabledStatusTimes[i], disabledStatusTimes[i-1])
		statusTimeGaps = append(statusTimeGaps, disabledStatusTimes[i]-disabledStatusTimes[i-1])
	}
	assert.Greater(t, distinctInt64Count(statusTimeGaps), 10)
}

func TestGenerateCanUseProvidedGroups(t *testing.T) {
	channels, _, err := Generate(Options{
		Count:  3,
		Groups: []string{"vip", "default", "vip", ""},
		Now:    1781763600,
	})
	require.NoError(t, err)
	require.Len(t, channels, 3)
	for _, channel := range channels {
		assert.Equal(t, "vip,default", channel.Group)
	}

	channels, _, err = Generate(Options{
		Count: 1,
		Group: "vip, default",
		Now:   1781763600,
	})
	require.NoError(t, err)
	require.Len(t, channels, 1)
	assert.Equal(t, "vip,default", channels[0].Group)
}

func TestGenerateCanRandomizeResponseTime(t *testing.T) {
	channels, _, err := Generate(Options{Count: DefaultCount, RandomResponseTime: true, Now: 1781763600})
	require.NoError(t, err)
	require.Len(t, channels, DefaultCount)

	responseTimes := make([]int, 0, len(channels))
	for _, channel := range channels {
		responseTimes = append(responseTimes, channel.ResponseTime)
	}
	sort.Ints(responseTimes)
	assert.Greater(t, responseTimes[len(responseTimes)-1], 0)
	assert.Greater(t, responseTimes[len(responseTimes)/2], 0)
	assert.Greater(t, responseTimes[0], 0)
	assert.Greater(t, responseTimes[len(responseTimes)-1], responseTimes[0])
}

func TestGenerateUsesProvidedModels(t *testing.T) {
	channels, _, err := Generate(Options{
		Count:  3,
		Models: "gemini-2.5-flash, gemini-2.5-pro\ngemini-2.5-flash",
		Now:    1781763600,
	})
	require.NoError(t, err)
	require.Len(t, channels, 3)
	for _, channel := range channels {
		assert.Equal(t, "gemini-2.5-flash,gemini-2.5-pro", channel.Models)
	}
}

func TestGenerateIsDeterministicWithSeedAndNow(t *testing.T) {
	first, _, err := Generate(Options{Count: 5, Seed: 123, Now: 1781763600})
	require.NoError(t, err)
	second, _, err := Generate(Options{Count: 5, Seed: 123, Now: 1781763600})
	require.NoError(t, err)

	require.Len(t, first, 5)
	require.Len(t, second, 5)
	assert.Equal(t, first[0].Name, second[0].Name)
	assert.Equal(t, first[0].Key, second[0].Key)
	assert.Equal(t, first[0].Other, second[0].Other)
	assert.Equal(t, first[0].UsedQuota, second[0].UsedQuota)
}

func knownRegionSet() map[string]struct{} {
	result := make(map[string]struct{}, len(vertexDefaultRegions))
	for _, region := range vertexDefaultRegions {
		result[region.Value] = struct{}{}
	}
	return result
}

func distinctInt64Count(values []int64) int {
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	return len(seen)
}

func indexesAreConsecutive(indexes []int) bool {
	if len(indexes) < 2 {
		return true
	}
	for i := 1; i < len(indexes); i++ {
		if indexes[i] != indexes[i-1]+1 {
			return false
		}
	}
	return true
}

func int64sAreNonDecreasing(values []int64) bool {
	if len(values) < 2 {
		return true
	}
	for i := 1; i < len(values); i++ {
		if values[i] < values[i-1] {
			return false
		}
	}
	return true
}
