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
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../components/settings-page-context'
import { MessageCenterSettingsSection } from '../integrations/message-center-settings-section'

const credentials = {
  enabled: true,
  baseUrl: 'http://message.example.edu.cn:9200',
  appId: 'app-1',
  token: 'token-1',
  channels: 'wechat',
  moduleId: '',
}

afterEach(() => {
  vi.restoreAllMocks()
})

/**
 * The save button lives in the page header portal, which needs a container. Each test
 * gets its own and leaves it in place: React owns the portal's children, so removing the
 * container underneath it breaks the unmount.
 */
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

function saveButton() {
  return screen.getByRole('button', { name: 'Save message center settings' })
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
    expect(
      screen.queryByLabelText('Message Center Token')
    ).not.toBeInTheDocument()

    fireEvent.click(
      screen.getByRole('switch', { name: 'Enable the campus message center' })
    )
    fireEvent.click(saveButton())

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
    fireEvent.click(saveButton())

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

  it('saving a site that sends through EIP asks for its message source first', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    renderSection(credentials)
    const user = userEvent.setup()

    await user.click(screen.getByRole('combobox'))
    await user.click(await screen.findByRole('option', { name: 'EIP' }))
    // An open popup hides the rest of the page from the accessibility tree.
    await user.keyboard('{Escape}')

    expect(screen.getByLabelText('EIP message source')).toBeInTheDocument()
    expect(
      screen.getByText('Select a message source for EIP')
    ).toBeInTheDocument()
    expect(saveButton()).toBeDisabled()

    await user.click(screen.getByLabelText('EIP message source'))
    await user.click(
      await screen.findByRole('option', { name: 'Announcement' })
    )
    await user.keyboard('{Escape}')

    await waitFor(() => expect(saveButton()).toBeEnabled())
    await user.click(saveButton())

    await waitFor(() => expect(put).toHaveBeenCalledTimes(2))
    expect(put).toHaveBeenCalledWith('/api/option/', {
      key: 'msgcenter.channels',
      value: 'wechat,eip',
    })
    expect(put).toHaveBeenCalledWith('/api/option/', {
      key: 'msgcenter.module_id',
      value: 'ANNOUNCEMENT',
    })
  })

  it('a site that sends through no channel cannot be saved', async () => {
    vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
    renderSection(credentials)
    const user = userEvent.setup()

    await user.click(screen.getByRole('combobox'))
    await user.keyboard('{Backspace}')
    await user.keyboard('{Escape}')

    expect(screen.getByText('Select at least one channel')).toBeInTheDocument()
    expect(saveButton()).toBeDisabled()
  })
})
