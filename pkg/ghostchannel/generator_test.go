package ghostchannel

import (
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
	assert.Equal(t, 1020, stats.Enabled)
	assert.Equal(t, 2080, stats.AutoDisabled)

	seenEmails := make(map[string]struct{}, len(channels))
	regionSet := knownRegionSet()
	for _, channel := range channels {
		assert.Zero(t, channel.Id)
		assert.Equal(t, constant.ChannelTypeVertexAi, channel.Type)
		assert.Equal(t, "Gemini", channel.Group)
		require.NotNil(t, channel.Weight)
		require.NotNil(t, channel.Priority)
		assert.Equal(t, uint(model.GhostChannelMarker), *channel.Weight)
		assert.Equal(t, int64(model.GhostChannelMarker), *channel.Priority)
		assert.Zero(t, channel.UsedQuota)
		assert.Equal(t, DefaultTag, *channel.Tag)
		assert.Contains(t, channel.Name, "@gmail.com")
		if _, ok := seenEmails[channel.Name]; ok {
			t.Fatalf("duplicate email: %s", channel.Name)
		}
		seenEmails[channel.Name] = struct{}{}

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
	}
}

func TestGenerateCanRandomizeUsedQuota(t *testing.T) {
	channels, _, err := Generate(Options{Count: DefaultCount, RandomUsedQuota: true, Now: 1781763600})
	require.NoError(t, err)
	require.Len(t, channels, DefaultCount)

	quotas := make([]int64, 0, len(channels))
	for _, channel := range channels {
		quotas = append(quotas, channel.UsedQuota)
	}
	sort.Slice(quotas, func(i, j int) bool { return quotas[i] < quotas[j] })
	assert.Greater(t, quotas[len(quotas)-1], int64(250_000_000))
	assert.Greater(t, quotas[len(quotas)/2], int64(20_000_000))
	assert.Less(t, quotas[0], int64(15_000_000))
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
