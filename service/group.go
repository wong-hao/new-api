package service

import (
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func GetUserUsableGroups(userGroup string) map[string]string {
	groupsCopy := setting.GetUserUsableGroupsCopy()
	if userGroup != "" {
		specialSettings, b := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)
		if b {
			// 处理特殊可用分组
			for specialGroup, desc := range specialSettings {
				if after, ok := strings.CutPrefix(specialGroup, "-:"); ok {
					// 移除分组
					groupToRemove := after
					delete(groupsCopy, groupToRemove)
				} else if after, ok := strings.CutPrefix(specialGroup, "+:"); ok {
					// 添加分组
					groupToAdd := after
					groupsCopy[groupToAdd] = desc
				} else {
					// 直接添加分组
					groupsCopy[specialGroup] = desc
				}
			}
		}
		// 如果userGroup不在UserUsableGroups中，返回UserUsableGroups + userGroup。
		// 描述是英文源文本，由 web 控制台翻译；管理员配置的描述原样显示。
		if _, ok := groupsCopy[userGroup]; !ok {
			groupsCopy[userGroup] = "User group"
		}
	}
	return groupsCopy
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := GetUserUsableGroups(userGroup)[groupName]
	return ok
}

func IsUserSelectableGroup(userGroup, groupName string) bool {
	if groupName == "" || groupName == "auto" {
		return false
	}
	return GroupInUserUsableGroups(userGroup, groupName) && ratio_setting.ContainsGroupRatio(groupName)
}

// GetUserAutoGroup 根据用户分组获取自动分组设置
func GetUserAutoGroup(userGroup string) []string {
	autoGroups := make([]string, 0)
	seen := make(map[string]struct{})
	for _, group := range setting.GetAutoGroups() {
		if !IsUserSelectableGroup(userGroup, group) {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		autoGroups = append(autoGroups, group)
	}
	return autoGroups
}

// FilterUserTokenAutoGroups applies current permissions before the current
// per-token limit. It intentionally does not fall back to the global Auto list.
func FilterUserTokenAutoGroups(userGroup string, groups []string) []string {
	maxCount := setting.GetMaxTokenAutoGroups()
	filtered := make([]string, 0, min(len(groups), maxCount))
	seen := make(map[string]struct{})
	for _, group := range groups {
		if !IsUserSelectableGroup(userGroup, group) {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		filtered = append(filtered, group)
		if len(filtered) == maxCount {
			break
		}
	}
	return filtered
}

// GetRequestAutoGroups resolves the ordered Auto groups for the current token.
// The absence of the context value means that the token inherits the complete
// global Auto list; a present (even empty) value is an explicit token snapshot.
func GetRequestAutoGroups(c *gin.Context, userGroup string) []string {
	value, ok := common.GetContextKey(c, constant.ContextKeyTokenAutoGroups)
	if !ok {
		return GetUserAutoGroup(userGroup)
	}
	groups, ok := value.([]string)
	if !ok {
		return []string{}
	}
	return FilterUserTokenAutoGroups(userGroup, groups)
}

// GetGroupsEnabledModels 按 groups 顺序获取各分组启用的模型并去重
func GetGroupsEnabledModels(groups []string) []string {
	seen := make(map[string]struct{})
	models := make([]string, 0)
	for _, group := range groups {
		for _, modelName := range model.GetGroupEnabledModels(group) {
			if _, ok := seen[modelName]; !ok {
				seen[modelName] = struct{}{}
				models = append(models, modelName)
			}
		}
	}
	return models
}

// GetUserGroupRatio 获取用户使用某个分组的倍率
// userGroup 用户分组
// group 需要获取倍率的分组
func GetUserGroupRatio(userGroup, group string) float64 {
	ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, group)
	if ok {
		return ratio
	}
	return ratio_setting.GetGroupRatio(group)
}

const (
	// defaultUserGroup is the group every account starts in, and the one an
	// asserted group copies its ratio from.
	defaultUserGroup = "default"
	// maxGroupNameLength matches users.group, a varchar(64) column: a longer name
	// would be truncated or rejected by the database.
	maxGroupNameLength = 64
)

// assertedGroupMutex serializes the read-modify-write that adds a group to the
// group ratio settings, so two sign-ins from different departments at the same
// moment cannot drop each other's group.
var assertedGroupMutex sync.Mutex

// AssignAssertedGroup places an account in the group an external identity source
// named for it, creating that group when it does not exist yet. It reports
// whether the account moved, so the caller can record the change.
//
// The asserted group is authoritative on every sign-in, which is what makes a
// department transfer follow the account. It therefore also undoes a group an
// administrator set by hand: a deployment that needs a hand-set group to stand on
// its own must not name a group attribute. A sign-in that asserts the group the
// account already has moves nothing, though it still records the ratio a group is
// missing.
func AssignAssertedGroup(user *model.User, group string) (bool, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return false, nil
	}
	if utf8.RuneCountInString(group) > maxGroupNameLength {
		common.SysLog(common.LogText("asserted group name is too long, ignored for user %d", user.Id))
		return false, nil
	}
	if err := ensureGroupRatio(group); err != nil {
		return false, err
	}
	if group == user.Group {
		return false, nil
	}
	user.Group = group
	return true, user.Edit(false)
}

// ensureGroupRatio makes sure a group can be billed for, copying the default
// group's ratio. A group with no ratio of its own logs a missing ratio and is
// charged as the default group anyway, so the entry makes the arrangement visible
// in the group ratio settings, where an administrator can price it separately.
func ensureGroupRatio(group string) error {
	assertedGroupMutex.Lock()
	defer assertedGroupMutex.Unlock()

	if ratio_setting.ContainsGroupRatio(group) {
		return nil
	}
	ratios := ratio_setting.GetGroupRatioCopy()
	ratios[group] = ratio_setting.GetGroupRatio(defaultUserGroup)
	encoded, err := common.Marshal(ratios)
	if err != nil {
		return err
	}
	if err := model.UpdateOption("GroupRatio", string(encoded)); err != nil {
		return err
	}
	common.SysLog(common.LogText("created group %s with the default group's ratio", group))
	return nil
}
