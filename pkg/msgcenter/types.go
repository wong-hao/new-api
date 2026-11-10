package msgcenter

import (
	"fmt"
	"strings"
)

// Delivery channels accepted by the message center.
const (
	ChannelSMS      = "sms"
	ChannelEmail    = "email"
	ChannelWeChat   = "wechat"
	ChannelDingTalk = "dingding"
	ChannelEIP      = "eip"
	ChannelApp      = "app"
)

// WeChat message types, valid in Message.WeChatMsgType when the wechat channel is used.
const (
	WeChatMsgTypeText     = "text"
	WeChatMsgTypeNews     = "news"
	WeChatMsgTypeFile     = "file"
	WeChatMsgTypeTextCard = "textcard"
)

// DingTalk message types, valid in Message.DingMsgType when the dingding channel is used.
const (
	DingTalkMsgTypeText       = "text"
	DingTalkMsgTypeLink       = "link"
	DingTalkMsgTypeFile       = "file"
	DingTalkMsgTypeActionCard = "action_card"
)

// Message sources, valid in Message.ModuleID when the eip channel is used.
const (
	ModuleCooperate     = "COOPERATE"
	ModuleAnnouncement  = "ANNOUNCEMENT"
	ModuleSurvey        = "SURVEY"
	ModuleMeeting       = "MEET"
	ModuleThirdPartyApp = "JOIN_APP"
)

// SuccessCode is the code the message center returns for a successful call.
const SuccessCode = 200

// Codes worth branching on: an invalid or expired appId/token, a request the
// center rejects as incomplete, a rejected channel permission, and the daily
// send quota. The full table lives in api.md.
const (
	CodeDomainInvalid     = 6010403 // 未经授权访问 / 拒绝访问
	CodeMissingParam      = 6010521 // 缺少必要参数
	CodeAppInvalid        = 6010604 // 无效的应用(appId不正确、不在有效期内、已停用)
	CodeChannelNotAllowed = 6010603 // 没有开通渠道权限
	CodeDailyQuotaReached = 6010624 // 今日发送消息次数已达上限，无法发送
)

// APIError is a non-success code returned by the message center. Message is the
// center's own text, in Chinese, with any {0}/{1} placeholders already filled in.
type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("msgcenter: code %d: %s", e.Code, e.Message)
}

// Authorization is what GetChannels reports: the channels this appId may send
// through, and the validity window the center describes as a display string.
type Authorization struct {
	Channels  []string
	ValidDate string
}

// Message is the msgData payload of SendMsg. The center treats an empty field as
// "use the server-side default", and its own example sends every field, so the
// JSON always carries the full set and only Cron is defaulted here.
type Message struct {
	Title         string `json:"title"`
	Content       string `json:"content"`
	Channels      string `json:"channels"`
	IntReceiver   string `json:"intReceiver"`
	SMSReceiver   string `json:"smsReceiver"`
	EmailReceiver string `json:"emailReceiver"`
	WeChatMsgType string `json:"wechatMsgType"`
	DingMsgType   string `json:"dingMsgType"`
	ModuleID      string `json:"moduleID"`
	BtnText       string `json:"btntext"`
	WeChatAgentID string `json:"wechatAgentId"`
	DingAgentID   string `json:"dingAgentId"`
	// Cron is "1" for a scheduled send and "0" for an immediate one. SendMsg
	// coerces an empty value to "0".
	Cron      string `json:"cron"`
	SendTime  string `json:"sendTime"`
	PCURL     string `json:"pcUrl"`
	MobileURL string `json:"mobileUrl"`
	FileID    string `json:"fileId"`
	Template  string `json:"template"`
}

// splitChannels turns the center's comma-separated channel list into a slice,
// dropping empty entries.
func splitChannels(raw string) []string {
	channels := make([]string, 0, 4)
	for channel := range strings.SplitSeq(raw, ",") {
		if channel = strings.TrimSpace(channel); channel != "" {
			channels = append(channels, channel)
		}
	}
	return channels
}
