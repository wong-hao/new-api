package system_setting

import (
	"net/url"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/msgcenter"
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
	// Channels are the channels this site sends through. The centre accepts only the
	// ones the application is authorized for — getChannels reports them — and sending
	// one it is not authorized for fails the whole message, so this stays an explicit
	// short list instead of trying every channel.
	Channels string `json:"channels"`
	// ModuleID is the message source EIP requires: 流程/通知/调查/会议/第三方.
	ModuleID string `json:"module_id"`
	// WeChatMsgType is what the wechat channel sends: plain text, a news card
	// (图文) or a text card. A news card needs the centre to have a news
	// configuration for the application, so plain text is the fallback.
	WeChatMsgType string `json:"wechat_msg_type"`
	// ButtonText is the label the centre shows on a message it draws as a card, such as
	// 更多 or 查看详情. A channel that renders plain text ignores it.
	ButtonText string `json:"button_text"`
	// Template names the centre's template code per channel, as the JSON object the send
	// API takes (`{"eip":"someCode"}`). The template itself lives in the centre; a code it
	// has not registered is rejected (6010628), so this stays empty until one exists.
	Template string `json:"template"`
}

// DeliveryChannels are the channels a notice can reach a campus account by, in the
// order they are sent, since each addresses the recipient's unified account. The
// centre's other channels need a phone number, an email address or a file.
var DeliveryChannels = []string{
	msgcenter.ChannelWeChat,
	msgcenter.ChannelDingTalk,
	msgcenter.ChannelEIP,
	msgcenter.ChannelApp,
}

// WeChatMsgTypes are the message types the wechat channel can send. A file message
// needs an uploaded fileId, which the site never sends, so it is not offered.
var WeChatMsgTypes = []string{
	msgcenter.WeChatMsgTypeText,
	msgcenter.WeChatMsgTypeNews,
	msgcenter.WeChatMsgTypeTextCard,
}

// EIPModuleIDs are the message sources the centre accepts for the EIP channel.
var EIPModuleIDs = []string{
	msgcenter.ModuleCooperate,
	msgcenter.ModuleAnnouncement,
	msgcenter.ModuleSurvey,
	msgcenter.ModuleMeeting,
	msgcenter.ModuleThirdPartyApp,
}

// defaultMessageCenterChannels is what an unconfigured site sends through. WeChat is
// the channel every centre seen so far authorizes and the only one needing no further
// field, so it is the safe single channel to fall back to.
const defaultMessageCenterChannels = msgcenter.ChannelWeChat

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

// EffectiveChannels returns the channels this site sends through, in DeliveryChannels
// order, with anything outside that vocabulary dropped. An unset or unusable value
// falls back to WeChat alone.
func (s *MessageCenterSettings) EffectiveChannels() []string {
	configured := make([]string, 0, len(DeliveryChannels))
	for channel := range strings.SplitSeq(s.Channels, ",") {
		if channel = strings.TrimSpace(channel); channel != "" {
			configured = append(configured, channel)
		}
	}

	channels := slices.DeleteFunc(slices.Clone(DeliveryChannels), func(channel string) bool {
		return !slices.Contains(configured, channel)
	})
	if len(channels) == 0 {
		return []string{defaultMessageCenterChannels}
	}
	return channels
}

// EffectiveModuleID returns the configured EIP message source, or "" when it is not one
// the centre accepts.
func (s *MessageCenterSettings) EffectiveModuleID() string {
	moduleID := strings.TrimSpace(s.ModuleID)
	if slices.Contains(EIPModuleIDs, moduleID) {
		return moduleID
	}
	return ""
}

// EffectiveWeChatMsgType returns the configured WeChat message type, or plain text
// when it is unset or not one the centre accepts.
func (s *MessageCenterSettings) EffectiveWeChatMsgType() string {
	msgType := strings.TrimSpace(s.WeChatMsgType)
	if slices.Contains(WeChatMsgTypes, msgType) {
		return msgType
	}
	return msgcenter.WeChatMsgTypeText
}

// EffectiveTemplate returns the configured template codes as the JSON object the centre
// takes, or "" when nothing usable is configured. A value that is not a JSON object of
// channel to code is treated as unset rather than failing every message.
func (s *MessageCenterSettings) EffectiveTemplate() string {
	template := strings.TrimSpace(s.Template)
	if template == "" {
		return ""
	}
	var codes map[string]string
	if err := common.UnmarshalJsonStr(template, &codes); err != nil || len(codes) == 0 {
		return ""
	}
	return template
}

// IsConfigured reports whether a message can actually be delivered: the feature is
// switched on and every value an authorized call needs is present.
func (s *MessageCenterSettings) IsConfigured() bool {
	return s.Enabled && s.EffectiveBaseURL() != "" && strings.TrimSpace(s.AppID) != "" && strings.TrimSpace(s.Token) != ""
}
