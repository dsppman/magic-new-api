package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessChannelErrorMasksGhostAutoDisableReason(t *testing.T) {
	db := setupAutoChannelControllerTestDB(t)

	oldAutomaticDisable := common.AutomaticDisableChannelEnabled
	oldMemoryCache := common.MemoryCacheEnabled
	oldErrorLogEnabled := constant.ErrorLogEnabled
	common.AutomaticDisableChannelEnabled = true
	common.MemoryCacheEnabled = false
	constant.ErrorLogEnabled = false
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = oldAutomaticDisable
		common.MemoryCacheEnabled = oldMemoryCache
		constant.ErrorLogEnabled = oldErrorLogEnabled
	})

	ghostWeight := uint(model.GhostChannelMarker)
	ghostPriority := int64(model.GhostChannelMarker)
	autoBan := 1
	channel := model.Channel{
		Type:     constant.ChannelTypeVertexAi,
		Key:      "ghost-key",
		Status:   common.ChannelStatusEnabled,
		Name:     "ghost-channel",
		Weight:   &ghostWeight,
		Priority: &ghostPriority,
		AutoBan:  &autoBan,
		Models:   "gemini-2.5-flash",
		Group:    "Gemini",
	}
	require.NoError(t, db.Create(&channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set(service.GhostUpstreamChannelMetaKey, true)

	rawErr := types.NewOpenAIError(
		errors.New("Gemini invalid_api_key x-goog-api-key"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusUnauthorized,
	)
	processChannelError(ctx, *types.NewChannelError(channel.Id, channel.Type, channel.Name, false, "", true), rawErr)

	var updated model.Channel
	require.Eventually(t, func() bool {
		if err := db.First(&updated, channel.Id).Error; err != nil {
			return false
		}
		return updated.Status == common.ChannelStatusAutoDisabled
	}, time.Second, 10*time.Millisecond)

	reason, ok := updated.GetOtherInfo()["status_reason"].(string)
	require.True(t, ok)
	assert.Contains(t, reason, "status_code=401")
	assert.Contains(t, reason, "Request had invalid authentication credentials.")
	assert.NotContains(t, reason, "Gemini")
	assert.NotContains(t, reason, "invalid_api_key")
	assert.NotContains(t, reason, "x-goog-api-key")
}
