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
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

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
import { Switch } from '@/components/ui/switch'

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
})

// NOTE: react-hook-form resolver uses the schema input type
type Values = z.input<typeof schema>

export function MessageCenterSettingsSection({
  defaultValues,
}: {
  defaultValues: {
    enabled: boolean
    baseUrl: string
    appId: string
    token: string
  }
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
    },
  })

  const { isDirty, isSubmitting } = form.formState
  const enabled = form.watch('enabled')

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
            isSaveDisabled={!isDirty}
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
                      {t(
                        'Address of the message center, including its port'
                      )}
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
            </>
          ) : null}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
