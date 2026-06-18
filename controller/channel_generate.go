package controller

import (
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelautogen"

	"github.com/gin-gonic/gin"
)

type GenerateAutoChannelsRequest struct {
	Count           int    `json:"count"`
	Seed            *int64 `json:"seed"`
	Models          string `json:"models"`
	RandomUsedQuota bool   `json:"random_used_quota"`
}

func GenerateAutoChannels(c *gin.Context) {
	req := GenerateAutoChannelsRequest{}
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

	channels, stats, err := channelautogen.Generate(channelautogen.Options{
		Count:           req.Count,
		Seed:            seed,
		Tag:             channelautogen.DefaultTag,
		Models:          req.Models,
		RandomUsedQuota: req.RandomUsedQuota,
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
		"count":         stats.Count,
		"enabled":       stats.Enabled,
		"auto_disabled": stats.AutoDisabled,
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
