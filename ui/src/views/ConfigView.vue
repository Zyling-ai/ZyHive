<template>
  <el-container class="config-page">
    <el-header class="config-header">
      <div style="display: flex; align-items: center; gap: 12px;">
        <el-button :icon="ArrowLeft" @click="$router.push('/')" circle />
        <h2>配置中心</h2>
      </div>
    </el-header>
    <el-main>
      <el-tabs type="border-card">
        <!-- Models & API Keys -->
        <el-tab-pane label="模型 & API Keys">
          <el-table :data="providers" stripe>
            <el-table-column prop="name" label="提供商" width="140" />
            <el-table-column label="API Key">
              <template #default="{ row }">
                <el-input
                  v-model="row.key"
                  :type="row.showKey ? 'text' : 'password'"
                  size="small"
                  style="max-width: 400px"
                >
                  <template #append>
                    <el-button @click="row.showKey = !row.showKey">
                      <el-icon><View v-if="!row.showKey" /><Hide v-else /></el-icon>
                    </el-button>
                  </template>
                </el-input>
              </template>
            </el-table-column>
            <el-table-column label="状态" width="120">
              <template #default="{ row }">
                <el-tag :type="row.testResult === true ? 'success' : row.testResult === false ? 'danger' : 'info'" size="small">
                  {{ row.testResult === true ? '有效' : row.testResult === false ? '无效' : '未测试' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="100">
              <template #default="{ row }">
                <el-button size="small" @click="testKey(row)" :loading="row.testing">测试</el-button>
              </template>
            </el-table-column>
          </el-table>

          <el-form style="margin-top: 24px" label-width="100px">
            <el-form-item label="默认模型">
              <el-select v-model="primaryModel">
                <el-option label="Claude Sonnet 4" value="anthropic/claude-sonnet-4-6" />
                <el-option label="Claude Opus 4" value="anthropic/claude-opus-4-0" />
                <el-option label="GPT-4o" value="openai/gpt-4o" />
                <el-option label="DeepSeek V3" value="deepseek/deepseek-chat" />
              </el-select>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <!-- Channels -->
        <el-tab-pane label="消息通道">
          <el-card header="Telegram Bot">
            <el-form label-width="140px">
              <el-form-item label="启用">
                <el-switch v-model="telegram.enabled" />
              </el-form-item>
              <el-form-item label="Bot Token">
                <el-input v-model="telegram.botToken" type="password" show-password style="max-width: 400px" />
              </el-form-item>
              <el-form-item label="默认 Agent">
                <el-input v-model="telegram.defaultAgent" placeholder="main" style="max-width: 200px" />
              </el-form-item>
              <el-form-item label="允许的发送者 ID">
                <el-input
                  v-model="telegram.allowedFromStr"
                  placeholder="逗号分隔，如 123456,789012（留空允许所有人）"
                  style="max-width: 400px"
                />
                <el-text type="info" size="small" style="display: block; margin-top: 4px">
                  Telegram 用户 ID，用于限制谁可以与 Bot 对话
                </el-text>
              </el-form-item>
              <el-form-item>
                <el-button
                  @click="testTelegram"
                  :loading="telegramTesting"
                  :disabled="!telegram.botToken || telegram.botToken.endsWith('***')"
                >
                  测试连接
                </el-button>
                <el-tag v-if="telegramTestResult !== null" :type="telegramTestResult ? 'success' : 'danger'" style="margin-left: 8px">
                  {{ telegramTestResult ? '连接成功' : '连接失败' }}
                </el-tag>
              </el-form-item>
            </el-form>
          </el-card>
        </el-tab-pane>
      </el-tabs>

      <div style="margin-top: 24px; text-align: right;">
        <el-button type="primary" size="large" @click="saveConfig" :loading="saving">
          <el-icon><Check /></el-icon> 保存配置
        </el-button>
      </div>
    </el-main>
  </el-container>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ArrowLeft } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { config as configApi } from '../api'

const primaryModel = ref('anthropic/claude-sonnet-4-6')
const saving = ref(false)
const telegram = reactive({
  enabled: false,
  botToken: '',
  defaultAgent: 'main',
  allowedFromStr: '',
})
const telegramTesting = ref(false)
const telegramTestResult = ref<boolean | null>(null)

interface ProviderRow {
  id: string; name: string; key: string;
  showKey: boolean; testing: boolean; testResult: boolean | null
}
const providers = ref<ProviderRow[]>([
  { id: 'anthropic', name: 'Anthropic', key: '', showKey: false, testing: false, testResult: null },
  { id: 'openai', name: 'OpenAI', key: '', showKey: false, testing: false, testResult: null },
  { id: 'deepseek', name: 'DeepSeek', key: '', showKey: false, testing: false, testResult: null },
])

onMounted(async () => {
  try {
    const res = await configApi.get()
    const cfg = res.data
    primaryModel.value = cfg.models?.primary || 'anthropic/claude-sonnet-4-6'
    if (cfg.models?.apiKeys) {
      for (const p of providers.value) {
        if (cfg.models.apiKeys[p.id]) p.key = cfg.models.apiKeys[p.id]
      }
    }
    if (cfg.channels?.telegram) {
      telegram.enabled = cfg.channels.telegram.enabled
      telegram.botToken = cfg.channels.telegram.botToken || ''
      telegram.defaultAgent = cfg.channels.telegram.defaultAgent || 'main'
      if (cfg.channels.telegram.allowedFrom) {
        telegram.allowedFromStr = cfg.channels.telegram.allowedFrom.join(',')
      }
    }
  } catch {}
})

async function testKey(row: ProviderRow) {
  if (!row.key || row.key.endsWith('***')) {
    ElMessage.warning('请输入完整的 API Key')
    return
  }
  row.testing = true
  row.testResult = null
  try {
    const res = await configApi.testKey(row.id, row.key)
    row.testResult = res.data.valid
    ElMessage[res.data.valid ? 'success' : 'error'](
      res.data.valid ? 'Key 有效' : `Key 无效: ${res.data.error}`
    )
  } catch {
    row.testResult = false
    ElMessage.error('测试请求失败')
  } finally {
    row.testing = false
  }
}

async function testTelegram() {
  telegramTesting.value = true
  telegramTestResult.value = null
  try {
    // Test by calling Telegram getMe API directly
    const token = telegram.botToken
    const resp = await fetch(`https://api.telegram.org/bot${token}/getMe`)
    const data = await resp.json()
    telegramTestResult.value = data.ok === true
    if (data.ok) {
      ElMessage.success(`Bot: @${data.result.username}`)
    } else {
      ElMessage.error(`连接失败: ${data.description}`)
    }
  } catch {
    telegramTestResult.value = false
    ElMessage.error('连接失败')
  } finally {
    telegramTesting.value = false
  }
}

async function saveConfig() {
  saving.value = true
  try {
    const apiKeys: Record<string, string> = {}
    for (const p of providers.value) {
      if (p.key && !p.key.endsWith('***')) apiKeys[p.id] = p.key
    }

    // Parse allowed sender IDs
    const allowedFrom: number[] = telegram.allowedFromStr
      .split(',')
      .map(s => s.trim())
      .filter(s => s.length > 0)
      .map(s => parseInt(s, 10))
      .filter(n => !isNaN(n))

    await configApi.patch({
      models: { primary: primaryModel.value, apiKeys },
      channels: {
        telegram: {
          enabled: telegram.enabled,
          botToken: telegram.botToken,
          defaultAgent: telegram.defaultAgent,
          allowedFrom,
        },
      },
    })
    ElMessage.success('配置已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.config-page { min-height: 100vh; background: #f5f7fa; }
.config-header {
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  display: flex;
  align-items: center;
  padding: 0 20px;
}
.config-header h2 { margin: 0; }
</style>
