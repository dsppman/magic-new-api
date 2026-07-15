package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// channelView is a minimal projection of the channel JSON the read endpoints
// return, limited to the fields the public-view restriction governs.
type channelView struct {
	Id        int     `json:"id"`
	Name      string  `json:"name"`
	Tag       *string `json:"tag"`
	BaseURL   *string `json:"base_url"`
	UsedQuota int64   `json:"used_quota"`
}

func channelTagPtr(s string) *string { return &s }

// publicChannelUsedQuota deliberately exceeds math.MaxInt32 so the scaling test
// also guards against a regression to an int32-saturating conversion.
const (
	publicChannelUsedQuota   = int64(10_000_000_000)
	publicChannelScaledQuota = int64(8_500_000_000) // 10e9 * 0.85
	privateChannelUsedQuota  = int64(2_000_000_000)
)

// seedPublicViewChannels installs one public-tagged channel plus a private and
// an untagged channel, each with a distinct upstream URL and used quota.
func seedPublicViewChannels(t *testing.T) {
	t.Helper()
	setupModelListControllerTestDB(t)
	publicURL := "https://public.example.com"
	privateURL := "https://private.example.com"
	untaggedURL := "https://untagged.example.com"
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: 1, Name: "public-ch", Group: "default",
		Tag: channelTagPtr(publicChannelTag), BaseURL: &publicURL,
		UsedQuota: publicChannelUsedQuota,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: 2, Name: "private-ch", Group: "default",
		Tag: channelTagPtr("private"), BaseURL: &privateURL,
		UsedQuota: privateChannelUsedQuota,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: 3, Name: "untagged-ch", Group: "default",
		BaseURL: &untaggedURL,
	}).Error)
}

func listChannels(t *testing.T, role int) []channelView {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", role)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/", nil)

	GetAllChannels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Items []channelView `json:"items"`
			Total int64         `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Equal(t, int64(len(payload.Data.Items)), payload.Data.Total)
	return payload.Data.Items
}

func getChannelDetail(t *testing.T, role int, id string) (bool, *channelView) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", role)
	ctx.Params = gin.Params{{Key: "id", Value: id}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/"+id, nil)

	GetChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool         `json:"success"`
		Data    *channelView `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload.Success, payload.Data
}

func TestGetAllChannelsAdminSeesOnlyPublicWithMaskedURL(t *testing.T) {
	seedPublicViewChannels(t)

	items := listChannels(t, common.RoleAdminUser)

	require.Len(t, items, 1)
	assert.Equal(t, "public-ch", items[0].Name)
	require.NotNil(t, items[0].BaseURL)
	assert.Equal(t, "", *items[0].BaseURL, "restricted viewer must not receive the upstream URL")
	assert.Equal(t, publicChannelScaledQuota, items[0].UsedQuota, "restricted viewer sees used quota scaled by 0.85")
}

func TestGetAllChannelsRootSeesAllWithRealURL(t *testing.T) {
	seedPublicViewChannels(t)

	items := listChannels(t, common.RoleRootUser)

	require.Len(t, items, 3)
	byName := map[string]channelView{}
	for _, ch := range items {
		require.NotNil(t, ch.BaseURL)
		byName[ch.Name] = ch
	}
	assert.Equal(t, "https://public.example.com", *byName["public-ch"].BaseURL)
	assert.Equal(t, "https://private.example.com", *byName["private-ch"].BaseURL)
	assert.Equal(t, "https://untagged.example.com", *byName["untagged-ch"].BaseURL)
	assert.Equal(t, publicChannelUsedQuota, byName["public-ch"].UsedQuota, "root sees the real used quota")
	assert.Equal(t, privateChannelUsedQuota, byName["private-ch"].UsedQuota)
}

func TestGetChannelAdminPublicMasksURL(t *testing.T) {
	seedPublicViewChannels(t)

	ok, ch := getChannelDetail(t, common.RoleAdminUser, "1")

	require.True(t, ok)
	require.NotNil(t, ch)
	require.NotNil(t, ch.BaseURL)
	assert.Equal(t, "", *ch.BaseURL)
	assert.Equal(t, publicChannelScaledQuota, ch.UsedQuota)
}

func TestGetChannelAdminNonPublicIsHidden(t *testing.T) {
	seedPublicViewChannels(t)

	ok, _ := getChannelDetail(t, common.RoleAdminUser, "2")

	assert.False(t, ok, "restricted viewer must not fetch a non-public channel by id")
}

func TestGetChannelRootSeesRealURL(t *testing.T) {
	seedPublicViewChannels(t)

	ok, ch := getChannelDetail(t, common.RoleRootUser, "2")

	require.True(t, ok)
	require.NotNil(t, ch)
	require.NotNil(t, ch.BaseURL)
	assert.Equal(t, "https://private.example.com", *ch.BaseURL)
	assert.Equal(t, privateChannelUsedQuota, ch.UsedQuota)
}
