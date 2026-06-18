package controller

import (
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/ghostchannel"

	"github.com/gin-gonic/gin"
)

type GenerateGhostChannelsRequest struct {
	Count              int      `json:"count"`
	Seed               *int64   `json:"seed"`
	Models             string   `json:"models"`
	Group              string   `json:"group"`
	Groups             []string `json:"groups"`
	RandomUsedQuota    bool     `json:"random_used_quota"`
	RandomResponseTime bool     `json:"random_response_time"`
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

	seed := time.Now().UnixNano()
	if req.Seed != nil {
		seed = *req.Seed
	}

	channels, stats, err := ghostchannel.Generate(ghostchannel.Options{
		Count:              req.Count,
		Seed:               seed,
		Tag:                ghostchannel.DefaultTag,
		Models:             req.Models,
		Group:              req.Group,
		Groups:             req.Groups,
		RandomUsedQuota:    req.RandomUsedQuota,
		RandomResponseTime: req.RandomResponseTime,
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
		"count":                stats.Count,
		"enabled":              stats.Enabled,
		"auto_disabled":        stats.AutoDisabled,
		"group":                req.Group,
		"groups":               req.Groups,
		"random_used_quota":    req.RandomUsedQuota,
		"random_response_time": req.RandomResponseTime,
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
