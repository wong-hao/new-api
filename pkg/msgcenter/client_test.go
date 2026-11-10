package msgcenter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const appIDHeader = "appId"

// newTestClient points a client at handler and records what the center would see.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewClient(server.URL, "app-1", "token-1")
}

func TestSendMsgSendsFormEncodedMsgData(t *testing.T) {
	var gotMethod, gotPath, gotAppID, gotToken, gotContentType string
	var gotMsgData string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAppID, gotToken = r.Method, r.URL.Path, r.Header.Get(appIDHeader), r.Header.Get("token")
		gotContentType = r.Header.Get("Content-Type")
		require.NoError(t, r.ParseForm())
		gotMsgData = r.PostFormValue("msgData")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"成功","data":"79709c8-msg"}`))
	})

	msgID, err := client.SendMsg(context.Background(), &Message{
		Title:         "渠道告警",
		Content:       "渠道已被自动禁用",
		Channels:      ChannelWeChat,
		IntReceiver:   "20200001",
		WeChatMsgType: WeChatMsgTypeTextCard,
		PCURL:         "https://example.com/logs",
	})
	require.NoError(t, err)
	assert.Equal(t, "79709c8-msg", msgID)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, sendMsgPath, gotPath)
	assert.Equal(t, "app-1", gotAppID)
	assert.Equal(t, "token-1", gotToken)
	assert.Equal(t, "application/x-www-form-urlencoded", gotContentType)

	assert.Equal(t, `{"title":"渠道告警","content":"渠道已被自动禁用","channels":"wechat","intReceiver":"20200001",`+
		`"smsReceiver":"","emailReceiver":"","wechatMsgType":"textcard","dingMsgType":"","moduleID":"","btntext":"",`+
		`"wechatAgentId":"","dingAgentId":"","cron":"0","sendTime":"","pcUrl":"https://example.com/logs",`+
		`"mobileUrl":"","fileId":"","template":""}`, gotMsgData)
}

func TestSendMsgKeepsScheduledCron(t *testing.T) {
	var gotMsgData string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		gotMsgData = r.PostFormValue("msgData")
		_, _ = w.Write([]byte(`{"code":200,"message":"成功","data":"m-1"}`))
	})

	_, err := client.SendMsg(context.Background(), &Message{
		Title:    "定时通知",
		Channels: ChannelSMS,
		Cron:     "1",
		SendTime: "2026-11-11 09:00:00",
	})
	require.NoError(t, err)
	assert.Contains(t, gotMsgData, `"cron":"1"`)
	assert.Contains(t, gotMsgData, `"sendTime":"2026-11-11 09:00:00"`)
}

func TestGetChannelsParsesAuthorizedChannels(t *testing.T) {
	var gotMethod, gotPath, gotAppID, gotToken string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAppID, gotToken = r.Method, r.URL.Path, r.Header.Get(appIDHeader), r.Header.Get("token")
		_, _ = w.Write([]byte(`{"code":200,"message":"成功","data":{"channels":"sms, wechat,eip,",` +
			`"validDate":"2021-10-21 08:00:00 - 2023-12-31 08:00:00"}}`))
	})

	auth, err := client.GetChannels(context.Background())
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, gotMethod)
	assert.Equal(t, getChannelsPath, gotPath)
	assert.Equal(t, "app-1", gotAppID)
	assert.Equal(t, "token-1", gotToken)
	assert.Equal(t, []string{ChannelSMS, ChannelWeChat, ChannelEIP}, auth.Channels)
	assert.Equal(t, "2021-10-21 08:00:00 - 2023-12-31 08:00:00", auth.ValidDate)
}

func TestCallReturnsAPIError(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr *APIError
	}{
		{
			name:    "message field",
			body:    `{"code":6010603,"message":"没有开通渠道权限","data":null}`,
			wantErr: &APIError{Code: CodeChannelNotAllowed, Message: "没有开通渠道权限"},
		},
		{
			name:    "msg field, as the spec text names it",
			body:    `{"code":6010624,"msg":"今日发送消息次数已达上限，无法发送"}`,
			wantErr: &APIError{Code: CodeDailyQuotaReached, Message: "今日发送消息次数已达上限，无法发送"},
		},
		{
			name:    "bad request row of the code table",
			body:    `{"code":400,"message":"Bad Request"}`,
			wantErr: &APIError{Code: 400, Message: "Bad Request"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(test.body))
			})

			msgID, err := client.SendMsg(context.Background(), &Message{Title: "t", Channels: ChannelSMS})
			assert.Empty(t, msgID)

			var apiErr *APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, test.wantErr, apiErr)
			assert.Contains(t, err.Error(), "msgcenter: code")
		})
	}
}

func TestSendMsgRejectsBodyWithoutResultCode(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "Internal Server Error")
	})

	_, err := client.SendMsg(context.Background(), &Message{Title: "t", Channels: ChannelSMS})
	require.Error(t, err)

	var apiErr *APIError
	assert.False(t, errors.As(err, &apiErr), "an unreadable body is a transport failure, not a center return code")
	assert.Contains(t, err.Error(), "HTTP 500")
}
