package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupDistributorGhostTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldSQLite := common.UsingSQLite
	oldMySQL := common.UsingMySQL
	oldPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled
	oldMemoryCacheEnabled := common.MemoryCacheEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(&model.Channel{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	return db
}

func TestSetupContextForSelectedChannelGhostUsesFixedUpstreamForRelay(t *testing.T) {
	db := setupDistributorGhostTestDB(t)

	normalWeight := uint(100)
	normalPriority := int64(100)
	ghostWeight := uint(model.GhostChannelMarker)
	ghostPriority := int64(model.GhostChannelMarker)
	autoBan := 1
	realBaseURL := "https://real-upstream.example.com"
	ghostBaseURL := "https://ghost.example.com"
	realOrganization := "org-real"
	ghostOrganization := "org-ghost"
	realModelMapping := `{"gpt-shadow":"gpt-real"}`
	ghostModelMapping := `{"gpt-shadow":"gpt-ghost"}`
	realStatusCodeMapping := `{"429":503}`
	ghostStatusCodeMapping := `{"500":502}`

	real := model.Channel{
		Id:                 model.GhostChannelUpstreamId,
		Type:               constant.ChannelTypeOpenAI,
		Key:                "real-key",
		Status:             common.ChannelStatusEnabled,
		Name:               "real-upstream",
		Weight:             &normalWeight,
		Priority:           &normalPriority,
		AutoBan:            &autoBan,
		Models:             "gpt-real",
		Group:              "default",
		BaseURL:            &realBaseURL,
		OpenAIOrganization: &realOrganization,
		ModelMapping:       &realModelMapping,
		StatusCodeMapping:  &realStatusCodeMapping,
		CreatedTime:        123,
	}
	require.NoError(t, db.Create(&real).Error)

	ghost := model.Channel{
		Type:               constant.ChannelTypeGemini,
		Key:                "ghost-key",
		Status:             common.ChannelStatusEnabled,
		Name:               "ghost-channel",
		Weight:             &ghostWeight,
		Priority:           &ghostPriority,
		AutoBan:            &autoBan,
		Models:             "gpt-shadow",
		Group:              "default",
		BaseURL:            &ghostBaseURL,
		OpenAIOrganization: &ghostOrganization,
		ModelMapping:       &ghostModelMapping,
		StatusCodeMapping:  &ghostStatusCodeMapping,
		CreatedTime:        456,
	}
	require.NoError(t, db.Create(&ghost).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	newAPIError := SetupContextForSelectedChannel(ctx, &ghost, "gpt-shadow")
	require.Nil(t, newAPIError)

	assert.Equal(t, ghost.Id, common.GetContextKeyInt(ctx, constant.ContextKeyChannelId))
	assert.Equal(t, ghost.Name, common.GetContextKeyString(ctx, constant.ContextKeyChannelName))
	assert.Equal(t, ghost.Type, common.GetContextKeyInt(ctx, constant.ContextKeyChannelType))
	assert.Equal(t, ghostBaseURL, common.GetContextKeyString(ctx, constant.ContextKeyChannelBaseUrl))
	assert.Equal(t, "ghost-key", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	assert.Equal(t, ghostModelMapping, common.GetContextKeyString(ctx, constant.ContextKeyChannelModelMapping))
	assert.Equal(t, ghostStatusCodeMapping, common.GetContextKeyString(ctx, constant.ContextKeyChannelStatusCodeMapping))
	assert.Equal(t, "org-ghost", ctx.GetString("channel_organization"))

	info := &relaycommon.RelayInfo{OriginModelName: "gpt-shadow"}
	info.InitChannelMeta(ctx)

	assert.Equal(t, ghost.Id, info.ChannelId)
	assert.Equal(t, constant.ChannelTypeOpenAI, info.ChannelType)
	assert.Equal(t, "real-key", info.ApiKey)
	assert.Equal(t, realBaseURL, info.ChannelBaseUrl)
	assert.Equal(t, realOrganization, info.Organization)
	assert.Equal(t, int64(123), info.ChannelCreateTime)
	assert.True(t, info.SupportStreamOptions)
	assert.Equal(t, ghost.Id, common.GetContextKeyInt(ctx, constant.ContextKeyChannelId))
	assert.Equal(t, ghost.Type, common.GetContextKeyInt(ctx, constant.ContextKeyChannelType))
	assert.Equal(t, realModelMapping, ctx.GetString("model_mapping"))
	assert.Equal(t, realStatusCodeMapping, ctx.GetString("status_code_mapping"))
}
