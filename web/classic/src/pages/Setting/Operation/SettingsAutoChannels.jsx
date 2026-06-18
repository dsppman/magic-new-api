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

import React, { useState } from 'react';
import { Button, Col, Form, Row, Spin, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';

const { Text } = Typography;

const defaultModels = 'gemini-2.5-flash,gemini-2.5-pro';

export default function SettingsAutoChannels() {
  const { t } = useTranslation();
  const [count, setCount] = useState(50);
  const [models, setModels] = useState(defaultModels);
  const [randomUsedQuota, setRandomUsedQuota] = useState(false);
  const [loading, setLoading] = useState(false);
  const [lastResult, setLastResult] = useState(null);

  const onGenerate = async () => {
    if (!count || count <= 0) {
      showError(t('生成数量必须大于0'));
      return;
    }
    setLoading(true);
    try {
      const res = await API.post('/api/option/channel_auto_generate', {
        count,
        models,
        random_used_quota: randomUsedQuota,
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

  return (
    <Spin spinning={loading}>
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
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Switch
                field='auto_channel_random_used_quota'
                label={t('随机已用额度')}
                checkedText='｜'
                uncheckedText='〇'
                onChange={setRandomUsedQuota}
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
        </Form.Section>
      </Form>
    </Spin>
  );
}
