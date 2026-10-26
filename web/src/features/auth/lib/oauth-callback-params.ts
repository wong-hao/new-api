/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

/** Search parameters a provider may use to return its callback credential. */
export interface OAuthCallbackCredentialSearch {
  code?: string
  ticket?: string
}

/**
 * Resolve the credential a provider returned to `/oauth/:provider`.
 *
 * OAuth2 providers return an authorization `code`; a CAS server returns a
 * service `ticket` at the same position in the round trip. The backend accepts
 * either as the value it exchanges, so both become the API's `code` parameter.
 * `code` wins so a provider that sends both keeps its OAuth2 semantics.
 */
export function resolveOAuthCallbackCode(
  search: OAuthCallbackCredentialSearch
): string {
  if (search.code) return search.code
  return search.ticket ?? ''
}
