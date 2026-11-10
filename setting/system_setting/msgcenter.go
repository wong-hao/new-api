package system_setting

import (
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// MessageCenterSettings holds the credentials of the campus message center (新消息中心,
// served under /htmq/) that delivers notices over WeChat, DingTalk, EIP, SMS, email and
// the campus app. One authorized application covers the whole site, so these are site
// settings rather than per-user ones; a user only chooses which channels reach them.
//
// BaseURL is the deployment root including its port. The interface document's text and
// its own examples disagree on that port (9200 against 8080), so no default is assumed:
// a deployment that has not been told the address is simply "not configured".
type MessageCenterSettings struct {
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"base_url"`
	AppID   string `json:"app_id"`
	Token   string `json:"token"`
}

// 默认配置
var defaultMessageCenterSettings = MessageCenterSettings{}

func init() {
	// 注册到全局配置管理器，option 键为 msgcenter.enabled / msgcenter.base_url /
	// msgcenter.app_id / msgcenter.token
	config.GlobalConfig.Register("msgcenter", &defaultMessageCenterSettings)
}

func GetMessageCenterSettings() *MessageCenterSettings {
	return &defaultMessageCenterSettings
}

// EffectiveBaseURL returns the configured deployment root without a trailing slash, or
// "" when it is unusable. A value that is not an absolute http(s) URL is rejected here
// rather than at send time, so a misconfigured center is treated as "not configured"
// instead of failing every notification.
func (s *MessageCenterSettings) EffectiveBaseURL() string {
	trimmed := strings.TrimRight(strings.TrimSpace(s.BaseURL), "/")
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	return trimmed
}

// IsConfigured reports whether a message can actually be delivered: the feature is
// switched on and every value an authorized call needs is present.
func (s *MessageCenterSettings) IsConfigured() bool {
	return s.Enabled && s.EffectiveBaseURL() != "" && strings.TrimSpace(s.AppID) != "" && strings.TrimSpace(s.Token) != ""
}
