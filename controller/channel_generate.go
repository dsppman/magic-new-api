package controller

import (
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/ghostchannel"

	"github.com/gin-gonic/gin"
)

type GenerateGhostChannelsRequest struct {
	Count                  int      `json:"count"`
	Seed                   *int64   `json:"seed"`
	Models                 string   `json:"models"`
	Group                  string   `json:"group"`
	Groups                 []string `json:"groups"`
	RandomUsedQuota        bool     `json:"random_used_quota"`
	RandomAutoDisable      *bool    `json:"random_auto_disable"`
	RandomDisableStartTime int64    `json:"random_disable_start_time"`
	RandomDisableEndTime   int64    `json:"random_disable_end_time"`
	RandomResponseTime     bool     `json:"random_response_time"`
}

type RandomDisableGhostChannelsRequest struct {
	Count                  int   `json:"count"`
	RandomDisableStartTime int64 `json:"random_disable_start_time"`
	RandomDisableEndTime   int64 `json:"random_disable_end_time"`
}

func GenerateGhostChannels(c *gin.Context) {
	req := GenerateGhostChannelsRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Count <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "生成数量必须大于 0"})
		return
	}
	if req.Count > 50000 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "单次最多生成 50000 条"})
		return
	}
	randomDisableStartTime, randomDisableEndTime, validationMessage := validateRandomDisableTimeRange(req.RandomDisableStartTime, req.RandomDisableEndTime)
	if validationMessage != "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": validationMessage})
		return
	}
	randomAutoDisable := req.RandomUsedQuota
	if req.RandomAutoDisable != nil {
		randomAutoDisable = *req.RandomAutoDisable
	}

	seed := time.Now().UnixNano()
	if req.Seed != nil {
		seed = *req.Seed
	}

	models, err := resolveGhostChannelModels(req.Models)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	channels, stats, err := ghostchannel.Generate(ghostchannel.Options{
		Count:                  req.Count,
		Seed:                   seed,
		Tag:                    ghostchannel.DefaultTag,
		Models:                 models,
		Group:                  req.Group,
		Groups:                 req.Groups,
		RandomUsedQuota:        req.RandomUsedQuota,
		RandomAutoDisable:      &randomAutoDisable,
		RandomDisableStartTime: randomDisableStartTime,
		RandomDisableEndTime:   randomDisableEndTime,
		RandomResponseTime:     req.RandomResponseTime,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.BatchInsertChannels(channels); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()

	recordManageAudit(c, "channel.auto_generate", map[string]interface{}{
		"count":                     stats.Count,
		"enabled":                   stats.Enabled,
		"auto_disabled":             stats.AutoDisabled,
		"group":                     req.Group,
		"groups":                    req.Groups,
		"random_used_quota":         req.RandomUsedQuota,
		"random_auto_disable":       randomAutoDisable,
		"random_disable_start_time": randomDisableStartTime,
		"random_disable_end_time":   randomDisableEndTime,
		"random_response_time":      req.RandomResponseTime,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"count":         stats.Count,
			"enabled":       stats.Enabled,
			"auto_disabled": stats.AutoDisabled,
		},
	})
}

func resolveGhostChannelModels(requestedModels string) (string, error) {
	upstreamChannel, err := model.GetChannelById(model.GhostChannelUpstreamId, true)
	if err != nil {
		return "", fmt.Errorf("获取影子上游渠道 #%d 失败: %w", model.GhostChannelUpstreamId, err)
	}

	upstreamModels := splitGhostModelNames(upstreamChannel.Models)
	if len(upstreamModels) == 0 {
		return "", fmt.Errorf("影子上游渠道 #%d 未配置模型", model.GhostChannelUpstreamId)
	}

	requested := splitGhostModelNames(requestedModels)
	if len(requested) == 0 {
		return strings.Join(upstreamModels, ","), nil
	}

	upstreamSet := make(map[string]struct{}, len(upstreamModels))
	for _, modelName := range upstreamModels {
		upstreamSet[modelName] = struct{}{}
	}

	result := make([]string, 0, len(requested))
	seen := map[string]struct{}{}
	for _, modelName := range requested {
		if _, ok := upstreamSet[modelName]; !ok {
			continue
		}
		if _, ok := seen[modelName]; ok {
			continue
		}
		seen[modelName] = struct{}{}
		result = append(result, modelName)
	}
	if len(result) == 0 {
		return "", fmt.Errorf("填写的模型均不在影子上游渠道 #%d 的模型列表中", model.GhostChannelUpstreamId)
	}
	return strings.Join(result, ","), nil
}

func splitGhostModelNames(models string) []string {
	parts := strings.FieldsFunc(models, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		modelName := strings.TrimSpace(part)
		if modelName == "" {
			continue
		}
		if _, ok := seen[modelName]; ok {
			continue
		}
		seen[modelName] = struct{}{}
		result = append(result, modelName)
	}
	return result
}

func validateRandomDisableTimeRange(startTime int64, endTime int64) (int64, int64, string) {
	if startTime == 0 && endTime == 0 {
		return 0, 0, ""
	}
	if startTime <= 0 || endTime <= 0 {
		return 0, 0, "请选择随机自动禁用时间段"
	}
	if endTime < startTime {
		return 0, 0, "随机自动禁用时间段开始时间不能晚于结束时间"
	}
	return startTime, endTime, ""
}

func RandomDisableGhostChannels(c *gin.Context) {
	req := RandomDisableGhostChannelsRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Count <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "随机自动禁用数量必须大于 0"})
		return
	}
	if req.Count > 50000 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "单次最多随机自动禁用 50000 条"})
		return
	}
	randomDisableStartTime, randomDisableEndTime, validationMessage := validateRandomDisableTimeRange(req.RandomDisableStartTime, req.RandomDisableEndTime)
	if validationMessage != "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": validationMessage})
		return
	}

	now := common.GetTimestamp()
	disableEndTime := now
	if randomDisableEndTime > 0 {
		disableEndTime = randomDisableEndTime
	}
	var candidates []model.Channel
	query := model.ApplyGhostChannelFilter(model.DB.Model(&model.Channel{})).
		Select("id", "created_time").
		Where("status = ?", common.ChannelStatusEnabled)
	query = query.Where("created_time = 0 OR created_time <= ?", disableEndTime-30*60)
	if err := query.Find(&candidates).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})
	limit := req.Count
	if limit > len(candidates) {
		limit = len(candidates)
	}
	statusTime := now
	statusTimeMin := int64(0)
	statusTimeMax := int64(0)
	disabled := 0
	statusTimes := ghostchannel.SequentialStatusTimes(limit, rng, now, randomDisableStartTime, randomDisableEndTime)
	for i, channel := range candidates[:limit] {
		reason := ghostchannel.RandomStatusReason(rng)
		channelStatusTime := statusTimes[i]
		if channel.CreatedTime > 0 && channelStatusTime < channel.CreatedTime+30*60 {
			channelStatusTime = channel.CreatedTime + 30*60
		}
		if model.UpdateChannelStatusWithTimestamp(channel.Id, common.ChannelStatusAutoDisabled, reason, channelStatusTime) {
			disabled++
			if statusTimeMin == 0 || channelStatusTime < statusTimeMin {
				statusTimeMin = channelStatusTime
			}
			if channelStatusTime > statusTimeMax {
				statusTimeMax = channelStatusTime
			}
		}
	}

	recordManageAudit(c, "channel.random_auto_disable", map[string]interface{}{
		"requested":                 req.Count,
		"available":                 len(candidates),
		"disabled":                  disabled,
		"status_time":               statusTime,
		"status_time_min":           statusTimeMin,
		"status_time_max":           statusTimeMax,
		"random_disable_start_time": randomDisableStartTime,
		"random_disable_end_time":   randomDisableEndTime,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"requested":                 req.Count,
			"available":                 len(candidates),
			"disabled":                  disabled,
			"status_time":               statusTime,
			"status_time_min":           statusTimeMin,
			"status_time_max":           statusTimeMax,
			"random_disable_start_time": randomDisableStartTime,
			"random_disable_end_time":   randomDisableEndTime,
		},
	})
}
