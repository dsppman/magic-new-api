package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
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
		Type:        constant.ChannelTypeOpenAI,
		Key:         "real-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "real-upstream",
		Weight:      &normalWeight,
		Priority:    &normalPriority,
		AutoBan:     &autoBan,
		Models:      "gpt-4o",
		Group:       "real",
		CreatedTime: 111,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Type:        constant.ChannelTypeVertexAi,
		Key:         "generated-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "generated.viewer@gmail.com",
		Weight:      &ghostWeight,
		Priority:    &ghostPriority,
		AutoBan:     &autoBan,
		Models:      "gemini-2.5-flash",
		Group:       "Gemini",
		CreatedTime: 222,
	}).Error)

	adminBody := performGetAllChannelsForRole(t, common.RoleAdminUser)
	require.True(t, adminBody.Success)
	require.Len(t, adminBody.Data.Items, 1)
	assert.Equal(t, int64(1), adminBody.Data.Total)
	assert.Equal(t, "generated.viewer@gmail.com", adminBody.Data.Items[0].Name)
	assert.Empty(t, adminBody.Data.Items[0].Key)
	assert.Equal(t, int64(adminChannelCreatedTime), adminBody.Data.Items[0].CreatedTime)

	rootBody := performGetAllChannelsForRole(t, common.RoleRootUser)
	require.True(t, rootBody.Success)
	assert.Equal(t, int64(2), rootBody.Data.Total)
	assert.Len(t, rootBody.Data.Items, 2)
	rootCreatedTimes := map[string]int64{}
	for _, channel := range rootBody.Data.Items {
		rootCreatedTimes[channel.Name] = channel.CreatedTime
	}
	assert.Equal(t, int64(111), rootCreatedTimes["real-upstream"])
	assert.Equal(t, int64(222), rootCreatedTimes["generated.viewer@gmail.com"])
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

func TestShouldRestrictChannelsForAdminOnlyForAdminRole(t *testing.T) {
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

			assert.Equal(t, tt.want, shouldRestrictChannelsForAdmin(ctx))
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
		Type:        constant.ChannelTypeOpenAI,
		Key:         "real-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "real-upstream",
		Weight:      &normalWeight,
		Priority:    &normalPriority,
		AutoBan:     &autoBan,
		Models:      "gpt-4o",
		Group:       "real",
		CreatedTime: 333,
	}
	require.NoError(t, db.Create(&real).Error)

	generated := model.Channel{
		Type:        constant.ChannelTypeVertexAi,
		Key:         "generated-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "generated.detail@gmail.com",
		Weight:      &ghostWeight,
		Priority:    &ghostPriority,
		AutoBan:     &autoBan,
		Models:      "gemini-2.5-flash",
		Group:       "Gemini",
		CreatedTime: 444,
	}
	require.NoError(t, db.Create(&generated).Error)

	adminGenerated := performGetChannelForRole(t, common.RoleAdminUser, generated.Id)
	require.True(t, adminGenerated.Success)
	assert.Equal(t, "generated.detail@gmail.com", adminGenerated.Data.Name)
	assert.Empty(t, adminGenerated.Data.Key)
	assert.Equal(t, int64(adminChannelCreatedTime), adminGenerated.Data.CreatedTime)

	adminReal := performGetChannelForRole(t, common.RoleAdminUser, real.Id)
	assert.False(t, adminReal.Success)

	rootReal := performGetChannelForRole(t, common.RoleRootUser, real.Id)
	require.True(t, rootReal.Success)
	assert.Equal(t, "real-upstream", rootReal.Data.Name)
	assert.Equal(t, int64(333), rootReal.Data.CreatedTime)
}

func TestAdminSeesHighPriorityNonGhostChannels(t *testing.T) {
	db := setupAutoChannelControllerTestDB(t)

	normalWeight := uint(100)
	normalPriority := int64(100)
	// A high-priority channel that is NOT a ghost channel: priority is at/above
	// the admin-visible threshold but weight is an ordinary value.
	highPriority := int64(model.AdminVisibleChannelPriorityThreshold)
	ghostWeight := uint(model.GhostChannelMarker)
	ghostPriority := int64(model.GhostChannelMarker)
	autoBan := 1

	normal := model.Channel{
		Type:        constant.ChannelTypeOpenAI,
		Key:         "real-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "real-upstream",
		Weight:      &normalWeight,
		Priority:    &normalPriority,
		AutoBan:     &autoBan,
		Models:      "gpt-4o",
		Group:       "real",
		CreatedTime: 111,
	}
	require.NoError(t, db.Create(&normal).Error)

	highPriorityChannel := model.Channel{
		Type:        constant.ChannelTypeOpenAI,
		Key:         "high-priority-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "high-priority-upstream",
		Weight:      &normalWeight,
		Priority:    &highPriority,
		AutoBan:     &autoBan,
		Models:      "gpt-4o",
		Group:       "high",
		CreatedTime: 222,
	}
	require.NoError(t, db.Create(&highPriorityChannel).Error)

	ghost := model.Channel{
		Type:        constant.ChannelTypeVertexAi,
		Key:         "generated-secret",
		Status:      common.ChannelStatusEnabled,
		Name:        "generated.viewer@gmail.com",
		Weight:      &ghostWeight,
		Priority:    &ghostPriority,
		AutoBan:     &autoBan,
		Models:      "gemini-2.5-flash",
		Group:       "Gemini",
		CreatedTime: 333,
	}
	require.NoError(t, db.Create(&ghost).Error)

	// Admin now sees both the ghost channel and the high-priority non-ghost
	// channel, but never the ordinary-priority channel.
	adminBody := performGetAllChannelsForRole(t, common.RoleAdminUser)
	require.True(t, adminBody.Success)
	assert.Equal(t, int64(2), adminBody.Data.Total)
	adminNames := map[string]bool{}
	for _, ch := range adminBody.Data.Items {
		adminNames[ch.Name] = true
		assert.Empty(t, ch.Key)
		assert.Equal(t, int64(adminChannelCreatedTime), ch.CreatedTime)
	}
	assert.True(t, adminNames["high-priority-upstream"])
	assert.True(t, adminNames["generated.viewer@gmail.com"])
	assert.False(t, adminNames["real-upstream"])

	// The single-channel endpoint exposes the high-priority non-ghost channel to
	// admins while still hiding the ordinary-priority one.
	adminHigh := performGetChannelForRole(t, common.RoleAdminUser, highPriorityChannel.Id)
	require.True(t, adminHigh.Success)
	assert.Equal(t, "high-priority-upstream", adminHigh.Data.Name)
	assert.Empty(t, adminHigh.Data.Key)

	adminNormal := performGetChannelForRole(t, common.RoleAdminUser, normal.Id)
	assert.False(t, adminNormal.Success)

	// Root keeps full visibility.
	rootBody := performGetAllChannelsForRole(t, common.RoleRootUser)
	require.True(t, rootBody.Success)
	assert.Equal(t, int64(3), rootBody.Data.Total)
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
		Id:       model.GhostChannelUpstreamId,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "real-secret",
		Status:   common.ChannelStatusEnabled,
		Name:     "real-upstream",
		Weight:   &normalWeight,
		Priority: &normalPriority,
		AutoBan:  &autoBan,
		Models:   "gemini-2.5-flash,gemini-2.5-pro",
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

	randomDisableStartTime := int64(1781331600)
	randomDisableEndTime := int64(1781763600)
	reqBody, err := common.Marshal(map[string]any{
		"count":                     20,
		"seed":                      int64(123),
		"models":                    "gemini-2.5-flash,gemini-2.5-pro,not-on-upstream",
		"groups":                    []string{"vip", "default"},
		"random_used_quota":         true,
		"random_disable_start_time": randomDisableStartTime,
		"random_disable_end_time":   randomDisableEndTime,
		"random_response_time":      true,
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
			Count        int `json:"count"`
			Enabled      int `json:"enabled"`
			AutoDisabled int `json:"auto_disabled"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	assert.Equal(t, 20, body.Data.Count)
	assert.Equal(t, body.Data.Count, body.Data.Enabled+body.Data.AutoDisabled)
	assert.Greater(t, body.Data.Enabled, 0)
	assert.Greater(t, body.Data.AutoDisabled, 0)

	var total int64
	require.NoError(t, db.Model(&model.Channel{}).Count(&total).Error)
	assert.Equal(t, int64(22), total)

	var generated []model.Channel
	require.NoError(t, model.ApplyGhostChannelFilter(db.Model(&model.Channel{})).Find(&generated).Error)
	require.Len(t, generated, 21)
	newGenerated := 0
	newAutoDisabled := 0
	responseTimeCount := 0
	for _, channel := range generated {
		require.NotNil(t, channel.Weight)
		require.NotNil(t, channel.Priority)
		assert.Equal(t, uint(model.GhostChannelMarker), *channel.Weight)
		assert.Equal(t, int64(model.GhostChannelMarker), *channel.Priority)
		if channel.Name == "old.generated@gmail.com" {
			continue
		}
		newGenerated++
		assert.NotEmpty(t, channel.Name)
		assert.NotContains(t, channel.Name, "@")
		assert.Equal(t, "gemini-2.5-flash,gemini-2.5-pro", channel.Models)
		assert.Equal(t, "vip,default", channel.Group)
		assert.Greater(t, channel.UsedQuota, int64(0))
		if channel.Status == common.ChannelStatusAutoDisabled {
			newAutoDisabled++
			statusTime, ok := channel.GetOtherInfo()["status_time"].(float64)
			require.True(t, ok)
			assert.GreaterOrEqual(t, int64(statusTime), randomDisableStartTime)
			assert.LessOrEqual(t, int64(statusTime), randomDisableEndTime)
		}
		if channel.ResponseTime > 0 {
			responseTimeCount++
		}
	}
	assert.Equal(t, 20, newGenerated)
	assert.Equal(t, body.Data.AutoDisabled, newAutoDisabled)
	assert.Greater(t, responseTimeCount, 0)
}

func TestResolveGhostChannelModelsUsesUpstreamModelList(t *testing.T) {
	db := setupAutoChannelControllerTestDB(t)

	normalWeight := uint(100)
	normalPriority := int64(100)
	autoBan := 1
	require.NoError(t, db.Create(&model.Channel{
		Id:       model.GhostChannelUpstreamId,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "real-secret",
		Status:   common.ChannelStatusEnabled,
		Name:     "real-upstream",
		Weight:   &normalWeight,
		Priority: &normalPriority,
		AutoBan:  &autoBan,
		Models:   "gemini-2.5-flash,gemini-2.5-pro,gemini-embedding-001",
		Group:    "real",
	}).Error)

	models, err := resolveGhostChannelModels("")
	require.NoError(t, err)
	assert.Equal(t, "gemini-2.5-flash,gemini-2.5-pro,gemini-embedding-001", models)

	models, err = resolveGhostChannelModels("gemini-2.5-pro,not-on-upstream,gemini-2.5-flash,gemini-2.5-pro")
	require.NoError(t, err)
	assert.Equal(t, "gemini-2.5-pro,gemini-2.5-flash", models)
}

func TestRandomDisableGhostChannelsUpdatesEnabledGhostChannels(t *testing.T) {
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
	for i := 0; i < 5; i++ {
		require.NoError(t, db.Create(&model.Channel{
			Type:     constant.ChannelTypeVertexAi,
			Key:      fmt.Sprintf("generated-secret-%d", i),
			Status:   common.ChannelStatusEnabled,
			Name:     fmt.Sprintf("generated-%d@gmail.com", i),
			Weight:   &ghostWeight,
			Priority: &ghostPriority,
			AutoBan:  &autoBan,
			Models:   "gemini-2.5-flash",
			Group:    "Gemini",
		}).Error)
	}
	require.NoError(t, db.Create(&model.Channel{
		Type:      constant.ChannelTypeVertexAi,
		Key:       "old-disabled",
		Status:    common.ChannelStatusAutoDisabled,
		Name:      "old.disabled@gmail.com",
		Weight:    &ghostWeight,
		Priority:  &ghostPriority,
		AutoBan:   &autoBan,
		Models:    "gemini-2.5-flash",
		Group:     "Gemini",
		OtherInfo: `{"status_reason":"old","status_time":1}`,
	}).Error)

	reqBody, err := common.Marshal(map[string]any{"count": 3})
	require.NoError(t, err)

	before := common.GetTimestamp()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/option/channel_random_auto_disable", bytes.NewReader(reqBody))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 1)
	ctx.Set("username", "root")
	ctx.Set("role", common.RoleRootUser)

	RandomDisableGhostChannels(ctx)
	after := common.GetTimestamp()

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Requested     int   `json:"requested"`
			Available     int   `json:"available"`
			Disabled      int   `json:"disabled"`
			StatusTime    int64 `json:"status_time"`
			StatusTimeMin int64 `json:"status_time_min"`
			StatusTimeMax int64 `json:"status_time_max"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	assert.Equal(t, 3, body.Data.Requested)
	assert.Equal(t, 5, body.Data.Available)
	assert.Equal(t, 3, body.Data.Disabled)
	assert.GreaterOrEqual(t, body.Data.StatusTime, before)
	assert.LessOrEqual(t, body.Data.StatusTime, after)
	assert.LessOrEqual(t, body.Data.StatusTimeMin, body.Data.StatusTimeMax)
	assert.LessOrEqual(t, body.Data.StatusTimeMax, body.Data.StatusTime)

	var real model.Channel
	require.NoError(t, db.Where("name = ?", "real-upstream").First(&real).Error)
	assert.Equal(t, common.ChannelStatusEnabled, real.Status)

	var ghosts []model.Channel
	require.NoError(t, model.ApplyGhostChannelFilter(db.Model(&model.Channel{})).Order("created_time asc").Order("id asc").Find(&ghosts).Error)
	require.Len(t, ghosts, 6)

	newlyDisabled := 0
	enabled := 0
	for _, channel := range ghosts {
		if channel.Name == "old.disabled@gmail.com" {
			assert.Equal(t, common.ChannelStatusAutoDisabled, channel.Status)
			assert.Equal(t, "old", channel.GetOtherInfo()["status_reason"])
			continue
		}
		switch channel.Status {
		case common.ChannelStatusEnabled:
			enabled++
		case common.ChannelStatusAutoDisabled:
			newlyDisabled++
			info := channel.GetOtherInfo()
			reason, ok := info["status_reason"].(string)
			require.True(t, ok)
			assert.NotEmpty(t, reason)
			statusTime, ok := info["status_time"].(float64)
			require.True(t, ok)
			assert.GreaterOrEqual(t, int64(statusTime), body.Data.StatusTimeMin)
			assert.LessOrEqual(t, int64(statusTime), body.Data.StatusTimeMax)
		default:
			t.Fatalf("unexpected ghost channel status %d", channel.Status)
		}
	}
	assert.Equal(t, 3, newlyDisabled)
	assert.Equal(t, 2, enabled)
}

func TestRandomDisableGhostChannelsUsesStatusTimesInRequestedRange(t *testing.T) {
	db := setupAutoChannelControllerTestDB(t)

	ghostWeight := uint(model.GhostChannelMarker)
	ghostPriority := int64(model.GhostChannelMarker)
	autoBan := 1

	for i := 0; i < 5; i++ {
		require.NoError(t, db.Create(&model.Channel{
			Type:     constant.ChannelTypeVertexAi,
			Key:      fmt.Sprintf("generated-secret-%d", i),
			Status:   common.ChannelStatusEnabled,
			Name:     fmt.Sprintf("generated-range-%d@gmail.com", i),
			Weight:   &ghostWeight,
			Priority: &ghostPriority,
			AutoBan:  &autoBan,
			Models:   "gemini-2.5-flash",
			Group:    "Gemini",
		}).Error)
	}

	randomDisableStartTime := int64(1781331600)
	randomDisableEndTime := int64(1781763600)
	reqBody, err := common.Marshal(map[string]any{
		"count":                     5,
		"random_disable_start_time": randomDisableStartTime,
		"random_disable_end_time":   randomDisableEndTime,
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/option/channel_random_auto_disable", bytes.NewReader(reqBody))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 1)
	ctx.Set("username", "root")
	ctx.Set("role", common.RoleRootUser)

	RandomDisableGhostChannels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Disabled      int   `json:"disabled"`
			StatusTimeMin int64 `json:"status_time_min"`
			StatusTimeMax int64 `json:"status_time_max"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	assert.Equal(t, 5, body.Data.Disabled)
	assert.GreaterOrEqual(t, body.Data.StatusTimeMin, randomDisableStartTime)
	assert.LessOrEqual(t, body.Data.StatusTimeMax, randomDisableEndTime)

	var ghosts []model.Channel
	require.NoError(t, model.ApplyGhostChannelFilter(db.Model(&model.Channel{})).Order("created_time asc").Order("id asc").Find(&ghosts).Error)
	require.Len(t, ghosts, 5)
	statusTimes := make([]int64, 0, len(ghosts))
	for _, channel := range ghosts {
		require.Equal(t, common.ChannelStatusAutoDisabled, channel.Status)
		statusTime, ok := channel.GetOtherInfo()["status_time"].(float64)
		require.True(t, ok)
		assert.GreaterOrEqual(t, int64(statusTime), randomDisableStartTime)
		assert.LessOrEqual(t, int64(statusTime), randomDisableEndTime)
		statusTimes = append(statusTimes, int64(statusTime))
	}
	sort.Slice(statusTimes, func(i, j int) bool {
		return statusTimes[i] < statusTimes[j]
	})
	assert.Equal(t, body.Data.StatusTimeMin, statusTimes[0])
	assert.Equal(t, body.Data.StatusTimeMax, statusTimes[len(statusTimes)-1])
}

func TestValidateRandomDisableTimeRangeRejectsFutureEndTime(t *testing.T) {
	now := common.GetTimestamp()

	start, end, message := validateRandomDisableTimeRange(now-60, now+60)

	assert.Zero(t, start)
	assert.Zero(t, end)
	assert.Equal(t, "随机自动禁用时间段结束时间不能晚于当前时间", message)
}
