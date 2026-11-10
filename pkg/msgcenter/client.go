// Package msgcenter is the client for the campus message center (新消息中心,
// served under /htmq/) that delivers notices over SMS, email, WeChat, DingTalk,
// EIP and the campus app. The wire contract is documented in api.md next to this
// file; the package only builds requests, parses responses and maps return codes,
// and takes its base URL and credentials from the caller.
package msgcenter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// DefaultTimeout bounds one call to the message center. The center answers in
// tens of milliseconds, but it fans out to SMS/email/WeChat/DingTalk upstreams.
const DefaultTimeout = 30 * time.Second

// maxResponseBytes caps how much of a response body is read. Both endpoints
// answer with a small envelope; a larger body means something other than the
// center replied.
const maxResponseBytes = 1 << 20

const (
	getChannelsPath = "/htmq/getChannels"
	sendMsgPath     = "/htmq/sendMsg"
)

// Client sends messages as one authorized application. Sign in with NewClient;
// HTTPClient may be replaced before the first call to change transport,
// timeouts or instrumentation.
type Client struct {
	baseURL string
	appID   string
	token   string

	HTTPClient *http.Client
}

// NewClient returns a client for the message center at baseURL, which is the
// deployment root without a trailing slash (its port differs between the spec
// text and its examples, so it is never inferred here).
func NewClient(baseURL, appID, token string) *Client {
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		appID:      appID,
		token:      token,
		HTTPClient: &http.Client{Timeout: DefaultTimeout},
	}
}

// GetChannels reports the channels this application may currently send through
// and the validity window the center reports for it.
func (c *Client) GetChannels(ctx context.Context) (*Authorization, error) {
	envelope, err := c.call(ctx, http.MethodGet, getChannelsPath, nil)
	if err != nil {
		return nil, err
	}

	var data struct {
		Channels  string `json:"channels"`
		ValidDate string `json:"validDate"`
	}
	if err := common.Unmarshal(envelope.Data, &data); err != nil {
		return nil, fmt.Errorf("msgcenter: decode %s data: %w", getChannelsPath, err)
	}

	return &Authorization{
		Channels:  splitChannels(data.Channels),
		ValidDate: data.ValidDate,
	}, nil
}

// SendMsg sends msg and returns the message id the center assigned to it.
func (c *Client) SendMsg(ctx context.Context, msg *Message) (string, error) {
	if msg == nil {
		return "", errors.New("msgcenter: message is nil")
	}

	payload := *msg
	if payload.Cron == "" {
		payload.Cron = "0"
	}
	encoded, err := common.Marshal(&payload)
	if err != nil {
		return "", fmt.Errorf("msgcenter: encode msgData: %w", err)
	}

	envelope, err := c.call(ctx, http.MethodPost, sendMsgPath, url.Values{"msgData": {string(encoded)}})
	if err != nil {
		return "", err
	}

	var msgID string
	if err := common.Unmarshal(envelope.Data, &msgID); err != nil {
		return "", fmt.Errorf("msgcenter: decode %s data: %w", sendMsgPath, err)
	}
	return msgID, nil
}

// envelope is the response shape shared by both endpoints. The spec text names
// the message field "msg" while the center's own examples return "message", so
// both are read, preferring the latter.
type envelope struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Msg     string            `json:"msg"`
	Data    common.RawMessage `json:"data"`
}

// call sends one request and returns its envelope, or an *APIError carrying the
// center's own code and message. form, when non-nil, is sent as the
// x-www-form-urlencoded body.
func (c *Client) call(ctx context.Context, method, path string, form url.Values) (*envelope, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("msgcenter: build %s request: %w", path, err)
	}
	req.Header.Set("appId", c.appID)
	req.Header.Set("token", c.token)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("msgcenter: call %s: %w", path, err)
	}
	defer resp.Body.Close()

	// A code in the 4xx/5xx rows of the return-code table arrives as the same JSON
	// envelope, so the body is decoded first and the HTTP status only reported
	// when there is no envelope to read.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("msgcenter: read %s response: %w", path, err)
	}

	var env envelope
	if err := common.Unmarshal(raw, &env); err != nil || env.Code == 0 {
		return nil, fmt.Errorf("msgcenter: %s returned HTTP %s without a result code", path, resp.Status)
	}
	if env.Code != SuccessCode {
		text := env.Message
		if text == "" {
			text = env.Msg
		}
		return nil, &APIError{Code: env.Code, Message: text}
	}
	return &env, nil
}
