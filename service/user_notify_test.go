package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/msgcenter"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// notifyTestContext is the request context a billing path needs; checking a
// relayInfo's billing preference and logging to the request's language only read
// what a test context already provides.
func notifyTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

// A low-balance reminder goes out once, on the charge that takes a wallet or
// subscription balance below the threshold. A balance already below it earns
// nothing more until a top-up lifts it back above the line. The site switch
// and the user's switch both turn reminders off; a user who never chose gets
// them.
func TestQuotaReminderFiresOncePerDrop(t *testing.T) {
	previousEnabled, previousThreshold := common.QuotaRemindEnabled, common.QuotaRemindThreshold
	t.Cleanup(func() {
		common.QuotaRemindEnabled, common.QuotaRemindThreshold = previousEnabled, previousThreshold
	})
	common.QuotaRemindThreshold = 1000
	on, off := true, false
	// changes move the balance from start: negative ones are charges, which
	// may remind; positive ones are top-ups. want holds one entry per charge.
	for _, tc := range []struct {
		name        string
		siteEnabled bool
		setting     dto.UserSetting
		start       int64
		changes     []int64
		want        []bool
	}{
		{
			name:        "never chosen: one reminder per drop, again after a top-up",
			siteEnabled: true,
			start:       1500,
			changes:     []int64{-400, -200, -300, +1500, -1200},
			want:        []bool{false, true, false, true},
		},
		{
			name:        "switched on with its own threshold",
			siteEnabled: true,
			setting:     dto.UserSetting{QuotaWarningEnabled: &on, QuotaWarningThreshold: 5000},
			start:       6000,
			changes:     []int64{-500, -600, -2000},
			want:        []bool{false, true, false},
		},
		{
			name:        "a charge that lands exactly on the threshold does not remind",
			siteEnabled: true,
			start:       1200,
			changes:     []int64{-200, -1},
			want:        []bool{false, true},
		},
		{
			name:        "user switched off",
			siteEnabled: true,
			setting:     dto.UserSetting{QuotaWarningEnabled: &off},
			start:       1500,
			changes:     []int64{-600},
			want:        []bool{false},
		},
		{
			name:    "site switched off",
			setting: dto.UserSetting{QuotaWarningEnabled: &on},
			start:   1500,
			changes: []int64{-600},
			want:    []bool{false},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.QuotaRemindEnabled = tc.siteEnabled
			got := make([]bool, 0, len(tc.want))
			balance := tc.start
			for _, change := range tc.changes {
				if change < 0 {
					got = append(got, quotaReminderDue(tc.setting, balance, balance+change))
				}
				balance += change
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

// Channel notices go to every enabled administrator whose channel switch is
// on, in that administrator's saved language. Until an administrator chooses,
// root receives them and other administrators do not; other users never do.
func TestChannelNoticesFollowEachAdministratorsSwitch(t *testing.T) {
	previousDB := model.DB
	previousCache, previousRedis := common.MemoryCacheEnabled, common.RedisEnabled
	previousNotifyLimit := constant.NotifyLimitCount
	previousClient, previousWorker := httpClient, system_setting.WorkerUrl
	fetch := system_setting.GetFetchSetting()
	previousFetch := *fetch
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled, common.RedisEnabled = previousCache, previousRedis
		constant.NotifyLimitCount = previousNotifyLimit
		httpClient, system_setting.WorkerUrl = previousClient, previousWorker
		*fetch = previousFetch
	})
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.User{}))
	model.DB = database
	common.MemoryCacheEnabled, common.RedisEnabled = false, false
	constant.NotifyLimitCount = 100

	var mutex sync.Mutex
	received := map[string][]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload WebhookPayload
		if err := common.DecodeJson(r.Body, &payload); err == nil {
			mutex.Lock()
			recipient := strings.TrimPrefix(r.URL.Path, "/")
			received[recipient] = append(received[recipient], payload.Type+" "+payload.Title)
			mutex.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	httpClient, system_setting.WorkerUrl = server.Client(), ""
	fetch.EnableSSRFProtection = false

	on, off := true, false
	for _, user := range []struct {
		name     string
		role     int
		status   int
		receives *bool
		language string
	}{
		{"root-never-chose", common.RoleRootUser, common.UserStatusEnabled, nil, ""},
		{"root-turned-off", common.RoleRootUser, common.UserStatusEnabled, &off, ""},
		{"admin-never-chose", common.RoleAdminUser, common.UserStatusEnabled, nil, ""},
		{"admin-turned-on", common.RoleAdminUser, common.UserStatusEnabled, &on, "zhCN"},
		{"disabled-admin-turned-on", common.RoleAdminUser, common.UserStatusDisabled, &on, ""},
		{"user-with-switch-stored-on", common.RoleCommonUser, common.UserStatusEnabled, &on, ""},
	} {
		setting, err := common.Marshal(dto.UserSetting{
			NotifyType:           dto.NotifyTypeWebhook,
			WebhookUrl:           server.URL + "/" + user.name,
			ChannelNotifyEnabled: user.receives,
			Language:             user.language,
		})
		require.NoError(t, err)
		require.NoError(t, database.Create(&model.User{Username: user.name, Role: user.role, Status: user.status, Setting: string(setting), AffCode: user.name}).Error)
	}
	channel := &model.Channel{Name: "notice-review", Key: "fixture-key", Type: 1, Status: common.ChannelStatusEnabled, Group: "default", Models: "test-model"}
	require.NoError(t, channel.Insert())

	DisableChannel(types.ChannelError{ChannelId: channel.Id, ChannelName: channel.Name, AutoBan: true}, "upstream failed")
	EnableChannel(channel.Id, "", channel.Name)
	NotifyChannelWatchers(dto.NotifyTypeChannelTest, func(lang string) (string, string) {
		return i18n.Translate(lang, i18n.MsgChannelTestCompletedSubject), i18n.Translate(lang, i18n.MsgChannelTestCompletedContent)
	})

	disabledType := formatNotifyType(channel.Id, common.ChannelStatusAutoDisabled)
	enabledType := formatNotifyType(channel.Id, common.ChannelStatusEnabled)
	assert.Equal(t, map[string][]string{
		"root-never-chose": {
			fmt.Sprintf(`%s Channel "notice-review" (#%d) has been disabled`, disabledType, channel.Id),
			fmt.Sprintf(`%s Channel "notice-review" (#%d) has been enabled`, enabledType, channel.Id),
			"channel_test Channel test completed",
		},
		"admin-turned-on": {
			fmt.Sprintf("%s 通道「notice-review」（#%d）已被禁用", disabledType, channel.Id),
			fmt.Sprintf("%s 通道「notice-review」（#%d）已被启用", enabledType, channel.Id),
			"channel_test 通道测试完成",
		},
	}, received)
}

// newNotifyTestDB is the database fixture notices need: users, with the quota and
// campus account a notice is addressed to or suppressed by.
func newNotifyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousCache, previousRedis := common.MemoryCacheEnabled, common.RedisEnabled
	previousNotifyLimit := constant.NotifyLimitCount
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled, common.RedisEnabled = previousCache, previousRedis
		constant.NotifyLimitCount = previousNotifyLimit
	})

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, database.AutoMigrate(&model.User{}))
	model.DB = database
	common.MemoryCacheEnabled, common.RedisEnabled = false, false
	constant.NotifyLimitCount = 100
	return database
}

// A notice for a user who chose the campus message center is delivered by it:
// the recipient is the campus account the user signed in with, and only the
// channels the center reaches that account by are sent. A center that is not
// configured, or an account with no campus login, gets no message at all.
func TestCampusNotifyGoesToTheMessageCenter(t *testing.T) {
	database := newNotifyTestDB(t)

	settings := system_setting.GetMessageCenterSettings()
	previousSettings := *settings
	t.Cleanup(func() { *settings = previousSettings })

	var mutex sync.Mutex
	sent := make([]map[string]any, 0, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		var payload map[string]any
		require.NoError(t, common.UnmarshalJsonStr(r.PostFormValue("msgData"), &payload))
		mutex.Lock()
		sent = append(sent, map[string]any{
			"path":    r.URL.Path,
			"appId":   r.Header.Get("appId"),
			"token":   r.Header.Get("token"),
			"payload": payload,
		})
		mutex.Unlock()
		_, _ = w.Write([]byte(`{"code":200,"message":"成功","data":"msg-1"}`))
	}))
	t.Cleanup(server.Close)

	for _, tc := range []struct {
		name     string
		enabled  bool
		casId    string
		channels string
		setting  dto.UserSetting
		wantSent bool
	}{
		{
			name:     "chosen method without choosing channels uses the site's channels",
			enabled:  true,
			casId:    "20200001",
			setting:  dto.UserSetting{NotifyType: dto.NotifyTypeCampus},
			wantSent: true,
		},
		{
			name:     "a user narrows the site's channels down to one of them",
			enabled:  true,
			casId:    "20200002",
			channels: "wechat,dingding",
			setting:  dto.UserSetting{NotifyType: dto.NotifyTypeCampus, CampusChannels: "dingding,email"},
			wantSent: true,
		},
		{
			name:     "an account with no campus login is not reachable",
			enabled:  true,
			casId:    "",
			setting:  dto.UserSetting{NotifyType: dto.NotifyTypeCampus},
			wantSent: false,
		},
		{
			name:     "a center that is not configured sends nothing",
			enabled:  false,
			casId:    "20200003",
			setting:  dto.UserSetting{NotifyType: dto.NotifyTypeCampus},
			wantSent: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutex.Lock()
			sent = sent[:0]
			mutex.Unlock()

			*settings = system_setting.MessageCenterSettings{Enabled: tc.enabled, BaseURL: server.URL, AppID: "app-1", Token: "token-1", Channels: tc.channels}
			user := &model.User{Username: "oa-" + tc.casId, CasId: tc.casId, Status: common.UserStatusEnabled, AffCode: "oa-" + tc.casId}
			require.NoError(t, database.Create(user).Error)

			require.NoError(t, NotifyUser(user.Id, "", tc.setting, dto.NewNotify(dto.NotifyTypeQuotaExceed, "额度提醒", "请及时充值", nil)))

			if !tc.wantSent {
				assert.Empty(t, sent)
				return
			}
			require.Len(t, sent, 1)
			assert.Equal(t, "/htmq/sendMsg", sent[0]["path"])
			assert.Equal(t, "app-1", sent[0]["appId"])
			assert.Equal(t, "token-1", sent[0]["token"])
			payload, ok := sent[0]["payload"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tc.casId, payload["intReceiver"])
			assert.Equal(t, "【"+common.SystemName+"】额度提醒", payload["title"])
			assert.Equal(t, "【"+common.SystemName+"】请及时充值", payload["content"])
			assert.Equal(t, "0", payload["cron"])
			if tc.setting.CampusChannels == "" {
				assert.Equal(t, "wechat", payload["channels"])
			} else {
				assert.Equal(t, "dingding", payload["channels"])
			}
		})
	}
}

// A request rejected because the balance could not cover it reminds the user, and
// only then: a rejection with a balance left over is not a zero-balance notice.
// The rejection itself keeps its own status and error code.
func TestRejectedRequestWithEmptyBalanceNotifiesTheUser(t *testing.T) {
	database := newNotifyTestDB(t)

	previousClient, previousWorker := httpClient, system_setting.WorkerUrl
	fetch := system_setting.GetFetchSetting()
	previousFetch := *fetch
	t.Cleanup(func() {
		httpClient, system_setting.WorkerUrl = previousClient, previousWorker
		*fetch = previousFetch
	})

	var mutex sync.Mutex
	received := make([]WebhookPayload, 0, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload WebhookPayload
		require.NoError(t, common.DecodeJson(r.Body, &payload))
		mutex.Lock()
		received = append(received, payload)
		mutex.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	httpClient, system_setting.WorkerUrl = server.Client(), ""
	fetch.EnableSSRFProtection = false

	countReceived := func() int {
		mutex.Lock()
		defer mutex.Unlock()
		return len(received)
	}

	setting := dto.UserSetting{
		NotifyType:        dto.NotifyTypeWebhook,
		WebhookUrl:        server.URL + "/empty-wallet",
		BillingPreference: "wallet_only",
	}
	user := &model.User{Username: "empty-wallet", Email: "owner@example.com", Quota: 0, Status: common.UserStatusEnabled, AffCode: "empty-wallet"}
	require.NoError(t, database.Create(user).Error)

	relayInfo := &relaycommon.RelayInfo{
		UserId:          user.Id,
		UserEmail:       user.Email,
		UserSetting:     setting,
		OriginModelName: "test-model",
	}

	apiErr := PreConsumeBilling(notifyTestContext(t), 1000, relayInfo)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)

	require.Eventually(t, func() bool { return countReceived() == 1 }, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, dto.NotifyTypeQuotaEmpty, received[0].Type)
	assert.Equal(t, i18n.Translate("", i18n.MsgNotifyQuotaEmptyTitle), received[0].Title)
	assert.Contains(t, received[0].Content, "test-model")
	assert.Contains(t, received[0].Content, PaymentReturnURL("/wallet"))

	// A balance that simply cannot cover this request is not a zero balance.
	require.NoError(t, database.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", 5000).Error)
	apiErr = PreConsumeBilling(notifyTestContext(t), 900000, relayInfo)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	require.Never(t, func() bool { return countReceived() > 1 }, 300*time.Millisecond, 50*time.Millisecond)
}

// The message center is told exactly which channels to send through: the site's own
// list, narrowed by the user's choice, minus any channel the site cannot fill in (EIP
// needs a message source). A channel the centre cannot fill in fails the whole message.
func TestMessageChannelsFollowTheSiteAndTheUsersChoice(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings system_setting.MessageCenterSettings
		chosen   string
		want     []string
	}{
		{
			name: "a site that named no channel sends through wechat",
			want: []string{msgcenter.ChannelWeChat},
		},
		{
			name:     "the site's channels are sent in the order they are declared",
			settings: system_setting.MessageCenterSettings{Channels: "eip,wechat", ModuleID: "ANNOUNCEMENT"},
			want:     []string{msgcenter.ChannelWeChat, msgcenter.ChannelEIP},
		},
		{
			name:     "a channel the vocabulary does not know is dropped",
			settings: system_setting.MessageCenterSettings{Channels: "sms,wechat"},
			want:     []string{msgcenter.ChannelWeChat},
		},
		{
			name:     "the user narrows the site's channels down",
			settings: system_setting.MessageCenterSettings{Channels: "wechat,dingding"},
			chosen:   "dingding",
			want:     []string{msgcenter.ChannelDingTalk},
		},
		{
			name:     "a user choice the site does not send through leaves nothing",
			settings: system_setting.MessageCenterSettings{Channels: "wechat"},
			chosen:   "dingding",
			want:     []string{},
		},
		{
			name:     "eip without a message source is left out",
			settings: system_setting.MessageCenterSettings{Channels: "wechat,eip"},
			want:     []string{msgcenter.ChannelWeChat},
		},
		{
			name:     "eip with a message source is kept",
			settings: system_setting.MessageCenterSettings{Channels: "wechat,eip", ModuleID: "JOIN_APP"},
			want:     []string{msgcenter.ChannelWeChat, msgcenter.ChannelEIP},
		},
		{
			name:     "a message source outside the vocabulary is refused",
			settings: system_setting.MessageCenterSettings{Channels: "eip", ModuleID: "SOMETHING_ELSE"},
			want:     []string{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channels, _ := messageChannels(&tc.settings, tc.chosen, 1)
			assert.Equal(t, tc.want, channels)
		})
	}
}

// Every channel the message center offers carries a required field of its own, and a
// message missing one is rejected as a whole, so the sender fills in the ones its
// channel list needs.
func TestCampusNotifyFillsInEachChannelsRequiredField(t *testing.T) {
	database := newNotifyTestDB(t)

	settings := system_setting.GetMessageCenterSettings()
	previousSettings := *settings
	t.Cleanup(func() { *settings = previousSettings })

	var mutex sync.Mutex
	var sent []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		var payload map[string]any
		require.NoError(t, common.UnmarshalJsonStr(r.PostFormValue("msgData"), &payload))
		mutex.Lock()
		sent = append(sent, payload)
		mutex.Unlock()
		_, _ = w.Write([]byte(`{"code":200,"message":"成功","data":"msg-3"}`))
	}))
	t.Cleanup(server.Close)

	for index, tc := range []struct {
		name     string
		channels string
		moduleID string
		want     map[string]any
	}{
		{
			name:     "wechat carries a message type",
			channels: "wechat",
			want:     map[string]any{"channels": "wechat", "wechatMsgType": "text"},
		},
		{
			name:     "dingding carries a message type",
			channels: "dingding",
			want:     map[string]any{"channels": "dingding", "dingMsgType": "text"},
		},
		{
			name:     "eip carries its message source",
			channels: "eip",
			moduleID: "ANNOUNCEMENT",
			want:     map[string]any{"channels": "eip", "moduleID": "ANNOUNCEMENT"},
		},
		{
			name:     "an eip channel the site cannot fill in is left out",
			channels: "wechat,eip",
			want:     map[string]any{"channels": "wechat", "wechatMsgType": "text"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutex.Lock()
			sent = nil
			mutex.Unlock()

			*settings = system_setting.MessageCenterSettings{
				Enabled: true, BaseURL: server.URL, AppID: "app-1", Token: "token-1",
				Channels: tc.channels, ModuleID: tc.moduleID,
			}
			user := &model.User{
				Username: fmt.Sprintf("channels-%d", index), CasId: "20200010",
				Status: common.UserStatusEnabled, AffCode: fmt.Sprintf("channels-%d", index),
			}
			require.NoError(t, database.Create(user).Error)

			require.NoError(t, NotifyUser(user.Id, "", dto.UserSetting{NotifyType: dto.NotifyTypeCampus},
				dto.NewNotify(dto.NotifyTypeQuotaEmpty, "标题", "正文", nil)))

			require.Len(t, sent, 1)
			for field, want := range tc.want {
				assert.Equal(t, want, sent[0][field], field)
			}
		})
	}
}

// A message the centre draws as a card takes its button label and template code from the
// site. A template that is not a JSON object of channel to code is left out rather than
// failing every message.
func TestCampusNotifyCarriesTheCardFields(t *testing.T) {
	database := newNotifyTestDB(t)

	settings := system_setting.GetMessageCenterSettings()
	previousSettings := *settings
	t.Cleanup(func() { *settings = previousSettings })

	var mutex sync.Mutex
	var sent []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		var payload map[string]any
		require.NoError(t, common.UnmarshalJsonStr(r.PostFormValue("msgData"), &payload))
		mutex.Lock()
		sent = append(sent, payload)
		mutex.Unlock()
		_, _ = w.Write([]byte(`{"code":200,"message":"成功","data":"msg-4"}`))
	}))
	t.Cleanup(server.Close)

	for index, tc := range []struct {
		name         string
		template     string
		wantTemplate string
	}{
		{
			name:         "the configured template code reaches the centre",
			template:     `{"eip":"code1"}`,
			wantTemplate: `{"eip":"code1"}`,
		},
		{
			name:     "a template that is not JSON is left out",
			template: "not json",
		},
		{
			name:     "a JSON array is not a template map",
			template: `["code1"]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutex.Lock()
			sent = nil
			mutex.Unlock()

			*settings = system_setting.MessageCenterSettings{
				Enabled: true, BaseURL: server.URL, AppID: "app-1", Token: "token-1",
				Channels: "eip", ModuleID: "ANNOUNCEMENT",
				ButtonText: "查看详情", Template: tc.template,
			}
			user := &model.User{
				Username: fmt.Sprintf("card-%d", index), CasId: "20200011",
				Status: common.UserStatusEnabled, AffCode: fmt.Sprintf("card-%d", index),
			}
			require.NoError(t, database.Create(user).Error)

			require.NoError(t, NotifyUser(user.Id, "", dto.UserSetting{NotifyType: dto.NotifyTypeCampus},
				dto.NewNotify(dto.NotifyTypeQuotaEmpty, "标题", "正文", nil)))

			require.Len(t, sent, 1)
			assert.Equal(t, "查看详情", sent[0]["btntext"])
			assert.Equal(t, tc.wantTemplate, sent[0]["template"])
		})
	}
}

// A user who chose the message center has to hear a rejected request there: the
// low-balance switches ask for at most one notice when a balance crosses a
// threshold, so they do not govern a notice about a request that already failed.
func TestZeroBalanceNoticeReachesAUserWhoChoseTheMessageCenter(t *testing.T) {
	database := newNotifyTestDB(t)

	settings := system_setting.GetMessageCenterSettings()
	previousSettings := *settings
	previousRemind := common.QuotaRemindEnabled
	previousSystemName := common.SystemName
	t.Cleanup(func() {
		*settings = previousSettings
		common.QuotaRemindEnabled = previousRemind
		common.SystemName = previousSystemName
	})
	*settings = system_setting.MessageCenterSettings{Enabled: true, AppID: "app-1", Token: "token-1"}
	// The site and the user both turned balance reminders off.
	common.QuotaRemindEnabled = false
	common.SystemName = "示例站点"
	off := false

	var mutex sync.Mutex
	var sent []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		var payload map[string]any
		require.NoError(t, common.UnmarshalJsonStr(r.PostFormValue("msgData"), &payload))
		mutex.Lock()
		sent = append(sent, payload)
		mutex.Unlock()
		_, _ = w.Write([]byte(`{"code":200,"message":"成功","data":"msg-2"}`))
	}))
	t.Cleanup(server.Close)
	settings.BaseURL = server.URL

	user := &model.User{Username: "broke-oa", CasId: "20200009", Quota: 0, Status: common.UserStatusEnabled, AffCode: "broke-oa"}
	require.NoError(t, database.Create(user).Error)

	relayInfo := &relaycommon.RelayInfo{
		UserId: user.Id,
		UserSetting: dto.UserSetting{
			NotifyType:          dto.NotifyTypeCampus,
			QuotaWarningEnabled: &off,
			BillingPreference:   "wallet_only",
		},
		OriginModelName: "test-model",
	}

	apiErr := PreConsumeBilling(notifyTestContext(t), 1000, relayInfo)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())

	require.Eventually(t, func() bool {
		mutex.Lock()
		defer mutex.Unlock()
		return len(sent) == 1
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, "20200009", sent[0]["intReceiver"])
	assert.Contains(t, sent[0]["content"], "test-model")
	// A campus message names the site it came from, taken from the console's system name.
	content, ok := sent[0]["content"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(content, "【示例站点】"),
		"the notice should name its site: %v", sent[0]["content"])
	title, ok := sent[0]["title"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(title, "【示例站点】"),
		"the title should name its site too: %v", sent[0]["title"])
}

// The message center becomes the default method only once the site has configured
// one; a user who chose a method always keeps it.
func TestDeliveryMethodFallsBackToTheMessageCenterOnceConfigured(t *testing.T) {
	settings := system_setting.GetMessageCenterSettings()
	previousSettings := *settings
	t.Cleanup(func() { *settings = previousSettings })

	for _, tc := range []struct {
		name       string
		configured bool
		setting    dto.UserSetting
		want       string
	}{
		{name: "never chose, no center", setting: dto.UserSetting{}, want: dto.NotifyTypeEmail},
		{name: "never chose, center configured", setting: dto.UserSetting{}, configured: true, want: dto.NotifyTypeCampus},
		{name: "chose email", setting: dto.UserSetting{NotifyType: dto.NotifyTypeEmail}, configured: true, want: dto.NotifyTypeEmail},
		{name: "chose the message center", setting: dto.UserSetting{NotifyType: dto.NotifyTypeCampus}, want: dto.NotifyTypeCampus},
		{name: "chose bark", setting: dto.UserSetting{NotifyType: dto.NotifyTypeBark}, want: dto.NotifyTypeBark},
	} {
		t.Run(tc.name, func(t *testing.T) {
			*settings = system_setting.MessageCenterSettings{}
			if tc.configured {
				*settings = system_setting.MessageCenterSettings{Enabled: true, BaseURL: "http://message.example.edu.cn:9200", AppID: "app-1", Token: "token-1"}
			}
			assert.Equal(t, tc.want, deliveryMethod(tc.setting))
		})
	}
}
