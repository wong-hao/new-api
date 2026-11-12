package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/msgcenter"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// messageChannels picks the channels one notice goes out through: what the site sends
// through, narrowed to the user's own choice when they made one, minus any channel whose
// required field the site has not configured (EIP also needs a message source). A
// channel the centre cannot fill in fails the whole message, taking down the channels
// that would have worked, so an unusable one is dropped instead.
func messageChannels(settings *system_setting.MessageCenterSettings, chosen string, userId int) ([]string, string) {
	channels := settings.EffectiveChannels()
	if chosen = strings.TrimSpace(chosen); chosen != "" {
		channels = slices.DeleteFunc(channels, func(channel string) bool {
			return !slices.Contains(strings.Split(chosen, ","), channel)
		})
	}

	moduleID := settings.EffectiveModuleID()
	if slices.Contains(channels, msgcenter.ChannelEIP) && moduleID == "" {
		common.SysLog(common.LogText("the message center EIP channel is selected for user %d but no message source is configured, sending without it", userId))
		channels = slices.DeleteFunc(channels, func(channel string) bool {
			return channel == msgcenter.ChannelEIP
		})
	}
	return channels, moduleID
}

// sendOANotify delivers a notice through the campus message center, addressed to the
// recipient's campus account (their CAS unified account, the 学工号 the center files
// messages under).
//
// A site that has not configured the center, or a recipient who has no campus account
// linked, is not an error: the notice is simply not deliverable, and the caller's other
// work must not depend on it.
func sendOANotify(userId int, userSetting dto.UserSetting, data dto.Notify) error {
	settings := system_setting.GetMessageCenterSettings()
	if !settings.IsConfigured() {
		common.SysLog(common.LogText("message center is not configured, skip sending OA notify to user %d", userId))
		return nil
	}

	user, err := model.GetUserById(userId, false)
	if err != nil {
		return fmt.Errorf("failed to load user %d for OA notify: %w", userId, err)
	}
	if strings.TrimSpace(user.CasId) == "" {
		common.SysLog(common.LogText("user %d has no campus account, skip sending OA notify", userId))
		return nil
	}

	channels, moduleID := messageChannels(settings, userSetting.OAChannels, userId)
	if len(channels) == 0 {
		common.SysLog(common.LogText("no channel is left to reach user %d, skip sending OA notify", userId))
		return nil
	}

	content := data.Content
	for _, value := range data.Values {
		content = strings.Replace(content, dto.ContentValueParam, fmt.Sprintf("%v", value), 1)
	}
	// A campus message lands in a shared portal or app next to messages from other
	// systems, so it names the site that sent it, the way email already goes out in the
	// site's name.
	if siteName := strings.TrimSpace(common.SystemName); siteName != "" {
		content = "【" + siteName + "】" + content
	}

	topUpURL := PaymentReturnURL("/wallet")
	message := &msgcenter.Message{
		Title:       data.Title,
		Content:     content,
		Channels:    strings.Join(channels, ","),
		IntReceiver: user.CasId,
		BtnText:     strings.TrimSpace(settings.ButtonText),
		PCURL:       topUpURL,
		MobileURL:   topUpURL,
		Template:    settings.EffectiveTemplate(),
	}
	// Each channel carries its own required field: the centre rejects the message when
	// one is missing, so it is filled in from the channel list the message sends to.
	for _, channel := range channels {
		switch channel {
		case msgcenter.ChannelWeChat:
			message.WeChatMsgType = msgcenter.WeChatMsgTypeText
		case msgcenter.ChannelDingTalk:
			message.DingMsgType = msgcenter.DingTalkMsgTypeText
		case msgcenter.ChannelEIP:
			message.ModuleID = moduleID
		}
	}

	client := msgcenter.NewClient(settings.EffectiveBaseURL(), settings.AppID, settings.Token)
	msgId, err := client.SendMsg(context.Background(), message)
	if err != nil {
		var apiErr *msgcenter.APIError
		if errors.As(err, &apiErr) {
			return fmt.Errorf("message center rejected the OA notify for user %d: code %d: %s", userId, apiErr.Code, apiErr.Message)
		}
		return err
	}

	common.SysLog(common.LogText("OA notify sent to user %d: %s", userId, msgId))
	return nil
}
