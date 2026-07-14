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
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { adjustUserGroupRatio } from '../api'

interface UserGroupRatioOverridesProps {
  userId: number
  /** Available group names (billing/using groups). */
  groups: string[]
  /** Current "using group -> ratio" overrides for this user. */
  overrides: Record<string, number>
  /** Called after a successful save to reload the user. */
  onSaved: () => void
}

/**
 * Per-user exclusive ratios, keyed by billing group — the user-level analog of
 * the group special-ratio rules. Each override says "when this user is billed as
 * group X, use ratio R instead of the group ratio". Mutations POST the full map
 * to /api/user/manage (set_group_ratio) and then reload.
 */
export function UserGroupRatioOverrides(props: UserGroupRatioOverridesProps) {
  const { t } = useTranslation()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingGroup, setEditingGroup] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const entries = useMemo(
    () =>
      Object.entries(props.overrides).sort((a, b) => a[0].localeCompare(b[0])),
    [props.overrides]
  )

  const saveMap = async (map: Record<string, number>, removed = false) => {
    setSaving(true)
    try {
      const result = await adjustUserGroupRatio({
        id: props.userId,
        action: 'set_group_ratio',
        group_ratio: Object.keys(map).length > 0 ? map : null,
      })
      if (result.success) {
        toast.success(
          removed ? t('Ratio cleared') : t('Ratio updated successfully')
        )
        setDialogOpen(false)
        props.onSaved()
      } else {
        toast.error(result.message || t('Failed to update ratio'))
      }
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : t('Failed to update ratio'))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = (group: string) => {
    const next = { ...props.overrides }
    delete next[group]
    saveMap(next, true)
  }

  return (
    <div className='space-y-2'>
      <div className='flex items-start justify-between gap-2'>
        <div>
          <Label>{t('Exclusive Group Ratio')}</Label>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Per-user ratio that overrides the group ratio (0 = free, empty = disabled)'
            )}
          </p>
        </div>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => {
            setEditingGroup(null)
            setDialogOpen(true)
          }}
        >
          <Plus className='mr-1 h-4 w-4' />
          {t('Add ratio override')}
        </Button>
      </div>

      {entries.length === 0 ? (
        <p className='text-muted-foreground text-sm'>{t('None')}</p>
      ) : (
        <div className='space-y-2'>
          {entries.map(([group, ratio]) => (
            <div
              key={group}
              className='flex items-center justify-between rounded-md border px-3 py-2'
            >
              <span className='font-medium'>{group}</span>
              <div className='flex items-center gap-2'>
                <span className='text-sm'>{ratio}</span>
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  aria-label={t('Edit')}
                  onClick={() => {
                    setEditingGroup(group)
                    setDialogOpen(true)
                  }}
                >
                  <Pencil className='h-4 w-4' />
                </Button>
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  aria-label={t('Delete')}
                  disabled={saving}
                  onClick={() => handleDelete(group)}
                >
                  <Trash2 className='h-4 w-4' />
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      <UserGroupRatioOverrideDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        groups={props.groups}
        overrides={props.overrides}
        editingGroup={editingGroup}
        saving={saving}
        onSave={(group, ratio) => {
          saveMap({ ...props.overrides, [group]: ratio })
        }}
      />
    </div>
  )
}

interface UserGroupRatioOverrideDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  groups: string[]
  overrides: Record<string, number>
  editingGroup: string | null
  saving: boolean
  onSave: (group: string, ratio: number) => void
}

function UserGroupRatioOverrideDialog(props: UserGroupRatioOverrideDialogProps) {
  const { t } = useTranslation()
  const isEdit = props.editingGroup !== null
  const [group, setGroup] = useState<string | null>(null)
  const [ratio, setRatio] = useState('')

  useEffect(() => {
    if (!props.open) {
      setGroup(null)
      setRatio('')
      return
    }
    if (props.editingGroup !== null) {
      setGroup(props.editingGroup)
      setRatio(String(props.overrides[props.editingGroup] ?? ''))
    } else {
      setGroup(null)
      setRatio('')
    }
  }, [props.open, props.editingGroup, props.overrides])

  // When adding, only offer groups that don't already have an override.
  const addOptions = useMemo(
    () => props.groups.filter((name) => !Object.hasOwn(props.overrides, name)),
    [props.groups, props.overrides]
  )

  const handleSave = () => {
    if (!group) return
    const trimmed = ratio.trim()
    if (trimmed === '') return
    const value = Number.parseFloat(trimmed)
    if (!Number.isFinite(value) || value < 0) {
      toast.error(t('Ratio must be a number not less than 0'))
      return
    }
    props.onSave(group, value)
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={isEdit ? t('Edit ratio override') : t('Add ratio override')}
      description={t(
        'Configure a custom ratio for when users use a specific token group.'
      )}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            onClick={handleSave}
            disabled={props.saving || !group || ratio.trim() === ''}
          >
            {isEdit ? t('Update') : t('Add')}
          </Button>
        </>
      }
    >
      <div className='space-y-4 py-4'>
        <div className='space-y-2'>
          <Label>{t('Billing group')}</Label>
          {isEdit ? (
            <Input value={props.editingGroup ?? ''} readOnly />
          ) : (
            <Select
              value={group}
              onValueChange={(v) => {
                if (typeof v === 'string' && v !== '') setGroup(v)
              }}
            >
              <SelectTrigger className='w-full'>
                <SelectValue placeholder={t('Select a group')} />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {addOptions.map((name) => (
                    <SelectItem key={name} value={name}>
                      {name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          )}
          <p className='text-muted-foreground text-xs'>
            {t('The token group that will have a custom ratio')}
          </p>
        </div>
        <div className='space-y-2'>
          <Label>{t('Ratio')}</Label>
          <Input
            value={ratio}
            onChange={(e) => {
              const val = e.target.value
              if (val === '' || !Number.isNaN(Number.parseFloat(val))) {
                setRatio(val)
              }
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleSave()
            }}
            placeholder={t('Enter ratio (e.g. 0.8)')}
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'Per-user ratio that overrides the group ratio (0 = free, empty = disabled)'
            )}
          </p>
        </div>
      </div>
    </Dialog>
  )
}
