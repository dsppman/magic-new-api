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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { adjustUserGroupRatio } from '../api'

interface UserGroupRatioDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number
  currentRatio: number | null
  onSuccess: () => void
}

export function UserGroupRatioDialog(props: UserGroupRatioDialogProps) {
  const { t } = useTranslation()
  const [value, setValue] = useState('')
  const [loading, setLoading] = useState(false)

  // Seed the input with the current ratio each time the dialog opens.
  useEffect(() => {
    if (props.open) {
      setValue(props.currentRatio === null ? '' : String(props.currentRatio))
    }
  }, [props.open, props.currentRatio])

  const submit = async (ratio: number | null) => {
    setLoading(true)
    try {
      const result = await adjustUserGroupRatio({
        id: props.userId,
        action: 'set_group_ratio',
        group_ratio: ratio,
      })
      if (result.success) {
        toast.success(
          ratio === null
            ? t('Ratio cleared')
            : t('Ratio updated successfully')
        )
        props.onOpenChange(false)
        props.onSuccess()
      } else {
        toast.error(result.message || t('Failed to update ratio'))
      }
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : t('Failed to update ratio'))
    } finally {
      setLoading(false)
    }
  }

  const handleConfirm = () => {
    const trimmed = value.trim()
    // Empty input clears the exclusive ratio (falls back to the group ratio).
    if (trimmed === '') {
      submit(null)
      return
    }
    const ratio = Number(trimmed)
    if (!Number.isFinite(ratio) || ratio < 0) {
      toast.error(t('Ratio must be a number not less than 0'))
      return
    }
    submit(ratio)
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Set Exclusive Group Ratio')}
      description={t(
        'This ratio replaces the group ratio for this user. 0 means free; leave empty to disable.'
      )}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            variant='outline'
            onClick={() => submit(null)}
            disabled={loading || props.currentRatio === null}
          >
            {t('Clear')}
          </Button>
          <Button onClick={handleConfirm} disabled={loading}>
            {loading ? t('Processing...') : t('Confirm')}
          </Button>
        </>
      }
    >
      <div className='space-y-2'>
        <Label>{t('Ratio')}</Label>
        <Input
          type='number'
          step={0.1}
          min={0}
          placeholder={t('Enter ratio (e.g. 0.8)')}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') handleConfirm()
          }}
        />
      </div>
    </Dialog>
  )
}
