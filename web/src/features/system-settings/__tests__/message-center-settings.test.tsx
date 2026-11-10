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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../components/settings-page-context'
import { MessageCenterSettingsSection } from '../integrations/message-center-settings-section'

const credentials = {
  enabled: true,
  baseUrl: 'http://message.example.edu.cn:9200',
  appId: 'app-1',
  token: 'token-1',
}

afterEach(() => {
  vi.restoreAllMocks()
  document.body.innerHTML = ''
})

/** The save button lives in the page header portal, which needs a container. */
function renderSection(defaultValues: typeof credentials) {
  const actions = document.createElement('div')
  document.body.appendChild(actions)
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <SettingsPageProvider actionsContainer={actions}>
        <MessageCenterSettingsSection defaultValues={defaultValues} />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
}

function save() {
  fireEvent.click(
    screen.getByRole('button', { name: 'Save message center settings' })
  )
}

describe('campus message center settings', () => {
  it('a disabled message center hides its credentials and saves the switch alone', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    renderSection({ ...credentials, enabled: false })

    expect(
      screen.queryByLabelText('Message Center Base URL')
    ).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Message Center Token')).not.toBeInTheDocument()

    fireEvent.click(
      screen.getByRole('switch', { name: 'Enable the campus message center' })
    )
    save()

    await waitFor(() =>
      expect(put).toHaveBeenCalledWith('/api/option/', {
        key: 'msgcenter.enabled',
        value: 'true',
      })
    )
  })

  it('an enabled message center saves each credential it was given', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    renderSection({ ...credentials, appId: '', token: '' })

    fireEvent.change(screen.getByLabelText('Message Center App ID'), {
      target: { value: 'app-2' },
    })
    fireEvent.change(
      screen.getByLabelText('Message Center Token', { selector: 'input' }),
      { target: { value: 'token-2' } }
    )
    save()

    await waitFor(() => expect(put).toHaveBeenCalledTimes(2))
    expect(put).toHaveBeenCalledWith('/api/option/', {
      key: 'msgcenter.app_id',
      value: 'app-2',
    })
    expect(put).toHaveBeenCalledWith('/api/option/', {
      key: 'msgcenter.token',
      value: 'token-2',
    })
  })
})
