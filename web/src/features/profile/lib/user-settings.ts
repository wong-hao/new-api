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
import { readCachedStatus } from '@/lib/status-query'

import { NOTIFICATION_METHODS, OA_CHANNELS } from '../constants'
import type { NotifyType, UpdateUserSettingsRequest } from '../types'
import { parseUserSettings } from './format'

/** Whether the site has a campus message center that can deliver a notice at all. */
export function messageCenterConfigured(): boolean {
  return readCachedStatus()?.msgcenter_configured === true
}

/** The method a user who never chose is served by. */
export function defaultNotifyType(): NotifyType {
  return messageCenterConfigured() ? 'oa' : 'email'
}

/**
 * The methods a user may pick. The message center is offered only once the site has
 * configured one: an unconfigured center would take the choice and deliver nothing.
 */
export function availableNotificationMethods(): readonly {
  value: NotifyType
  label: string
}[] {
  return messageCenterConfigured()
    ? NOTIFICATION_METHODS
    : NOTIFICATION_METHODS.filter((method) => method.value !== 'oa')
}

/**
 * Channels of the campus message center in the order they are saved, with
 * anything the center cannot reach a campus account by dropped. An empty result
 * means the server's default channels.
 */
export function normalizeOAChannels(configured: string): string {
  return OA_CHANNELS.filter((channel) => configured.split(',').includes(channel.value))
    .map((channel) => channel.value)
    .join(',')
}

/** Switches whose absence means "never chosen", which the server resolves */
type DefaultedSwitch = 'quota_warning_enabled' | 'channel_notify_enabled'

type NormalizedUserSettings = Required<
  Omit<UpdateUserSettingsRequest, DefaultedSwitch>
> &
  Pick<UpdateUserSettingsRequest, DefaultedSwitch> & { notify_type: NotifyType }

/**
 * Settings sent back on every save. A switch the user never chose stays
 * undefined, so the save leaves it unset and its default keeps applying. A
 * threshold of 0 means the site default.
 */
export function normalizeUserSettings(
  setting?: string
): NormalizedUserSettings {
  const parsed = parseUserSettings(setting)
  const notifyType =
    availableNotificationMethods().find(
      (method) => method.value === parsed.notify_type
    )?.value ?? defaultNotifyType()
  return {
    notify_type: notifyType,
    quota_warning_threshold: parsed.quota_warning_threshold ?? 0,
    quota_warning_enabled: parsed.quota_warning_enabled,
    notification_email: parsed.notification_email ?? '',
    webhook_url: parsed.webhook_url ?? '',
    webhook_secret: parsed.webhook_secret ?? '',
    bark_url: parsed.bark_url ?? '',
    gotify_url: parsed.gotify_url ?? '',
    gotify_token: parsed.gotify_token ?? '',
    gotify_priority: parsed.gotify_priority ?? 5,
    oa_channels: normalizeOAChannels(parsed.oa_channels ?? ''),
    accept_unset_model_ratio_model:
      parsed.accept_unset_model_ratio_model || false,
    record_ip_log: parsed.record_ip_log || false,
    upstream_model_update_notify_enabled:
      parsed.upstream_model_update_notify_enabled || false,
    channel_notify_enabled: parsed.channel_notify_enabled,
  }
}
