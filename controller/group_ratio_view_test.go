package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const groupRatioViewUserId = 7

// seedGroupRatioView installs a "vip" user whose usable groups span all three
// ratio tiers: "svip" carries a user-exclusive ratio (0.3) that must beat the
// vip->svip group-special ratio (0.5), "special" resolves to the group-special
// ratio (0.9), and "default" falls back to the plain group ratio (1).
func seedGroupRatioView(t *testing.T, userSetting dto.UserSetting) {
	t.Helper()
	setupModelListControllerTestDB(t)

	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":2,"special":3}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"vip":{"svip":0.5,"special":0.9}}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","svip":"超级会员","special":"特殊分组"}`))

	user := model.User{Id: groupRatioViewUserId, Username: "ratio-view", Group: "vip"}
	user.SetSetting(userSetting)
	require.NoError(t, model.DB.Create(&user).Error)
}

// userGroupRatios drives GET /api/user/self/groups and returns groupName -> ratio
// as the frontend group selector receives it. The synthetic "auto" entry carries
// a string ratio, so only numeric entries are collected.
func userGroupRatios(t *testing.T, userId int) map[string]float64 {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", userId)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/self/groups", nil)

	GetUserGroups(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    map[string]struct {
			Ratio any    `json:"ratio"`
			Desc  string `json:"desc"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)

	ratios := map[string]float64{}
	for name, entry := range payload.Data {
		if ratio, ok := entry.Ratio.(float64); ok {
			ratios[name] = ratio
		}
	}
	return ratios
}

// pricingGroupRatios drives GET /api/pricing and returns the group_ratio map the
// console plaza renders.
func pricingGroupRatios(t *testing.T, userId int) map[string]float64 {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	if userId != 0 {
		ctx.Set("id", userId)
	}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/pricing", nil)

	GetPricing(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success    bool               `json:"success"`
		GroupRatio map[string]float64 `json:"group_ratio"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	return payload.GroupRatio
}

// TestGroupRatioViewsApplyUserExclusiveRatio locks the contract that the ratio a
// user is shown equals the ratio they are billed at: both read paths must resolve
// the same three tiers as billing (user exclusive > group special > group ratio).
// Regression: both endpoints previously resolved only the bottom two tiers, so a
// user with an exclusive ratio saw the group's ratio but was charged their own.
func TestGroupRatioViewsApplyUserExclusiveRatio(t *testing.T) {
	seedGroupRatioView(t, dto.UserSetting{GroupRatio: map[string]float64{"svip": 0.3}})

	want := map[string]float64{
		"svip":    0.3, // user exclusive ratio beats the vip->svip special ratio
		"special": 0.9, // no exclusive entry: group special ratio
		"default": 1,   // neither: plain group ratio
		"vip":     1,   // the user's own group is always usable
	}

	assert.Equal(t, want, userGroupRatios(t, groupRatioViewUserId), "group list ratios")
	assert.Equal(t, want, pricingGroupRatios(t, groupRatioViewUserId), "console plaza ratios")
}

// TestGroupRatioViewsUserExclusiveZeroIsFree guards the free tier: 0 is a valid
// exclusive ratio and must not be mistaken for "unset" and replaced by fallback.
func TestGroupRatioViewsUserExclusiveZeroIsFree(t *testing.T) {
	seedGroupRatioView(t, dto.UserSetting{GroupRatio: map[string]float64{"svip": 0}})

	assert.Equal(t, float64(0), userGroupRatios(t, groupRatioViewUserId)["svip"], "group list ratio")
	assert.Equal(t, float64(0), pricingGroupRatios(t, groupRatioViewUserId)["svip"], "console plaza ratio")
}

// TestGroupRatioViewsWithoutUserExclusiveRatio pins the fallback behaviour for a
// user with no exclusive ratio configured, so the fix cannot leak an override
// onto users who have none.
func TestGroupRatioViewsWithoutUserExclusiveRatio(t *testing.T) {
	seedGroupRatioView(t, dto.UserSetting{})

	want := map[string]float64{"svip": 0.5, "special": 0.9, "default": 1, "vip": 1}

	assert.Equal(t, want, userGroupRatios(t, groupRatioViewUserId), "group list ratios")
	assert.Equal(t, want, pricingGroupRatios(t, groupRatioViewUserId), "console plaza ratios")
}

// TestPricingAnonymousUsesPlainGroupRatios covers the unauthenticated plaza: with
// no user in context there is no user group, so every group shows its plain ratio.
func TestPricingAnonymousUsesPlainGroupRatios(t *testing.T) {
	seedGroupRatioView(t, dto.UserSetting{GroupRatio: map[string]float64{"svip": 0.3}})

	assert.Equal(t, map[string]float64{"svip": 2, "special": 3, "default": 1}, pricingGroupRatios(t, 0))
}
