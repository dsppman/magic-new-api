package controller

import (
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type adminChannelListResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Items []struct {
			Id      int    `json:"id"`
			Name    string `json:"name"`
			Key     string `json:"key"`
			BaseURL string `json:"base_url"`
		} `json:"items"`
		Total int64 `json:"total"`
	} `json:"data"`
}

type adminChannelDetailResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Id      int    `json:"id"`
		Name    string `json:"name"`
		BaseURL string `json:"base_url"`
	} `json:"data"`
}

func setupChannelVisibilityTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	return db
}

func createVisibilityChannel(t *testing.T, db *gorm.DB, name string, priority int64) model.Channel {
	t.Helper()
	weight := uint(1)
	autoBan := 1
	baseURL := "https://upstream.example/" + name
	ch := model.Channel{
		Type:     1,
		Key:      "secret-" + name,
		Status:   common.ChannelStatusEnabled,
		Name:     name,
		BaseURL:  &baseURL,
		Weight:   &weight,
		Priority: &priority,
		AutoBan:  &autoBan,
		Models:   "gpt-4o",
		Group:    "default",
	}
	require.NoError(t, db.Create(&ch).Error)
	return ch
}

func getAllChannelsForRole(t *testing.T, role int) adminChannelListResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/api/channel/?p=1&page_size=50", nil)
	ctx.Set("role", role)
	GetAllChannels(ctx)

	var resp adminChannelListResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	return resp
}

func getChannelDetailForRole(t *testing.T, role int, id int) adminChannelDetailResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/api/channel/"+strconv.Itoa(id), nil)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(id)}}
	ctx.Set("role", role)
	GetChannel(ctx)

	var resp adminChannelDetailResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	return resp
}

// TestAdminChannelVisibilityWindow locks in the contract that an admin
// (non-root) role only sees channels whose priority is at or above
// model.AdminVisibleChannelPriorityThreshold, while root sees every channel.
func TestAdminChannelVisibilityWindow(t *testing.T) {
	db := setupChannelVisibilityTestDB(t)

	ordinary := createVisibilityChannel(t, db, "ordinary", 100)
	atThreshold := createVisibilityChannel(t, db, "at-threshold", model.AdminVisibleChannelPriorityThreshold)
	createVisibilityChannel(t, db, "above-threshold", model.AdminVisibleChannelPriorityThreshold+10)

	// List: admin sees only the two priority>=threshold channels, never the ordinary one.
	adminList := getAllChannelsForRole(t, common.RoleAdminUser)
	require.True(t, adminList.Success)
	assert.Equal(t, int64(2), adminList.Data.Total)
	names := map[string]bool{}
	for _, item := range adminList.Data.Items {
		names[item.Name] = true
		assert.Empty(t, item.Key, "channel key must not leak in the list response")
		assert.Empty(t, item.BaseURL, "channel API address (base_url) must be empty for the admin role")
	}
	assert.True(t, names["at-threshold"])
	assert.True(t, names["above-threshold"])
	assert.False(t, names["ordinary"])

	// List: root sees all three, with the real API address intact.
	rootList := getAllChannelsForRole(t, common.RoleRootUser)
	require.True(t, rootList.Success)
	assert.Equal(t, int64(3), rootList.Data.Total)
	for _, item := range rootList.Data.Items {
		assert.NotEmpty(t, item.BaseURL, "root must still see the channel API address")
	}

	// Detail: admin is allowed to read the at-threshold channel, but its API
	// address is blanked.
	adminAllowed := getChannelDetailForRole(t, common.RoleAdminUser, atThreshold.Id)
	require.True(t, adminAllowed.Success)
	assert.Equal(t, "at-threshold", adminAllowed.Data.Name)
	assert.Empty(t, adminAllowed.Data.BaseURL, "admin detail must not expose base_url")

	// ...but is denied the ordinary (below-threshold) channel.
	adminDenied := getChannelDetailForRole(t, common.RoleAdminUser, ordinary.Id)
	assert.False(t, adminDenied.Success)

	// Detail: root can read the ordinary channel, including its API address.
	rootDetail := getChannelDetailForRole(t, common.RoleRootUser, ordinary.Id)
	require.True(t, rootDetail.Success)
	assert.Equal(t, "ordinary", rootDetail.Data.Name)
	assert.NotEmpty(t, rootDetail.Data.BaseURL, "root detail must include base_url")
}
