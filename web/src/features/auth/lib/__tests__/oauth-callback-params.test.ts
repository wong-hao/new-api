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
import { describe, expect, test } from 'vitest'

import { resolveOAuthCallbackCode } from '../oauth-callback-params'

describe('resolveOAuthCallbackCode', () => {
  test('an OAuth2 authorization code is returned unchanged', () => {
    expect(resolveOAuthCallbackCode({ code: 'auth-code' })).toBe('auth-code')
  })

  test('a CAS service ticket is returned when no code is present', () => {
    expect(resolveOAuthCallbackCode({ ticket: 'ST-1-abc' })).toBe('ST-1-abc')
  })

  test('the code wins when a provider returns both', () => {
    expect(
      resolveOAuthCallbackCode({ code: 'auth-code', ticket: 'ST-1-abc' })
    ).toBe('auth-code')
  })

  test('a callback with neither parameter resolves empty', () => {
    expect(resolveOAuthCallbackCode({})).toBe('')
  })

  test('an explicit empty code falls through to the ticket', () => {
    expect(resolveOAuthCallbackCode({ code: '', ticket: 'ST-1-abc' })).toBe(
      'ST-1-abc'
    )
  })
})
