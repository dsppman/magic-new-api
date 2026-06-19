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
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessChannelErrorUsesMappedGhostErrorForAutoDisable(t *testing.T) {
	db := setupAutoChannelControllerTestDB(t)

	oldAutomaticDisable := common.AutomaticDisableChannelEnabled
	oldMemoryCache := common.MemoryCacheEnabled
	oldErrorLogEnabled := constant.ErrorLogEnabled
	oldAutomaticDisableStatusCodeRanges := operation_setting.AutomaticDisableStatusCodeRanges
	oldAutomaticDisableKeywords := operation_setting.AutomaticDisableKeywords
	common.AutomaticDisableChannelEnabled = true
	common.MemoryCacheEnabled = false
	constant.ErrorLogEnabled = false
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: http.StatusUnauthorized, End: http.StatusUnauthorized}}
	operation_setting.AutomaticDisableKeywords = nil
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = oldAutomaticDisable
		common.MemoryCacheEnabled = oldMemoryCache
		constant.ErrorLogEnabled = oldErrorLogEnabled
		operation_setting.AutomaticDisableStatusCodeRanges = oldAutomaticDisableStatusCodeRanges
		operation_setting.AutomaticDisableKeywords = oldAutomaticDisableKeywords
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
		http.StatusInternalServerError,
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
