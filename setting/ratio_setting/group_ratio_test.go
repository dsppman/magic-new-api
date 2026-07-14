package ratio_setting

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fp(v float64) *float64 { return &v }

// seedRatios 显式初始化分组倍率与分组特殊倍率，供解析器测试使用。
func seedRatios(t *testing.T) {
	t.Helper()
	require.NoError(t, UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{"vip":{"special":0.9}}`))
}

// TestGetEffectiveGroupRatioInfo 锁定三级倍率优先级这一计费不变量：
// 用户专属倍率 > 分组特殊倍率 > 普通分组倍率，且非法专属倍率必须被忽略以防算出负额度。
func TestGetEffectiveGroupRatioInfo(t *testing.T) {
	seedRatios(t)

	cases := []struct {
		name             string
		userSpecial      *float64
		userGroup        string
		usingGroup       string
		wantRatio        float64
		wantHasUser      bool
		wantUserSpecial  float64
		wantHasGroupSpec bool
	}{
		{
			name:            "user special ratio wins over group ratio",
			userSpecial:     fp(0.5),
			userGroup:       "default",
			usingGroup:      "default",
			wantRatio:       0.5,
			wantHasUser:     true,
			wantUserSpecial: 0.5,
		},
		{
			name:            "user special ratio zero means free and is allowed",
			userSpecial:     fp(0),
			userGroup:       "default",
			usingGroup:      "default",
			wantRatio:       0,
			wantHasUser:     true,
			wantUserSpecial: 0,
		},
		{
			name:            "user special ratio wins even over group special ratio",
			userSpecial:     fp(0.3),
			userGroup:       "vip",
			usingGroup:      "special",
			wantRatio:       0.3,
			wantHasUser:     true,
			wantUserSpecial: 0.3,
		},
		{
			name:             "no user special falls back to group special ratio",
			userSpecial:      nil,
			userGroup:        "vip",
			usingGroup:       "special",
			wantRatio:        0.9,
			wantHasUser:      false,
			wantUserSpecial:  -1,
			wantHasGroupSpec: true,
		},
		{
			name:            "no user special no group special falls back to group ratio",
			userSpecial:     nil,
			userGroup:       "default",
			usingGroup:      "vip",
			wantRatio:       2,
			wantHasUser:     false,
			wantUserSpecial: -1,
		},
		{
			name:            "negative user special is ignored and falls back",
			userSpecial:     fp(-1),
			userGroup:       "default",
			usingGroup:      "vip",
			wantRatio:       2,
			wantHasUser:     false,
			wantUserSpecial: -1,
		},
		{
			name:            "NaN user special is ignored and falls back",
			userSpecial:     fp(math.NaN()),
			userGroup:       "default",
			usingGroup:      "vip",
			wantRatio:       2,
			wantHasUser:     false,
			wantUserSpecial: -1,
		},
		{
			name:            "Inf user special is ignored and falls back",
			userSpecial:     fp(math.Inf(1)),
			userGroup:       "default",
			usingGroup:      "vip",
			wantRatio:       2,
			wantHasUser:     false,
			wantUserSpecial: -1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := GetEffectiveGroupRatioInfo(tc.userSpecial, tc.userGroup, tc.usingGroup)
			assert.Equal(t, tc.wantRatio, info.GroupRatio, "GroupRatio")
			assert.Equal(t, tc.wantHasUser, info.HasUserSpecialRatio, "HasUserSpecialRatio")
			assert.Equal(t, tc.wantUserSpecial, info.UserSpecialRatio, "UserSpecialRatio")
			assert.Equal(t, tc.wantHasGroupSpec, info.HasSpecialRatio, "HasSpecialRatio")
		})
	}
}

func TestCheckUserSpecialRatio(t *testing.T) {
	cases := []struct {
		name    string
		ratio   float64
		wantErr bool
	}{
		{"zero is allowed (free)", 0, false},
		{"positive is allowed", 0.5, false},
		{"upper bound is allowed", MaxUserSpecialRatio, false},
		{"negative is rejected", -0.1, true},
		{"above upper bound is rejected", MaxUserSpecialRatio + 1, true},
		{"NaN is rejected", math.NaN(), true},
		{"positive Inf is rejected", math.Inf(1), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckUserSpecialRatio(tc.ratio)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
