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
/**
 * Vocabulary of the campus message center (OA), shared by the site settings that
 * decide what it sends through and by the notification settings a user picks from.
 *
 * The center's other channels need a phone number, an email address or a file, so they
 * are not part of this list: every channel here reaches the recipient's campus account.
 */
export const MESSAGE_CENTER_CHANNELS = [
  { value: 'wechat', label: 'WeChat' },
  { value: 'dingding', label: 'DingTalk' },
  { value: 'eip', label: 'EIP' },
  { value: 'app', label: 'Campus App' },
] as const

/** The message sources the center accepts for the EIP channel, as its moduleID field. */
export const MESSAGE_CENTER_MODULE_IDS = [
  { value: 'COOPERATE', label: 'Workflow' },
  { value: 'ANNOUNCEMENT', label: 'Announcement' },
  { value: 'SURVEY', label: 'Survey' },
  { value: 'MEET', label: 'Meeting' },
  { value: 'JOIN_APP', label: 'Third-party app' },
] as const
