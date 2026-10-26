package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	casTestOrigin  = "https://api.example.edu.cn"
	casTestService = casTestOrigin + "/oauth/cas"
)

// withCASEnabled points the CAS settings at a test server for one test and
// restores the global settings and public origin afterwards, so tests stay
// independent of each other and of the order they run in.
func withCASEnabled(t *testing.T, serverURL string) {
	t.Helper()
	settings := system_setting.GetCASSettings()
	originalSettings := *settings
	originalOrigin := system_setting.ServerAddress
	t.Cleanup(func() {
		*settings = originalSettings
		system_setting.ServerAddress = originalOrigin
	})
	*settings = system_setting.CASSettings{Enabled: true, ServerURL: serverURL}
	system_setting.ServerAddress = casTestOrigin
}

// newCASServer serves one canned serviceValidate response and records the query
// CAS received, so a test can assert what was actually sent.
func newCASServer(t *testing.T, body string, status int) (*httptest.Server, *url.Values) {
	t.Helper()
	received := &url.Values{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*received = r.URL.Query()
		if r.URL.Path != "/cas/p3/serviceValidate" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, received
}

// casCallbackContext is the request the generic OAuth controller hands to the
// provider: the service ticket arrives as the authorization code argument, and
// the flow state sits beside it in the query for the controller to match. The
// provider must not read either from the request when it builds the service
// identifier it validates with.
func casCallbackContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/oauth/cas?code=ST-1-abc&state=flow-state", nil)
	return c
}

func casSuccessBody(attributes string) string {
	return `{"serviceResponse":{"authenticationSuccess":{"user":"TEST001","attributes":{` + attributes + `}}}}`
}

// requireOAuthMessageKey asserts the translatable key the web console will
// render, which is the stable contract rather than any one language's text.
func requireOAuthMessageKey(t *testing.T, err error, expectedKey string) {
	t.Helper()
	var oauthErr *OAuthError
	require.ErrorAs(t, err, &oauthErr)
	require.NotNil(t, oauthErr.Message)
	assert.Equal(t, expectedKey, oauthErr.Message.Key)
}

func TestCASProviderSendsTicketServiceAndJSONFormat(t *testing.T) {
	server, received := newCASServer(t, casSuccessBody(`"name":"Zhang San"`), http.StatusOK)
	withCASEnabled(t, server.URL+"/cas")

	provider := &CASProvider{}
	token, err := provider.ExchangeToken(context.Background(), "ST-1-abc", casCallbackContext(t))
	require.NoError(t, err)
	require.NotNil(t, token)

	assert.Equal(t, "ST-1-abc", received.Get("ticket"))
	// The service identifier must be the exact constant string the browser was
	// sent to CAS with, and the one the CAS registration list holds. It is
	// rebuilt from server configuration alone, so it carries no per-attempt
	// parameter and nothing the request supplied can influence it.
	assert.Equal(t, casTestService, received.Get("service"))
	assert.Equal(t, "JSON", received.Get("format"))
	assert.Equal(t, "TEST001", token.AccessToken)
}

func TestCASProviderIgnoresRequestSuppliedService(t *testing.T) {
	server, received := newCASServer(t, casSuccessBody(`"name":"Zhang San"`), http.StatusOK)
	withCASEnabled(t, server.URL+"/cas")

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet,
		"/api/oauth/cas?code=ST-1-abc&state=injected&service="+
			url.QueryEscape("https://attacker.example.com/oauth/cas"), nil)

	_, err := (&CASProvider{}).ExchangeToken(context.Background(), "ST-1-abc", c)
	require.NoError(t, err)
	assert.Equal(t, casTestService, received.Get("service"))
}

func TestCASProviderMapsAttributes(t *testing.T) {
	// CAS returns multi-valued attributes as arrays and single-valued ones as
	// strings; the login flow has to read both shapes.
	server, _ := newCASServer(t, casSuccessBody(`"name":"Zhang San","email":["zhangsan@example.edu.cn","other@example.edu.cn"]`), http.StatusOK)
	withCASEnabled(t, server.URL+"/cas")

	provider := &CASProvider{}
	token, err := provider.ExchangeToken(context.Background(), "ST-1-abc", casCallbackContext(t))
	require.NoError(t, err)

	user, err := provider.GetUserInfo(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, "TEST001", user.ProviderUserID)
	assert.Equal(t, "TEST001", user.Username)
	assert.Equal(t, "Zhang San", user.DisplayName)
	assert.Equal(t, "zhangsan@example.edu.cn", user.Email)
}

func TestCASProviderFallsBackThroughAttributeNames(t *testing.T) {
	// A deployment that publishes cn/mail instead of name/email still resolves.
	server, _ := newCASServer(t, casSuccessBody(`"cn":"Li Si","mail":"lisi@example.edu.cn"`), http.StatusOK)
	withCASEnabled(t, server.URL+"/cas")

	provider := &CASProvider{}
	token, err := provider.ExchangeToken(context.Background(), "ST-1-abc", casCallbackContext(t))
	require.NoError(t, err)

	user, err := provider.GetUserInfo(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, "Li Si", user.DisplayName)
	assert.Equal(t, "lisi@example.edu.cn", user.Email)
}

func TestCASProviderHonoursConfiguredAttributeNames(t *testing.T) {
	server, _ := newCASServer(t, casSuccessBody(`"displayName":"Wang Wu","schoolMail":"wangwu@example.edu.cn"`), http.StatusOK)
	withCASEnabled(t, server.URL+"/cas")
	settings := system_setting.GetCASSettings()
	settings.NameAttribute = "displayName"
	settings.EmailAttribute = "schoolMail"

	provider := &CASProvider{}
	token, err := provider.ExchangeToken(context.Background(), "ST-1-abc", casCallbackContext(t))
	require.NoError(t, err)

	user, err := provider.GetUserInfo(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, "Wang Wu", user.DisplayName)
	assert.Equal(t, "wangwu@example.edu.cn", user.Email)
}

func TestCASProviderLogsInWithoutAttributesOrEmail(t *testing.T) {
	// Many campus accounts carry no email; refusing them would lock out real
	// users, so the flow completes with an empty email instead.
	server, _ := newCASServer(t, `{"serviceResponse":{"authenticationSuccess":{"user":"TEST001"}}}`, http.StatusOK)
	withCASEnabled(t, server.URL+"/cas")

	provider := &CASProvider{}
	token, err := provider.ExchangeToken(context.Background(), "ST-1-abc", casCallbackContext(t))
	require.NoError(t, err)

	user, err := provider.GetUserInfo(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, "TEST001", user.ProviderUserID)
	assert.Empty(t, user.Email)
	assert.Empty(t, user.DisplayName)
}

func TestCASProviderRejectsAuthenticationFailureWithoutLeakingCASDetail(t *testing.T) {
	body := `{"serviceResponse":{"authenticationFailure":{"code":"INVALID_TICKET","description":"Ticket ST-1-abc not recognized"}}}`
	server, _ := newCASServer(t, body, http.StatusOK)
	withCASEnabled(t, server.URL+"/cas")

	provider := &CASProvider{}
	_, err := provider.ExchangeToken(context.Background(), "ST-1-abc", casCallbackContext(t))

	requireOAuthMessageKey(t, err, msgCASValidateFailed)
	// The CAS failure code and description name the ticket and the server's
	// internal state, so neither may reach the client.
	assert.NotContains(t, err.Error(), "INVALID_TICKET")
	assert.NotContains(t, err.Error(), "ST-1-abc")
}

func TestCASProviderRejectsEmptyTicketBeforeCallingCAS(t *testing.T) {
	server, received := newCASServer(t, casSuccessBody(`"name":"Zhang San"`), http.StatusOK)
	withCASEnabled(t, server.URL+"/cas")

	provider := &CASProvider{}
	_, err := provider.ExchangeToken(context.Background(), "  ", casCallbackContext(t))

	requireOAuthMessageKey(t, err, msgInvalidCode)
	assert.Empty(t, received.Get("ticket"))
}

func TestCASProviderRejectsUnusableResponses(t *testing.T) {
	testCases := []struct {
		name string
		body string
		code int
	}{
		{name: "http error status", body: `{}`, code: http.StatusInternalServerError},
		{name: "malformed json", body: `{"serviceResponse":`, code: http.StatusOK},
		{name: "success without a principal", body: `{"serviceResponse":{"authenticationSuccess":{"attributes":{"name":"Zhang San"}}}}`, code: http.StatusOK},
		{name: "empty envelope", body: `{}`, code: http.StatusOK},
		{name: "oversized response", body: `{"padding":"` + strings.Repeat("x", maxCASResponseBytes) + `"}`, code: http.StatusOK},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server, _ := newCASServer(t, testCase.body, testCase.code)
			withCASEnabled(t, server.URL+"/cas")

			provider := &CASProvider{}
			_, err := provider.ExchangeToken(context.Background(), "ST-1-abc", casCallbackContext(t))
			require.Error(t, err)
		})
	}
}

func TestCASProviderRejectsUserInfoWithoutValidation(t *testing.T) {
	withCASEnabled(t, "https://cas.example.edu.cn/cas")

	provider := &CASProvider{}
	_, err := provider.GetUserInfo(context.Background(), &OAuthToken{})

	requireOAuthMessageKey(t, err, msgGetUserFailed)
}

func TestCASProviderIsDisabledUntilConfigured(t *testing.T) {
	settings := system_setting.GetCASSettings()
	original := *settings
	t.Cleanup(func() { *settings = original })

	provider := &CASProvider{}
	settings.Enabled, settings.ServerURL = true, ""
	assert.False(t, provider.IsEnabled())

	settings.ServerURL = "not-a-url"
	assert.False(t, provider.IsEnabled())

	settings.ServerURL = "https://cas.example.edu.cn/cas/"
	assert.True(t, provider.IsEnabled())

	settings.Enabled = false
	assert.False(t, provider.IsEnabled())
}

func TestCASSettingsDeriveEndpointsFromServerURL(t *testing.T) {
	settings := system_setting.GetCASSettings()
	original := *settings
	originalOrigin := system_setting.ServerAddress
	t.Cleanup(func() {
		*settings = original
		system_setting.ServerAddress = originalOrigin
	})
	*settings = system_setting.CASSettings{Enabled: true, ServerURL: "https://cas.example.edu.cn/cas/"}
	system_setting.ServerAddress = casTestOrigin

	// A trailing slash would otherwise produce "//login".
	assert.Equal(t, "https://cas.example.edu.cn/cas/login", settings.LoginURL())
	assert.Equal(t, "https://cas.example.edu.cn/cas/logout", settings.LogoutURL())
	assert.Equal(t, "https://cas.example.edu.cn/cas/p3/serviceValidate", settings.ServiceValidateURL())
	assert.Equal(t, casTestService, settings.ServiceURL())
	// The sign-in URL hands out the same service the validation will use, encoded
	// as CAS expects to receive it.
	assert.Equal(t, "https://cas.example.edu.cn/cas/login?service="+url.QueryEscape(casTestService), settings.SignInURL())
	assert.Equal(t, "https://cas.example.edu.cn/cas/logout?service="+url.QueryEscape(casTestOrigin+"/sign-in"), settings.SingleLogoutURL())
}

func TestCASProviderGetNameFallsBackToCAS(t *testing.T) {
	settings := system_setting.GetCASSettings()
	original := *settings
	t.Cleanup(func() { *settings = original })

	provider := &CASProvider{}
	settings.DisplayName = "  "
	assert.Equal(t, "CAS", provider.GetName())

	settings.DisplayName = "  Campus SSO  "
	assert.Equal(t, "Campus SSO", provider.GetName())
}

func TestCASProviderUsesItsOwnBindingColumn(t *testing.T) {
	provider := &CASProvider{}
	assert.Equal(t, "cas_id", provider.ProviderUserIDColumn())
	assert.Equal(t, "cas_", provider.GetProviderPrefix())
}
