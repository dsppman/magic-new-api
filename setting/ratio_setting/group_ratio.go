package ratio_setting

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

// MaxUserSpecialRatio 是用户专属倍率允许的上界。倍率最终经 decimal 乘入 32 位额度列，
// 溢出会被饱和转换拦截并记日志，这里再加一道防御上界以拒绝明显的误配置。
const MaxUserSpecialRatio = 100000

var defaultGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

var groupRatioMap = types.NewRWMap[string, float64]()

var defaultGroupGroupRatio = map[string]map[string]float64{
	"vip": {
		"edit_this": 0.9,
	},
}

var groupGroupRatioMap = types.NewRWMap[string, map[string]float64]()

var defaultGroupSpecialUsableGroup = map[string]map[string]string{}

type GroupRatioSetting struct {
	GroupRatio              *types.RWMap[string, float64]            `json:"group_ratio"`
	GroupGroupRatio         *types.RWMap[string, map[string]float64] `json:"group_group_ratio"`
	GroupSpecialUsableGroup *types.RWMap[string, map[string]string]  `json:"group_special_usable_group"`
}

var groupRatioSetting GroupRatioSetting

func init() {
	groupSpecialUsableGroup := types.NewRWMap[string, map[string]string]()
	groupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)

	groupRatioMap.AddAll(defaultGroupRatio)
	groupGroupRatioMap.AddAll(defaultGroupGroupRatio)

	groupRatioSetting = GroupRatioSetting{
		GroupSpecialUsableGroup: groupSpecialUsableGroup,
		GroupRatio:              groupRatioMap,
		GroupGroupRatio:         groupGroupRatioMap,
	}

	config.GlobalConfig.Register("group_ratio_setting", &groupRatioSetting)
}

func GetGroupRatioSetting() *GroupRatioSetting {
	if groupRatioSetting.GroupSpecialUsableGroup == nil {
		groupRatioSetting.GroupSpecialUsableGroup = types.NewRWMap[string, map[string]string]()
		groupRatioSetting.GroupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)
	}
	return &groupRatioSetting
}

func GetGroupRatioCopy() map[string]float64 {
	return groupRatioMap.ReadAll()
}

func ContainsGroupRatio(name string) bool {
	_, ok := groupRatioMap.Get(name)
	return ok
}

func GroupRatio2JSONString() string {
	return groupRatioMap.MarshalJSONString()
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupRatioMap, jsonStr)
}

func GetGroupRatio(name string) float64 {
	ratio, ok := groupRatioMap.Get(name)
	if !ok {
		common.SysLog("group ratio not found: " + name)
		return 1
	}
	return ratio
}

// ValidRatioValue 判断 r 是否为可用倍率：有限且不小于 0。允许 0（表示免费），与分组倍率语义一致。
func ValidRatioValue(r float64) bool {
	return !math.IsNaN(r) && !math.IsInf(r, 0) && r >= 0
}

// CheckUserSpecialRatio 校验管理员设置的用户专属倍率：必须有限、不小于 0，且不超过上界。
func CheckUserSpecialRatio(ratio float64) error {
	if !ValidRatioValue(ratio) {
		return errors.New("user special ratio must be a finite number not less than 0")
	}
	if ratio > MaxUserSpecialRatio {
		return fmt.Errorf("user special ratio must not exceed %d", MaxUserSpecialRatio)
	}
	return nil
}

// GetEffectiveGroupRatioInfo 按三级优先级解析本次请求的分组倍率：
// 用户专属倍率（最高） > 分组特殊倍率(GroupGroupRatio) > 普通分组倍率(GroupRatio)。
// userSpecialRatios 为该用户的「使用分组 -> 倍率」专属映射（未配置时为 nil）；只有命中
// 当前 usingGroup 的条目才生效，与 GroupGroupRatio 语义一致，只是键控到具体用户。
// 若命中值非法（负数/NaN/Inf）则忽略并回退，确保错误配置永远不会算出负额度（credit）。
// usingGroup 应为已完成 auto 分组解析后的最终分组。
func GetEffectiveGroupRatioInfo(userSpecialRatios map[string]float64, userGroup, usingGroup string) types.GroupRatioInfo {
	info := types.GroupRatioInfo{
		GroupRatio:        1.0,
		GroupSpecialRatio: -1,
		UserSpecialRatio:  -1,
	}
	if ratio, ok := userSpecialRatios[usingGroup]; ok {
		if ValidRatioValue(ratio) {
			info.GroupRatio = ratio
			info.UserSpecialRatio = ratio
			info.HasUserSpecialRatio = true
			return info
		}
		common.SysError(fmt.Sprintf("ignored invalid user special ratio %v for using group %q (user group %q)", ratio, usingGroup, userGroup))
	}
	if ratio, ok := GetGroupGroupRatio(userGroup, usingGroup); ok {
		info.GroupRatio = ratio
		info.GroupSpecialRatio = ratio
		info.HasSpecialRatio = true
		return info
	}
	info.GroupRatio = GetGroupRatio(usingGroup)
	return info
}

func GetGroupGroupRatio(userGroup, usingGroup string) (float64, bool) {
	gp, ok := groupGroupRatioMap.Get(userGroup)
	if !ok {
		return -1, false
	}
	ratio, ok := gp[usingGroup]
	if !ok {
		return -1, false
	}
	return ratio, true
}

func GroupGroupRatio2JSONString() string {
	return groupGroupRatioMap.MarshalJSONString()
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupGroupRatioMap, jsonStr)
}

func CheckGroupRatio(jsonStr string) error {
	checkGroupRatio := make(map[string]float64)
	err := json.Unmarshal([]byte(jsonStr), &checkGroupRatio)
	if err != nil {
		return err
	}
	for name, ratio := range checkGroupRatio {
		if ratio < 0 {
			return errors.New("group ratio must be not less than 0: " + name)
		}
	}
	return nil
}
