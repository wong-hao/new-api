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

// oaChannels are the message center channels that reach a campus account by its
// unified account number, which is the only recipient identity this site has. The
// other channels the center offers need a phone number, an email address, or an
// EIP message source, so a user cannot pick them here yet.
var oaChannels = []string{msgcenter.ChannelWeChat, msgcenter.ChannelDingTalk}

// defaultOAChannels is used when a user chose the OA method without narrowing down
// the channels, so choosing the method alone is enough to receive messages.
const defaultOAChannels = msgcenter.ChannelWeChat + "," + msgcenter.ChannelDingTalk

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

	content := data.Content
	for _, value := range data.Values {
		content = strings.Replace(content, dto.ContentValueParam, fmt.Sprintf("%v", value), 1)
	}

	topUpURL := PaymentReturnURL("/wallet")
	client := msgcenter.NewClient(settings.EffectiveBaseURL(), settings.AppID, settings.Token)
	msgId, err := client.SendMsg(context.Background(), &msgcenter.Message{
		Title:       data.Title,
		Content:     content,
		Channels:    allowedOAChannels(userSetting.OAChannels),
		IntReceiver: user.CasId,
		PCURL:       topUpURL,
		MobileURL:   topUpURL,
	})
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

// allowedOAChannels keeps the channels a user may choose from, dropping anything else
// a stored setting might carry. An empty result falls back to the default channels.
func allowedOAChannels(configured string) string {
	kept := make([]string, 0, len(oaChannels))
	for channel := range strings.SplitSeq(configured, ",") {
		channel = strings.TrimSpace(channel)
		if channel != "" && slices.Contains(oaChannels, channel) {
			kept = append(kept, channel)
		}
	}
	if len(kept) == 0 {
		return defaultOAChannels
	}
	return strings.Join(kept, ",")
}
