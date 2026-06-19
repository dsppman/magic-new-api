/*
Copyright (C) 2025 QuantumNous

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

import React, { useEffect, useState } from 'react';
import { Button, Col, Form, Row, Spin, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  API,
  showError,
  showSuccess,
  timestamp2string,
} from '../../../helpers';

const { Text } = Typography;

const defaultModels = '';
const defaultRandomDisableCount = 50;
const randomDisableRangeDays = 5;

const buildGroupOptions = (groups = []) =>
  Array.from(new Set(groups))
    .filter(Boolean)
    .map((group) => ({ label: group, value: group }));

const getDefaultRandomDisableTimeRange = () => {
  const end = Math.floor(Date.now() / 1000);
  return [
    timestamp2string(end - randomDisableRangeDays * 24 * 3600),
    timestamp2string(end),
  ];
};

const parseDateTimeToTimestamp = (value) => {
  if (!value) {
    return 0;
  }
  if (value instanceof Date) {
    return Math.floor(value.getTime() / 1000);
  }
  if (typeof value === 'number') {
    return Math.floor(value > 100000000000 ? value / 1000 : value);
  }
  if (typeof value !== 'string') {
    return 0;
  }

  const trimmed = value.trim();
  const match = trimmed.match(
    /^(\d{4})-(\d{1,2})-(\d{1,2})(?:[ T](\d{1,2}):(\d{1,2})(?::(\d{1,2}))?)?$/,
  );
  if (match) {
    const [, year, month, day, hour = '0', minute = '0', second = '0'] = match;
    return Math.floor(
      new Date(
        Number(year),
        Number(month) - 1,
        Number(day),
        Number(hour),
        Number(minute),
        Number(second),
      ).getTime() / 1000,
    );
  }

  const timestamp = new Date(trimmed.replace(' ', 'T')).getTime();
  return Number.isNaN(timestamp) ? 0 : Math.floor(timestamp / 1000);
};

export default function SettingsAutoChannels() {
  const { t } = useTranslation();
  const [count, setCount] = useState(50);
  const [models, setModels] = useState(defaultModels);
  const [group, setGroup] = useState('');
  const [groupOptions, setGroupOptions] = useState([]);
  const [randomDisableCount, setRandomDisableCount] = useState(
    defaultRandomDisableCount,
  );
  const [randomUsedQuota, setRandomUsedQuota] = useState(false);
  const [randomAutoDisable, setRandomAutoDisable] = useState(false);
  const [randomDisableTimeRange, setRandomDisableTimeRange] = useState(
    getDefaultRandomDisableTimeRange,
  );
  const [randomResponseTime, setRandomResponseTime] = useState(false);
  const [loading, setLoading] = useState(false);
  const [disableLoading, setDisableLoading] = useState(false);
  const [lastResult, setLastResult] = useState(null);
  const [lastDisableResult, setLastDisableResult] = useState(null);

  useEffect(() => {
    const fetchGroups = async () => {
      try {
        const res = await API.get('/api/group/');
        if (res?.data?.success && Array.isArray(res.data.data)) {
          const options = buildGroupOptions(res.data.data);
          setGroupOptions(options);
          setGroup((currentGroup) => currentGroup || options[0]?.value || '');
        }
      } catch (error) {
        showError(error?.message || t('加载分组失败'));
      }
    };
    fetchGroups();
  }, [t]);

  const buildRandomDisableTimePayload = () => {
    if (
      !Array.isArray(randomDisableTimeRange) ||
      randomDisableTimeRange.length !== 2
    ) {
      return { error: t('请选择随机自动禁用时间段') };
    }

    const startTime = parseDateTimeToTimestamp(randomDisableTimeRange[0]);
    const endTime = parseDateTimeToTimestamp(randomDisableTimeRange[1]);
    if (!startTime || !endTime) {
      return { error: t('请选择随机自动禁用时间段') };
    }
    if (endTime < startTime) {
      return { error: t('随机自动禁用时间段开始时间不能晚于结束时间') };
    }

    return {
      payload: {
        random_disable_start_time: startTime,
        random_disable_end_time: endTime,
      },
    };
  };

  const onRandomUsedQuotaChange = (value) => {
    setRandomUsedQuota(value);
    if (value) {
      setRandomAutoDisable(true);
    }
  };

  const onGenerate = async () => {
    if (!count || count <= 0) {
      showError(t('生成数量必须大于0'));
      return;
    }
    const { payload: randomDisableTimePayload = {}, error } = randomAutoDisable
      ? buildRandomDisableTimePayload()
      : {};
    if (error) {
      showError(error);
      return;
    }
    setLoading(true);
    try {
      const res = await API.post('/api/option/channel_auto_generate', {
        count,
        models,
        groups: group ? [group] : [],
        random_used_quota: randomUsedQuota,
        random_auto_disable: randomAutoDisable,
        ...randomDisableTimePayload,
        random_response_time: randomResponseTime,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('生成渠道失败'));
        return;
      }
      setLastResult(data);
      showSuccess(t('渠道已生成'));
    } catch (error) {
      showError(
        error?.response?.data?.message || error?.message || t('生成渠道失败'),
      );
    } finally {
      setLoading(false);
    }
  };

  const onRandomDisable = async () => {
    if (!randomDisableCount || randomDisableCount <= 0) {
      showError(t('随机自动禁用数量必须大于0'));
      return;
    }
    const { payload: randomDisableTimePayload = {}, error } =
      buildRandomDisableTimePayload();
    if (error) {
      showError(error);
      return;
    }
    setDisableLoading(true);
    try {
      const res = await API.post('/api/option/channel_random_auto_disable', {
        count: randomDisableCount,
        ...randomDisableTimePayload,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('随机自动禁用失败'));
        return;
      }
      setLastDisableResult(data);
      showSuccess(
        t('已随机自动禁用 ${count} 个影子渠道').replace(
          '${count}',
          data.disabled,
        ),
      );
    } catch (error) {
      showError(
        error?.response?.data?.message ||
          error?.message ||
          t('随机自动禁用失败'),
      );
    } finally {
      setDisableLoading(false);
    }
  };

  return (
    <Spin spinning={loading || disableLoading}>
      <Form style={{ marginBottom: 15 }}>
        <Form.Section text={t('自动生成渠道')}>
          <Row gutter={16}>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='auto_channel_count'
                label={t('生成数量')}
                initValue={50}
                min={1}
                max={50000}
                step={10}
                onChange={(value) => setCount(Number(value) || 0)}
              />
            </Col>
            <Col xs={24} sm={12} md={16} lg={16} xl={16}>
              <Form.Select
                field='auto_channel_groups'
                label={t('分组')}
                placeholder={t('请选择可以使用该渠道的分组')}
                value={group}
                optionList={groupOptions}
                style={{ width: '100%' }}
                onChange={(value) => setGroup(value || '')}
              />
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Switch
                field='auto_channel_random_used_quota'
                label={t('随机已用额度')}
                checked={randomUsedQuota}
                checkedText='｜'
                uncheckedText='〇'
                onChange={onRandomUsedQuotaChange}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Switch
                field='auto_channel_random_auto_disable'
                label={t('随机自动禁用')}
                checked={randomAutoDisable}
                checkedText='｜'
                uncheckedText='〇'
                onChange={setRandomAutoDisable}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Switch
                field='auto_channel_random_response_time'
                label={t('随机响应时间')}
                checked={randomResponseTime}
                checkedText='｜'
                uncheckedText='〇'
                onChange={setRandomResponseTime}
              />
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={24} md={16} lg={16} xl={16}>
              <Form.DatePicker
                field='auto_channel_random_disable_time_range'
                label={t('随机自动禁用时间段')}
                type='dateTimeRange'
                initValue={randomDisableTimeRange}
                value={randomDisableTimeRange}
                inputReadOnly={true}
                placeholder={[t('开始时间'), t('结束时间')]}
                style={{ width: '100%' }}
                onChange={(value) => setRandomDisableTimeRange(value)}
              />
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={24} md={16} lg={16} xl={16}>
              <Form.TextArea
                field='auto_channel_models'
                label={t('模型')}
                initValue={defaultModels}
                autosize
                onChange={(value) => setModels(value)}
              />
            </Col>
          </Row>
          {lastResult ? (
            <Text type='tertiary' size='small'>
              {t(
                '最近生成：共 ${count} 条，启用 ${enabled} 条，自动禁用 ${autoDisabled} 条',
              )
                .replace('${count}', lastResult.count)
                .replace('${enabled}', lastResult.enabled)
                .replace('${autoDisabled}', lastResult.auto_disabled)}
            </Text>
          ) : null}
          <Row style={{ marginTop: 16 }}>
            <Button type='primary' onClick={onGenerate} loading={loading}>
              {t('生成渠道')}
            </Button>
          </Row>
          <Row gutter={16} style={{ marginTop: 16 }}>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='auto_channel_random_disable_count'
                label={t('随机自动禁用数量')}
                initValue={defaultRandomDisableCount}
                min={1}
                max={50000}
                step={10}
                onChange={(value) => setRandomDisableCount(Number(value) || 0)}
              />
            </Col>
          </Row>
          {lastDisableResult ? (
            <Text type='tertiary' size='small'>
              {t(
                '最近自动禁用：请求 ${requested} 个，可用 ${available} 个，已禁用 ${disabled} 个',
              )
                .replace('${requested}', lastDisableResult.requested)
                .replace('${available}', lastDisableResult.available)
                .replace('${disabled}', lastDisableResult.disabled)}
            </Text>
          ) : null}
          <Row style={{ marginTop: 16 }}>
            <Button
              type='warning'
              onClick={onRandomDisable}
              loading={disableLoading}
            >
              {t('随机自动禁用')}
            </Button>
          </Row>
        </Form.Section>
      </Form>
    </Spin>
  );
}
