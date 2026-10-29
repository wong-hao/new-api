package system_setting

import (
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// CASSettings holds the CAS 3.0 (Apereo) single sign-on configuration.
//
// The only deployment-specific value an administrator must supply is ServerURL,
// the CAS context root such as "https://cas.example.edu.cn/cas". The protocol
// endpoints are derived from it because every CAS server exposes them at fixed
// paths below that root.
type CASSettings struct {
	Enabled     bool   `json:"enabled"`
	DisplayName string `json:"display_name"`
	ServerURL   string `json:"server_url"`
	// NameAttribute and EmailAttribute name the CAS attributes carrying the
	// display name and email address. Their names are chosen by the school's
	// CAS administrators and differ between deployments.
	NameAttribute  string `json:"name_attribute"`
	EmailAttribute string `json:"email_attribute"`
	// GroupEnabled turns the department an account signs in with into its group,
	// and GroupAttribute names the attribute carrying that department. The switch
	// is what asks for the grouping; the attribute keeps a default so turning it
	// on without naming one still uses what the campus directory is known to send.
	GroupEnabled   bool   `json:"group_enabled"`
	GroupAttribute string `json:"group_attribute"`
}

// 默认配置
var defaultCASSettings = CASSettings{}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("cas", &defaultCASSettings)
}

func GetCASSettings() *CASSettings {
	return &defaultCASSettings
}

// GetEffectiveDisplayName returns the admin-configured display name, or the
// literal "CAS" when none has been set. The default lives here so the OAuth
// provider name and the public status payload cannot drift apart.
func (s *CASSettings) GetEffectiveDisplayName() string {
	if trimmed := strings.TrimSpace(s.DisplayName); trimmed != "" {
		return trimmed
	}
	return "CAS"
}

// EffectiveNameAttribute returns the configured display-name attribute, or
// "name". The school's CAS decides the attribute names, so both attributes are
// administrator-configurable with a conventional default.
func (s *CASSettings) EffectiveNameAttribute() string {
	if trimmed := strings.TrimSpace(s.NameAttribute); trimmed != "" {
		return trimmed
	}
	return "name"
}

// EffectiveEmailAttribute returns the configured email attribute, or "email".
func (s *CASSettings) EffectiveEmailAttribute() string {
	if trimmed := strings.TrimSpace(s.EmailAttribute); trimmed != "" {
		return trimmed
	}
	return "email"
}

// EffectiveGroupAttribute returns the configured department attribute, or
// "deptName". Unlike name and email this one has no broadly conventional name,
// but the switch beside it is what asks for the grouping, so a deployment that
// turns that on and names nothing still gets the attribute the campus directory
// is known to send — and a wrong name is reported rather than silently doing
// nothing.
func (s *CASSettings) EffectiveGroupAttribute() string {
	if trimmed := strings.TrimSpace(s.GroupAttribute); trimmed != "" {
		return trimmed
	}
	return "deptName"
}

// BaseURL returns the trimmed CAS context root, or "" when it is unusable.
// A value that is not an absolute http(s) URL is rejected here rather than at
// request time, so a misconfigured server is treated as "not configured".
func (s *CASSettings) BaseURL() string {
	trimmed := strings.TrimRight(strings.TrimSpace(s.ServerURL), "/")
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	return trimmed
}

// IsConfigured reports whether the provider can serve a login round trip.
func (s *CASSettings) IsConfigured() bool {
	return s.BaseURL() != ""
}

// LoginURL is the endpoint the browser is redirected to for single sign-on.
func (s *CASSettings) LoginURL() string {
	if base := s.BaseURL(); base != "" {
		return base + "/login"
	}
	return ""
}

// LogoutURL is the single sign-out endpoint. Passing a service URL makes CAS
// return the browser to this application afterwards.
func (s *CASSettings) LogoutURL() string {
	if base := s.BaseURL(); base != "" {
		return base + "/logout"
	}
	return ""
}

// ServiceValidateURL is the CAS 3.0 ticket validation endpoint. The JSON format
// avoids parsing CAS's XML response, and the /p3/ prefix selects the protocol
// version whose response carries the authentication attributes.
func (s *CASSettings) ServiceValidateURL() string {
	if base := s.BaseURL(); base != "" {
		return base + "/p3/serviceValidate"
	}
	return ""
}

// ServiceURL is the callback this application registers with CAS as its
// service. It is a constant, with no per-attempt parameter: a CAS server
// matches the service against its registration list, and a deployment whose
// registration only accepts an exact string rejects anything appended to it.
// The authorization flow state therefore travels in the browser rather than in
// this URL (see the frontend's stored callback state).
// CAS echoes the service identifier back on validation, so the string built
// when sending the browser to CAS and the one sent to serviceValidate MUST be
// byte-identical; both callers use this method.
func (s *CASSettings) ServiceURL() string {
	if !s.IsConfigured() {
		return ""
	}
	return serviceOrigin() + "/oauth/cas"
}

// SignInURL is the complete CAS sign-in URL, service included. Handing it out as
// one value keeps the service identifier the browser is sent with identical to
// the one this application validates against, instead of leaving every caller
// to rebuild it from the public origin.
func (s *CASSettings) SignInURL() string {
	loginURL := s.LoginURL()
	service := s.ServiceURL()
	if loginURL == "" || service == "" {
		return ""
	}
	values := url.Values{}
	values.Set("service", service)
	return loginURL + "?" + values.Encode()
}

// SingleLogoutURL is the endpoint that ends the CAS single sign-on session and
// returns the browser to this application's sign-in page. Following it signs
// the user out of every application sharing that CAS session, so callers must
// only send the browser here when the session being ended was established
// through CAS.
func (s *CASSettings) SingleLogoutURL() string {
	logoutURL := s.LogoutURL()
	if logoutURL == "" {
		return ""
	}
	values := url.Values{}
	values.Set("service", serviceOrigin()+"/sign-in")
	return logoutURL + "?" + values.Encode()
}

// serviceOrigin is this application's public origin. The frontend builds the
// same callback URL from window.location.origin, and CAS rejects a validation
// whose service does not match the one used at login, so both must agree.
func serviceOrigin() string {
	return strings.TrimRight(ServerAddress, "/")
}
