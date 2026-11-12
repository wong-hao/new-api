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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { MultiSelect } from '@/components/multi-select'
import { PasswordInput } from '@/components/password-input'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  MESSAGE_CENTER_CHANNELS,
  MESSAGE_CENTER_MODULE_IDS,
} from '@/lib/msgcenter'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const schema = z.object({
  enabled: z.boolean(),
  baseUrl: z.string(),
  appId: z.string(),
  token: z.string(),
  /** Channels to send through, comma separated */
  channels: z.string(),
  /** Message source the EIP channel requires */
  moduleId: z.string(),
  /** Label the centre puts on a message it draws as a card */
  buttonText: z.string(),
  /** Template code per channel, as the JSON object the centre takes */
  template: z
    .string()
    .refine(
      (value) => {
        if (value.trim() === '') return true
        try {
          const parsed: unknown = JSON.parse(value)
          return (
            typeof parsed === 'object' &&
            parsed !== null &&
            !Array.isArray(parsed)
          )
        } catch {
          return false
        }
      },
      { message: 'Enter a JSON object' }
    ),
})

// NOTE: react-hook-form resolver uses the schema input type
type Values = z.input<typeof schema>

export function MessageCenterSettingsSection({
  defaultValues,
}: {
  defaultValues: Values
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: {
      enabled: defaultValues.enabled,
      baseUrl: defaultValues.baseUrl ?? '',
      appId: defaultValues.appId ?? '',
      token: defaultValues.token ?? '',
      channels: defaultValues.channels ?? '',
      moduleId: defaultValues.moduleId ?? '',
      buttonText: defaultValues.buttonText ?? '',
      template: defaultValues.template ?? '',
    },
  })

  const { isDirty, isSubmitting } = form.formState
  const enabled = form.watch('enabled')
  const selected = (form.watch('channels') ?? '')
    .split(',')
    .filter((channel) => channel !== '')
  const moduleId = form.watch('moduleId') ?? ''
  // An EIP message carries a source; without one the centre rejects the message, so a
  // site that cannot name one may not be saved.
  const eip = selected.includes('eip')
  const canSave = selected.length > 0 && (!eip || moduleId !== '')

  const channelOptions = useMemo(
    () =>
      MESSAGE_CENTER_CHANNELS.map((channel) => ({
        value: channel.value,
        label: t(channel.label),
      })),
    [t]
  )
  const moduleIdOptions = useMemo(
    () =>
      MESSAGE_CENTER_MODULE_IDS.map((module) => ({
        value: module.value,
        label: t(module.label),
      })),
    [t]
  )

  async function onSubmit(values: Values) {
    const updates: Array<{ key: string; value: string }> = []

    if (values.enabled !== defaultValues.enabled) {
      updates.push({
        key: 'msgcenter.enabled',
        value: String(values.enabled),
      })
    }
    if ((values.baseUrl || '') !== (defaultValues.baseUrl || '')) {
      updates.push({
        key: 'msgcenter.base_url',
        value: String(values.baseUrl || ''),
      })
    }
    if ((values.appId || '') !== (defaultValues.appId || '')) {
      updates.push({
        key: 'msgcenter.app_id',
        value: String(values.appId || ''),
      })
    }
    if ((values.token || '') !== (defaultValues.token || '')) {
      updates.push({
        key: 'msgcenter.token',
        value: String(values.token || ''),
      })
    }
    if ((values.channels || '') !== (defaultValues.channels || '')) {
      updates.push({
        key: 'msgcenter.channels',
        value: String(values.channels || ''),
      })
    }
    if ((values.moduleId || '') !== (defaultValues.moduleId || '')) {
      updates.push({
        key: 'msgcenter.module_id',
        value: String(values.moduleId || ''),
      })
    }
    if ((values.buttonText || '') !== (defaultValues.buttonText || '')) {
      updates.push({
        key: 'msgcenter.button_text',
        value: String(values.buttonText || ''),
      })
    }
    if ((values.template || '') !== (defaultValues.template || '')) {
      updates.push({
        key: 'msgcenter.template',
        value: String(values.template || ''),
      })
    }

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }

    form.reset(values)
  }

  return (
    <SettingsSection title={t('Message Center')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending || isSubmitting}
            isSaveDisabled={!isDirty || updateOption.isPending || !canSave}
            saveLabel='Save message center settings'
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable the campus message center')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Send notifications through the campus message center, the default method for users who have not chosen one'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={(v) => field.onChange(v)}
                    disabled={updateOption.isPending || isSubmitting}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          {enabled ? (
            <>
              <FormField
                control={form.control}
                name='baseUrl'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Message Center Base URL')}</FormLabel>
                    <FormControl>
                      <Input
                        type='url'
                        placeholder='http://message.example.edu.cn:9200'
                        autoComplete='off'
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Address of the message center, including its port')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='appId'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Message Center App ID')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('Authorized application appId')}
                        autoComplete='off'
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='token'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Message Center Token')}</FormLabel>
                    <FormControl>
                      <PasswordInput
                        placeholder={t('Token issued to the application')}
                        autoComplete='off'
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='channels'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Channels to send through')}</FormLabel>
                    <FormControl>
                      <MultiSelect
                        options={channelOptions}
                        selected={(field.value ?? '').split(',').filter(Boolean)}
                        onChange={(values) =>
                          // Save them in the order they are sent, not the order the
                          // administrator happened to click them in.
                          field.onChange(
                            MESSAGE_CENTER_CHANNELS.filter((channel) =>
                              values.includes(channel.value)
                            )
                              .map((channel) => channel.value)
                              .join(',')
                          )
                        }
                        placeholder={t('Select at least one channel')}
                        disabled={updateOption.isPending || isSubmitting}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'The message center rejects a message that names a channel this application is not authorized for'
                      )}
                    </FormDescription>
                    <FormMessage>
                      {selected.length < 1
                        ? t('Select at least one channel')
                        : null}
                    </FormMessage>
                  </FormItem>
                )}
              />

              {eip ? (
                <FormField
                  control={form.control}
                  name='moduleId'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('EIP message source')}</FormLabel>
                      <Select
                        items={moduleIdOptions}
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectGroup>
                            {MESSAGE_CENTER_MODULE_IDS.map((module) => (
                              <SelectItem key={module.value} value={module.value}>
                                {t(module.label)}
                              </SelectItem>
                            ))}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      <FormMessage>
                        {moduleId === ''
                          ? t('Select a message source for EIP')
                          : null}
                      </FormMessage>
                    </FormItem>
                  )}
                />
              ) : null}

              <FormField
                control={form.control}
                name='buttonText'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Button text')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('View details')}
                        autoComplete='off'
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Shown on a message the centre draws as a card; a channel that renders plain text ignores it'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='template'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Message template')}</FormLabel>
                    <FormControl>
                      <Textarea
                        rows={3}
                        placeholder='{"eip":"someCode"}'
                        autoComplete='off'
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Template code per channel, as a JSON object. The template itself lives in the message centre, and a code it has not registered is rejected'
                      )}
                    </FormDescription>
                    <FormMessage>
                      {form.formState.errors.template
                        ? t('Enter a JSON object')
                        : null}
                    </FormMessage>
                  </FormItem>
                )}
              />
            </>
          ) : null}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
