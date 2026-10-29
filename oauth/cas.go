package oauth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// maxCASResponseBytes bounds the ticket validation response we are willing to
// buffer. A serviceValidate reply is a few kilobytes at most.
const maxCASResponseBytes = 1 << 20

// casLoginMethod is what controller.loginMethodFromContext records for a login
// that went through the generic /api/oauth/:provider route, which is how the
// sign-out flow recognises a session it must also end at the CAS server.
const casLoginMethod = "oauth:cas"

func init() {
	Register("cas", &CASProvider{})
}

// CASProvider implements OAuth for a CAS 3.0 (Apereo) single sign-on server.
//
// CAS is not an OAuth2 protocol: there is no authorization code exchanged for a
// token. The browser returns from the CAS login page with a service ticket, and
// that ticket is validated against the CAS server, which answers with the
// authenticated principal and its attributes in one response. ExchangeToken
// performs that validation and GetUserInfo returns what it produced, so the two
// interface halves stay a single round trip.
type CASProvider struct{}

// A CAS sign-in carries the campus directory's own record of the account, so it
// takes part in filling in missing profile fields (see BackfillsMissingProfile)
// and in naming the group the account belongs to (see AssertedGroup).
var (
	_ ProfileBackfillProvider = (*CASProvider)(nil)
	_ GroupAssignmentProvider = (*CASProvider)(nil)
)

// CASValidation is the outcome of one successful CAS ticket validation,
// carrying it from ExchangeToken to GetUserInfo.
type CASValidation struct {
	// Principal is the unified account (CAS <user>).
	Principal string
	// Attributes are the CAS authentication attributes, keyed by their
	// deployment-specific names.
	Attributes map[string]any
}

type casServiceResponse struct {
	ServiceResponse struct {
		AuthenticationSuccess *struct {
			User       string         `json:"user"`
			Attributes map[string]any `json:"attributes"`
		} `json:"authenticationSuccess"`
		AuthenticationFailure *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"authenticationFailure"`
	} `json:"serviceResponse"`
}

func (p *CASProvider) GetName() string {
	return system_setting.GetCASSettings().GetEffectiveDisplayName()
}

// SingleLogoutURLForSession returns the CAS single sign-out endpoint for a
// session that CAS established, and "" for any other session.
//
// Signing out of an application that shares a CAS session has to end that CAS
// session too, but only for the sessions CAS created: sending a password-based
// sign-out to CAS would end an unrelated single sign-on session that the user
// never linked to this application.
func SingleLogoutURLForSession(sessionID string) string {
	settings := system_setting.GetCASSettings()
	if sessionID == "" || !settings.Enabled || !settings.IsConfigured() {
		return ""
	}
	session, err := model.GetUserSessionBySID(sessionID)
	if err != nil || session.LoginMethod != casLoginMethod {
		return ""
	}
	return settings.SingleLogoutURL()
}

// IsEnabled reports whether CAS login is both switched on and configured.
// A cleared or malformed server URL disables the provider rather than producing
// redirects to nowhere.
func (p *CASProvider) IsEnabled() bool {
	settings := system_setting.GetCASSettings()
	return settings.Enabled && settings.IsConfigured()
}

func (p *CASProvider) ExchangeToken(ctx context.Context, ticket string, c *gin.Context) (*OAuthToken, error) {
	if strings.TrimSpace(ticket) == "" {
		return nil, NewOAuthError(common.NewMessage(msgInvalidCode))
	}
	settings := system_setting.GetCASSettings()
	if !settings.IsConfigured() {
		return nil, fmt.Errorf("cas: the CAS server URL is not configured")
	}
	// CAS rejects a validation whose service does not match the one used at
	// login, so the service identifier is rebuilt from server configuration —
	// never taken from the request. It is a constant, which is what a CAS
	// deployment whose registration list only holds exact strings can match.
	service := settings.ServiceURL()
	if service == "" {
		return nil, fmt.Errorf("cas: the server address is not configured, CAS cannot verify the service URL")
	}
	values := url.Values{}
	values.Set("ticket", ticket)
	values.Set("service", service)
	values.Set("format", "JSON")
	validateURL := settings.ServiceValidateURL() + "?" + values.Encode()

	logger.LogDebug(ctx, "[OAuth-CAS] ExchangeToken: service_validate=%s", settings.ServiceValidateURL())

	req, err := http.NewRequestWithContext(ctx, "GET", validateURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	client := http.Client{
		Timeout: 5 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		logger.LogError(ctx, common.LogText("[OAuth-CAS] ExchangeToken error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(common.NewMessage(msgConnectFailed, map[string]any{"provider": "CAS"}), err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-CAS] ExchangeToken response status: %d", res.StatusCode)

	if res.StatusCode != http.StatusOK {
		logger.LogError(ctx, common.LogText("[OAuth-CAS] ExchangeToken failed: status=%d", res.StatusCode))
		return nil, NewOAuthError(common.NewMessage(msgCASValidateFailed))
	}

	// Read with a bound before decoding so an oversized or endless response
	// cannot exhaust memory.
	body, err := io.ReadAll(io.LimitReader(res.Body, maxCASResponseBytes+1))
	if err != nil {
		logger.LogError(ctx, common.LogText("[OAuth-CAS] ExchangeToken read error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(common.NewMessage(msgCASValidateFailed), err.Error())
	}
	if len(body) > maxCASResponseBytes {
		logger.LogError(ctx, common.LogText("[OAuth-CAS] ExchangeToken failed: response exceeds %d bytes", maxCASResponseBytes))
		return nil, NewOAuthError(common.NewMessage(msgCASValidateFailed))
	}

	var response casServiceResponse
	if err := common.Unmarshal(body, &response); err != nil {
		// The CAS failure code and description stay server-side: they name the
		// ticket and the internal validation state.
		logger.LogError(ctx, common.LogText("[OAuth-CAS] ExchangeToken decode error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(common.NewMessage(msgCASValidateFailed), err.Error())
	}

	if failure := response.ServiceResponse.AuthenticationFailure; failure != nil {
		logger.LogError(ctx, common.LogText("[OAuth-CAS] Ticket rejected by CAS: code=%s description=%s", failure.Code, failure.Description))
		return nil, NewOAuthError(common.NewMessage(msgCASValidateFailed))
	}

	success := response.ServiceResponse.AuthenticationSuccess
	if success == nil || strings.TrimSpace(success.User) == "" {
		logger.LogError(ctx, common.LogText("[OAuth-CAS] ExchangeToken failed: no authenticated principal in the CAS response"))
		return nil, NewOAuthError(common.NewMessage(msgUserInfoEmpty, map[string]any{"provider": "CAS"}))
	}

	principal := strings.TrimSpace(success.User)
	logger.LogDebug(ctx, "[OAuth-CAS] ExchangeToken success: principal=%s", principal)

	return &OAuthToken{
		AccessToken: principal,
		TokenType:   "cas",
		CASValidation: &CASValidation{
			Principal:  principal,
			Attributes: success.Attributes,
		},
	}, nil
}

func (p *CASProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	validation := token.CASValidation
	if validation == nil || validation.Principal == "" {
		return nil, NewOAuthError(common.NewMessage(msgGetUserFailed))
	}
	settings := system_setting.GetCASSettings()

	nameCandidates := []string{settings.EffectiveNameAttribute(), "name", "nick", "cn", "displayName"}
	emailCandidates := []string{settings.EffectiveEmailAttribute(), "email", "mail"}
	displayName := firstCASAttribute(validation.Attributes, nameCandidates...)
	email := firstCASAttribute(validation.Attributes, emailCandidates...)
	// Which attributes a deployment releases, and in which shape, is decided by
	// the CAS server, so a field that does not arrive is the most likely
	// configuration error and the one nothing else records. Report the candidate
	// names for the field, and alongside them the shape of every attribute that
	// did arrive, so an attribute carrying something can be told from one the
	// server left empty: names and JSON kinds, never a value.
	if displayName == "" {
		logger.LogError(ctx, common.LogText("[OAuth-CAS] Display name attribute not resolved; candidates: %s; CAS returned: %s", casAttributeReport(validation.Attributes, nameCandidates...), casAttributeShapes(validation.Attributes)))
	}
	if email == "" {
		logger.LogError(ctx, common.LogText("[OAuth-CAS] Email attribute not resolved; candidates: %s; CAS returned: %s", casAttributeReport(validation.Attributes, emailCandidates...), casAttributeShapes(validation.Attributes)))
	}
	if settings.GroupEnabled {
		// A department attribute this deployment named but does not receive is a
		// wrong name or an attribute the server withholds, and the sign-in would
		// otherwise apply no group without saying why.
		department := settings.EffectiveGroupAttribute()
		if casAttributeString(validation.Attributes, department) == "" {
			logger.LogError(ctx, common.LogText("[OAuth-CAS] Department attribute not resolved; candidates: %s; CAS returned: %s", casAttributeReport(validation.Attributes, department), casAttributeShapes(validation.Attributes)))
		}
	}

	logger.LogDebug(ctx, "[OAuth-CAS] GetUserInfo success: principal=%s", validation.Principal)

	return &OAuthUser{
		ProviderUserID: validation.Principal,
		Username:       validation.Principal,
		DisplayName:    displayName,
		Email:          email,
		Extra: map[string]any{
			"attributes": validation.Attributes,
		},
	}, nil
}

func (p *CASProvider) IsUserIDTaken(providerUserID string) bool {
	return model.IsCasIdAlreadyTaken(providerUserID)
}

func (p *CASProvider) FillUserByProviderID(user *model.User, providerUserID string) error {
	user.CasId = providerUserID
	return user.FillUserByCasId()
}

func (p *CASProvider) SetProviderUserID(user *model.User, providerUserID string) {
	user.CasId = providerUserID
}

func (p *CASProvider) GetProviderPrefix() string {
	return "cas_"
}

// BackfillsMissingProfile reports that a CAS sign-in may fill in a display name
// and an email address an existing account does not have yet.
//
// The attributes come from the campus directory for the account that just
// authenticated through ticket validation, so they are the school's own record
// rather than something the account holder chose. An account created before the
// attribute names were configured therefore catches up on its next sign-in.
func (p *CASProvider) BackfillsMissingProfile() bool {
	return true
}

// AssertedGroup returns the department this assertion places the account in, or
// "" when an administrator has not switched the grouping on or the assertion
// carries no department. The switch is what asks for it, so no deployment groups
// its accounts by department without saying so.
func (p *CASProvider) AssertedGroup(token *OAuthToken) string {
	settings := system_setting.GetCASSettings()
	if !settings.GroupEnabled || token == nil || token.CASValidation == nil {
		return ""
	}
	return firstCASAttribute(token.CASValidation.Attributes, settings.EffectiveGroupAttribute())
}

// ProviderUserIDColumn returns the users-table column storing this provider's user ID.
func (p *CASProvider) ProviderUserIDColumn() string {
	return "cas_id"
}

// firstCASAttribute returns the first of the named attributes that carries a
// usable string, so a deployment that renames its attributes still resolves.
func firstCASAttribute(attributes map[string]any, names ...string) string {
	for _, name := range names {
		if value := casAttributeString(attributes, name); value != "" {
			return value
		}
	}
	return ""
}

// casAttributeString reads one attribute as a string. CAS returns multi-valued
// attributes as arrays and single-valued ones as strings, so both shapes are
// accepted.
func casAttributeString(attributes map[string]any, name string) string {
	switch typed := attributes[name].(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

// casShapeItemLimit and casShapeDepth bound how much of a value is described:
// elements listed per collection, and how deep a nested collection is opened
// before it is summarized by size. A CAS deployment can return a whole directory
// object as one attribute, and that must not flood the log.
const (
	casShapeItemLimit = 4
	casShapeDepth     = 2
)

// casAttributeReport describes the candidate attributes of one field without
// revealing any of their values: for each of the names, whether it arrived and
// the shape of the value if it did. A configured attribute name is usually one of
// the fallbacks already in the list, so each name is described once.
func casAttributeReport(attributes map[string]any, names ...string) string {
	reported := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		value, ok := attributes[name]
		if !ok {
			reported = append(reported, name+"=absent")
			continue
		}
		reported = append(reported, name+"="+casAttributeShape(value, casShapeDepth))
	}
	return strings.Join(reported, ", ")
}

// casAttributeShapes describes every returned attribute the same way, sorted by
// name, so the ones that carry something can be told from the ones the CAS server
// left empty in a single line.
func casAttributeShapes(attributes map[string]any) string {
	shapes := make([]string, 0, len(attributes))
	for name, value := range attributes {
		shapes = append(shapes, name+"="+casAttributeShape(value, casShapeDepth))
	}
	slices.Sort(shapes)
	return strings.Join(shapes, ", ")
}

// casAttributeShape describes one attribute value without revealing it: the JSON
// kind and, for a string, array or object, how much of it there is. Attribute
// values are PII, so no value is ever written — a test asserts that for strings,
// objects and arrays alike.
//
// Describing a collection one level below the attribute is what tells a list of
// strings this provider reads from a structure it cannot, such as an object that
// holds the value under a key.
func casAttributeShape(value any, depth int) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case string:
		return fmt.Sprintf("string(%d)", len(typed))
	case []any:
		if depth <= 0 {
			return fmt.Sprintf("array(%d)", len(typed))
		}
		shown := min(len(typed), casShapeItemLimit)
		items := make([]string, 0, shown+1)
		for _, item := range typed[:shown] {
			items = append(items, casAttributeShape(item, depth-1))
		}
		if len(typed) > shown {
			items = append(items, fmt.Sprintf("+%d", len(typed)-shown))
		}
		return "array[" + strings.Join(items, ", ") + "]"
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		if depth <= 0 || len(keys) > casShapeItemLimit {
			return fmt.Sprintf("object(%d keys)", len(typed))
		}
		return "object{" + strings.Join(keys, ", ") + "}"
	default:
		return fmt.Sprintf("%T", typed)
	}
}
