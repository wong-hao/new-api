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
import { afterEach, describe, expect, test } from 'vitest'

import {
  clearOAuthCallbackState,
  getOAuthSessionStorage,
  markOAuthPopup,
  providerStateComesFromStorage,
  readOAuthCallbackState,
  rememberOAuthCallbackState,
  resolveOAuthCallbackMode,
  type OAuthModeStorage,
} from '../oauth-callback-mode'

function fakeStorage(initial: Record<string, string> = {}): OAuthModeStorage {
  const data = new Map(Object.entries(initial))
  return {
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => void data.set(key, value),
  }
}

const openOpener = { closed: false }
const bindState = 'bind-state'

describe('resolveOAuthCallbackMode', () => {
  test('matching provider and state mark is treated as a bind flow', () => {
    const storage = fakeStorage()
    expect(markOAuthPopup(storage, 'oidc', bindState, 'bind')).toBe(true)

    expect(
      resolveOAuthCallbackMode('oidc', bindState, {
        opener: openOpener,
        storage,
      })
    ).toBe('bind')
  })

  test('verification markers cannot be confused with account binding', () => {
    const storage = fakeStorage()
    expect(
      markOAuthPopup(storage, 'oidc', 'verification-state', 'verify')
    ).toBe(true)
    expect(
      resolveOAuthCallbackMode('oidc', 'verification-state', {
        opener: openOpener,
        storage,
      })
    ).toBe('verify')
    expect(
      resolveOAuthCallbackMode('oidc', bindState, {
        opener: openOpener,
        storage,
      })
    ).toBe('login')
  })

  // Regression: a tab opened from an external link (Slack, e-mail, another
  // site) keeps a live window.opener across the cross-origin round trip to the
  // identity provider. Treating that opener as proof of a bind flow made every
  // such login hang on the binding screen until the 30s handshake deadline.
  test('login redirect in a tab with a foreign opener stays a login flow', () => {
    const storage = fakeStorage()

    expect(
      resolveOAuthCallbackMode('oidc', bindState, {
        opener: openOpener,
        storage,
      })
    ).toBe('login')
  })

  test('bind marker for another provider does not hijack this callback', () => {
    const storage = fakeStorage()
    markOAuthPopup(storage, 'github', bindState, 'bind')

    expect(
      resolveOAuthCallbackMode('oidc', bindState, {
        opener: openOpener,
        storage,
      })
    ).toBe('login')
  })

  test('stale bind marker does not hijack a later callback', () => {
    const storage = fakeStorage()
    markOAuthPopup(storage, 'oidc', 'previous-state', 'bind')

    expect(
      resolveOAuthCallbackMode('oidc', bindState, {
        opener: openOpener,
        storage,
      })
    ).toBe('login')
  })

  test('bind marker without an opener falls back to login', () => {
    const storage = fakeStorage()
    markOAuthPopup(storage, 'oidc', bindState, 'bind')

    expect(
      resolveOAuthCallbackMode('oidc', bindState, {
        opener: null,
        storage,
      })
    ).toBe('login')
  })

  test('closed opener falls back to login', () => {
    const storage = fakeStorage()
    markOAuthPopup(storage, 'oidc', bindState, 'bind')

    expect(
      resolveOAuthCallbackMode('oidc', bindState, {
        opener: { closed: true },
        storage,
      })
    ).toBe('login')
  })

  test('missing storage degrades to login instead of throwing', () => {
    expect(
      resolveOAuthCallbackMode('oidc', bindState, {
        opener: openOpener,
        storage: null,
      })
    ).toBe('login')
  })

  test('storage read failure degrades to login instead of throwing', () => {
    const storage: OAuthModeStorage = {
      getItem: () => {
        throw new Error('blocked')
      },
      setItem: () => undefined,
    }

    expect(
      resolveOAuthCallbackMode('oidc', bindState, {
        opener: openOpener,
        storage,
      })
    ).toBe('login')
  })
})

describe('OAuth bind popup storage', () => {
  test('blocked sessionStorage getter is contained', () => {
    const owner = {
      get sessionStorage(): OAuthModeStorage {
        throw new Error('blocked')
      },
    }

    expect(getOAuthSessionStorage(owner)).toBe(null)
  })

  test('marking reports unavailable or unwritable storage', () => {
    const storage: OAuthModeStorage = {
      getItem: () => null,
      setItem: () => {
        throw new Error('blocked')
      },
    }

    expect(markOAuthPopup(null, 'oidc', bindState, 'bind')).toBe(false)
    expect(markOAuthPopup(storage, 'oidc', bindState, 'bind')).toBe(false)
    expect(
      markOAuthPopup(
        {
          getItem: () => null,
          setItem: () => undefined,
        },
        'oidc',
        bindState,
        'bind'
      )
    ).toBe(false)
  })
})

describe('OAuth callback state storage', () => {
  afterEach(() => {
    window.sessionStorage.clear()
  })

  test('CAS supplies its callback state from storage and other providers do not', () => {
    expect(providerStateComesFromStorage('cas')).toBe(true)
    expect(providerStateComesFromStorage('oidc')).toBe(false)
  })

  test('state stored before the round trip is readable at the callback', () => {
    rememberOAuthCallbackState(window.sessionStorage, 'cas', bindState)

    expect(readOAuthCallbackState('cas')).toBe(bindState)
  })

  test('stored state is scoped to the provider that started the flow', () => {
    rememberOAuthCallbackState(window.sessionStorage, 'cas', bindState)

    expect(readOAuthCallbackState('oidc')).toBe(null)
  })

  test('a state that was never stored reads as null', () => {
    expect(readOAuthCallbackState('cas')).toBe(null)
  })

  test('clearing drops the state so a spent flow cannot be replayed', () => {
    rememberOAuthCallbackState(window.sessionStorage, 'cas', bindState)
    clearOAuthCallbackState('cas')

    expect(readOAuthCallbackState('cas')).toBe(null)
  })

  test('missing or unwritable storage is contained instead of throwing', () => {
    const storage: OAuthModeStorage = {
      getItem: () => null,
      setItem: () => {
        throw new Error('blocked')
      },
    }

    expect(() =>
      rememberOAuthCallbackState(null, 'cas', bindState)
    ).not.toThrow()
    expect(() =>
      rememberOAuthCallbackState(storage, 'cas', bindState)
    ).not.toThrow()
  })
})
