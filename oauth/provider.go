package oauth

import (
	"context"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// Provider defines the interface for OAuth providers
type Provider interface {
	// GetName returns the display name of the provider (e.g., "GitHub", "Discord")
	GetName() string

	// IsEnabled returns whether this OAuth provider is enabled
	IsEnabled() bool

	// ExchangeToken exchanges the authorization code for an access token
	// The gin.Context is passed for providers that need request info (e.g., for redirect_uri)
	ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error)

	// GetUserInfo retrieves user information using the access token
	GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error)

	// IsUserIDTaken checks if the provider user ID is already associated with an account
	IsUserIDTaken(providerUserID string) bool

	// FillUserByProviderID fills the user model by provider user ID
	FillUserByProviderID(user *model.User, providerUserID string) error

	// SetProviderUserID sets the provider user ID on the user model
	SetProviderUserID(user *model.User, providerUserID string)

	// GetProviderPrefix returns the prefix for auto-generated usernames (e.g., "github_")
	GetProviderPrefix() string

	// ProviderUserIDColumn returns the users-table column that stores this provider's
	// user ID, used by bind flows to update only the binding column instead of
	// writing back a full user snapshot. Providers that persist bindings elsewhere
	// (e.g. the user_oauth_bindings table) return an empty string.
	ProviderUserIDColumn() string
}

// VerifiedEmailProvider is implemented by providers that can list the email
// addresses they have confirmed for the signed-in account. The login flow uses
// it to match a provider account against an existing account's email.
type VerifiedEmailProvider interface {
	GetVerifiedEmails(ctx context.Context, token *OAuthToken) ([]string, error)
}

// ProfileBackfillProvider is implemented by providers whose returned display
// name and email may fill in a field an existing account does not have yet.
//
// Opting in is a statement about the provider's directory, not about the login
// flow: the values are only written where the account holds none, so a provider
// whose name and address are optional and self-chosen (GitHub, for example) must
// not implement it, while an institutional single sign-on server that asserts
// its own record for the account that just authenticated may.
type ProfileBackfillProvider interface {
	// BackfillsMissingProfile reports whether a login through this provider may
	// fill in a missing display name and email on an existing account.
	BackfillsMissingProfile() bool
}

// GroupAssignmentProvider is implemented by providers whose assertion names the
// group an account belongs in.
//
// The provider only reports the name; whether the account is moved, and what a
// new group is billed at, stay in the login flow, so a provider never writes user
// settings itself.
type GroupAssignmentProvider interface {
	// AssertedGroup returns the group this assertion names, or "" when the
	// deployment has not asked for one or the assertion carries none.
	AssertedGroup(token *OAuthToken) string
}
