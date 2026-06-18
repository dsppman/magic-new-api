package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type channelListAPIResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Items []model.Channel `json:"items"`
		Total int64           `json:"total"`
	} `json:"data"`
}

func setupAutoChannelControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	initModelListColumnNames(t)
	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.Log{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func TestAdminOnlyReadsGhostChannels(t *testing.T) {
	db := setupAutoChannelControllerTestDB(t)

	normalWeight := uint(100)
	normalPriority := int64(100)
	ghostWeight := uint(model.GhostChannelMarker)
	ghostPriority := int64(model.GhostChannelMarker)
	autoBan := 1

	require.NoError(t, db.Create(&model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "real-secret",
		Status:   common.ChannelStatusEnabled,
		Name:     "real-upstream",
		Weight:   &normalWeight,
		Priority: &normalPriority,
		AutoBan:  &autoBan,
		Models:   "gpt-4o",
		Group:    "real",
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Type:     constant.ChannelTypeVertexAi,
		Key:      "generated-secret",
		Status:   common.ChannelStatusEnabled,
		Name:     "generated.viewer@gmail.com",
		Weight:   &ghostWeight,
		Priority: &ghostPriority,
		AutoBan:  &autoBan,
		Models:   "gemini-2.5-flash",
		Group:    "Gemini",
	}).Error)

	adminBody := performGetAllChannelsForRole(t, common.RoleAdminUser)
	require.True(t, adminBody.Success)
	require.Len(t, adminBody.Data.Items, 1)
	assert.Equal(t, int64(1), adminBody.Data.Total)
	assert.Equal(t, "generated.viewer@gmail.com", adminBody.Data.Items[0].Name)
	assert.Empty(t, adminBody.Data.Items[0].Key)

	rootBody := performGetAllChannelsForRole(t, common.RoleRootUser)
	require.True(t, rootBody.Success)
	assert.Equal(t, int64(2), rootBody.Data.Total)
	assert.Len(t, rootBody.Data.Items, 2)
}

func performGetAllChannelsForRole(t *testing.T, role int) channelListAPIResponse {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/?p=1&page_size=20", nil)
	ctx.Set("role", role)

	GetAllChannels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body channelListAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	return body
}

func TestShouldFilterGhostChannelsOnlyForAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		role any
		want bool
	}{
		{name: "missing role", role: nil, want: false},
		{name: "common user", role: common.RoleCommonUser, want: false},
		{name: "admin user", role: common.RoleAdminUser, want: true},
		{name: "custom admin range", role: 50, want: true},
		{name: "root user", role: common.RoleRootUser, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			if tt.role != nil {
				ctx.Set("role", tt.role)
			}

			assert.Equal(t, tt.want, shouldFilterGhostChannels(ctx))
		})
	}
}

func TestGetChannelFiltersGhostForAdmin(t *testing.T) {
	db := setupAutoChannelControllerTestDB(t)

	normalWeight := uint(100)
	normalPriority := int64(100)
	ghostWeight := uint(model.GhostChannelMarker)
	ghostPriority := int64(model.GhostChannelMarker)
	autoBan := 1

	real := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "real-secret",
		Status:   common.ChannelStatusEnabled,
		Name:     "real-upstream",
		Weight:   &normalWeight,
		Priority: &normalPriority,
		AutoBan:  &autoBan,
		Models:   "gpt-4o",
		Group:    "real",
	}
	require.NoError(t, db.Create(&real).Error)

	generated := model.Channel{
		Type:     constant.ChannelTypeVertexAi,
		Key:      "generated-secret",
		Status:   common.ChannelStatusEnabled,
		Name:     "generated.detail@gmail.com",
		Weight:   &ghostWeight,
		Priority: &ghostPriority,
		AutoBan:  &autoBan,
		Models:   "gemini-2.5-flash",
		Group:    "Gemini",
	}
	require.NoError(t, db.Create(&generated).Error)

	adminGenerated := performGetChannelForRole(t, common.RoleAdminUser, generated.Id)
	require.True(t, adminGenerated.Success)
	assert.Equal(t, "generated.detail@gmail.com", adminGenerated.Data.Name)
	assert.Empty(t, adminGenerated.Data.Key)

	adminReal := performGetChannelForRole(t, common.RoleAdminUser, real.Id)
	assert.False(t, adminReal.Success)

	rootReal := performGetChannelForRole(t, common.RoleRootUser, real.Id)
	require.True(t, rootReal.Success)
	assert.Equal(t, "real-upstream", rootReal.Data.Name)
}

func TestEnabledListModelsFiltersGhostForAdmin(t *testing.T) {
	db := setupAutoChannelControllerTestDB(t)

	normalWeight := uint(100)
	normalPriority := int64(100)
	ghostWeight := uint(model.GhostChannelMarker)
	ghostPriority := int64(model.GhostChannelMarker)
	autoBan := 1

	real := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "real-secret",
		Status:   common.ChannelStatusEnabled,
		Name:     "real-upstream",
		Weight:   &normalWeight,
		Priority: &normalPriority,
		AutoBan:  &autoBan,
		Models:   "gpt-4o",
		Group:    "real",
	}
	require.NoError(t, db.Create(&real).Error)
	require.NoError(t, real.AddAbilities(nil))

	generated := model.Channel{
		Type:     constant.ChannelTypeVertexAi,
		Key:      "generated-secret",
		Status:   common.ChannelStatusEnabled,
		Name:     "generated.enabled@gmail.com",
		Weight:   &ghostWeight,
		Priority: &ghostPriority,
		AutoBan:  &autoBan,
		Models:   "gemini-2.5-flash",
		Group:    "Gemini",
	}
	require.NoError(t, db.Create(&generated).Error)
	require.NoError(t, generated.AddAbilities(nil))

	disabledGenerated := model.Channel{
		Type:     constant.ChannelTypeVertexAi,
		Key:      "disabled-generated-secret",
		Status:   common.ChannelStatusAutoDisabled,
		Name:     "generated.disabled@gmail.com",
		Weight:   &ghostWeight,
		Priority: &ghostPriority,
		AutoBan:  &autoBan,
		Models:   "gemini-2.5-pro",
		Group:    "Gemini",
	}
	require.NoError(t, db.Create(&disabledGenerated).Error)
	require.NoError(t, disabledGenerated.AddAbilities(nil))

	adminBody := performEnabledListModelsForRole(t, common.RoleAdminUser)
	require.True(t, adminBody.Success)
	assert.ElementsMatch(t, []string{"gemini-2.5-flash"}, adminBody.Data)

	rootBody := performEnabledListModelsForRole(t, common.RoleRootUser)
	require.True(t, rootBody.Success)
	assert.ElementsMatch(t, []string{"gpt-4o", "gemini-2.5-flash"}, rootBody.Data)
}

type enabledListModelsAPIResponse struct {
	Success bool     `json:"success"`
	Data    []string `json:"data"`
}

func performEnabledListModelsForRole(t *testing.T, role int) enabledListModelsAPIResponse {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/models_enabled", nil)
	ctx.Set("role", role)

	EnabledListModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body enabledListModelsAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	return body
}

type channelDetailAPIResponse struct {
	Success bool          `json:"success"`
	Data    model.Channel `json:"data"`
}

func performGetChannelForRole(t *testing.T, role int, id int) channelDetailAPIResponse {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/"+strconv.Itoa(id), nil)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(id)}}
	ctx.Set("role", role)

	GetChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body channelDetailAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	return body
}

func TestGenerateGhostChannelsWritesRealChannelTable(t *testing.T) {
	db := setupAutoChannelControllerTestDB(t)

	normalWeight := uint(100)
	normalPriority := int64(100)
	ghostWeight := uint(model.GhostChannelMarker)
	ghostPriority := int64(model.GhostChannelMarker)
	autoBan := 1

	require.NoError(t, db.Create(&model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "real-secret",
		Status:   common.ChannelStatusEnabled,
		Name:     "real-upstream",
		Weight:   &normalWeight,
		Priority: &normalPriority,
		AutoBan:  &autoBan,
		Models:   "gpt-4o",
		Group:    "real",
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Type:     constant.ChannelTypeVertexAi,
		Key:      "old-generated",
		Status:   common.ChannelStatusEnabled,
		Name:     "old.generated@gmail.com",
		Weight:   &ghostWeight,
		Priority: &ghostPriority,
		AutoBan:  &autoBan,
		Models:   "gemini-2.5-flash",
		Group:    "Gemini",
	}).Error)

	reqBody, err := common.Marshal(map[string]any{
		"count":             3,
		"seed":              int64(123),
		"models":            "gemini-2.5-flash,gemini-2.5-pro",
		"random_used_quota": true,
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/option/channel_auto_generate", bytes.NewReader(reqBody))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 1)
	ctx.Set("username", "root")
	ctx.Set("role", common.RoleRootUser)

	GenerateGhostChannels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Count int `json:"count"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	assert.Equal(t, 3, body.Data.Count)

	var total int64
	require.NoError(t, db.Model(&model.Channel{}).Count(&total).Error)
	assert.Equal(t, int64(5), total)

	var generated []model.Channel
	require.NoError(t, model.ApplyGhostChannelFilter(db.Model(&model.Channel{})).Find(&generated).Error)
	require.Len(t, generated, 4)
	newGenerated := 0
	for _, channel := range generated {
		require.NotNil(t, channel.Weight)
		require.NotNil(t, channel.Priority)
		assert.Equal(t, uint(model.GhostChannelMarker), *channel.Weight)
		assert.Equal(t, int64(model.GhostChannelMarker), *channel.Priority)
		if channel.Name == "old.generated@gmail.com" {
			continue
		}
		newGenerated++
		assert.Contains(t, channel.Name, "@gmail.com")
		assert.Equal(t, "gemini-2.5-flash,gemini-2.5-pro", channel.Models)
		assert.Greater(t, channel.UsedQuota, int64(0))
	}
	assert.Equal(t, 3, newGenerated)
}
