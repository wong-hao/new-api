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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { LogStatCards } from '../log-stat-cards'

beforeEach(() => {
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'dashboard-user',
    role: 1,
    quota: 1000000,
    used_quota: 1000,
    request_count: 1,
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url !== '/api/data/self') {
      throw new Error(`Unexpected dashboard request: ${url}`)
    }
    return {
      data: {
        success: true,
        data: [
          {
            model_name: 'deepseek-v41-flash',
            created_at: 1794048600,
            count: 12,
            quota: 8300,
            token_used: 170000000,
            cache_read_tokens: 2688,
            cache_creation_tokens: 0,
          },
        ],
      },
    }
  })
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
})

describe('LogStatCards cache stat cards', () => {
  it('renders six cards whose cache card is labelled as cache read hits', async () => {
    render(<LogStatCards />)

    await screen.findByRole('group', { name: 'Cache Read (hits)' })
    expect(screen.queryByRole('group', { name: 'Cache Write' })).toBeNull()
    expect(screen.getAllByRole('group')).toHaveLength(6)
  })

  it('shows the cached token total reported by the quota data', async () => {
    render(<LogStatCards />)

    const cacheReadCard = await screen.findByRole('group', {
      name: 'Cache Read (hits)',
    })
    expect(cacheReadCard).toHaveTextContent('2,688')
  })

  it('lays the cards out on a six-column grid at the large breakpoint', async () => {
    const { container } = render(<LogStatCards />)

    await screen.findByRole('group', { name: 'Cache Read (hits)' })
    const grid = container.querySelector('.grid')
    expect(grid?.className).toContain('lg:grid-cols-6')
  })
})
