<template>
  <div class="feishu-wizard">
    <!-- Stepper -->
    <el-steps :active="step" finish-status="success" align-center style="margin-bottom: 18px">
      <el-step title="创建应用" />
      <el-step title="填凭据" />
      <el-step title="自动修复" />
      <el-step title="启动测试" />
    </el-steps>

    <!-- ── Step 1: 创建应用引导 ── -->
    <div v-if="step === 0">
      <el-card shadow="never" class="step-card">
        <h3 class="step-title">🪶 创建一个企业自建应用</h3>
        <p class="step-hint">在飞书开放平台创建应用，按下面的清单配置好凭据 + 权限 + 事件订阅。</p>

        <el-button type="primary" size="default" @click="openCreatePage">
          <el-icon><Link /></el-icon> 打开飞书开放平台 (新窗口)
        </el-button>

        <el-divider />

        <div class="config-list">
          <div class="config-item">
            <div class="config-label">应用名称建议</div>
            <div class="config-row">
              <code class="config-value">{{ defaultAppName }}</code>
              <el-button size="small" plain @click="copy(defaultAppName)">📋 复制</el-button>
            </div>
          </div>

          <div class="config-item">
            <div class="config-label">权限点（共 5 个，逐条添加）</div>
            <div class="scope-grid">
              <div v-for="s in requiredScopes" :key="s" class="scope-chip">
                <code>{{ s }}</code>
                <el-button size="small" link @click="copy(s)">复制</el-button>
              </div>
              <el-button size="small" plain @click="copy(requiredScopes.join('\n'))">📋 复制全部</el-button>
            </div>
          </div>

          <div class="config-item">
            <div class="config-label">事件订阅</div>
            <div class="config-row">
              <code class="config-value">im.message.receive_v1</code>
              <el-button size="small" plain @click="copy('im.message.receive_v1')">📋 复制</el-button>
            </div>
          </div>

          <div class="config-item">
            <div class="config-label">事件接收方式</div>
            <code class="config-value">使用长连接接收事件（WebSocket）</code>
          </div>
        </div>

        <el-divider />
        <div class="step-actions">
          <el-button type="primary" @click="step = 1">下一步 →</el-button>
        </div>
      </el-card>
    </div>

    <!-- ── Step 2: 粘贴凭据 + 一键验证 ── -->
    <div v-else-if="step === 1">
      <el-card shadow="never" class="step-card">
        <h3 class="step-title">🔐 粘贴应用凭据</h3>
        <p class="step-hint">从应用「凭证与基础信息」页复制 App ID 和 App Secret。</p>

        <el-form label-width="100px" label-position="left">
          <el-form-item label="App ID" required>
            <el-input v-model="appId" placeholder="cli_xxxxxxxxxxxxxxxx" autofocus />
          </el-form-item>
          <el-form-item label="App Secret" required>
            <el-input v-model="appSecret" type="password" show-password placeholder="从飞书后台复制" />
          </el-form-item>
        </el-form>

        <el-button type="primary" size="default" :loading="probing" @click="runProbe" style="width:100%">
          <el-icon v-if="!probing"><Aim /></el-icon>
          {{ probing ? probeStage : '🔍 一键绑定（验证凭据 + 检查配置）' }}
        </el-button>

        <!-- 验证通过的 bot 卡片 -->
        <div v-if="probe && probe.bot.name" class="bot-card">
          <div class="bot-avatar">
            <img v-if="probe.bot.avatarUrl" :src="probe.bot.avatarUrl" alt="" />
            <span v-else>{{ probe.bot.name.charAt(0) }}</span>
          </div>
          <div class="bot-info">
            <div class="bot-name">{{ probe.bot.name }}</div>
            <div class="bot-id">{{ probe.bot.openId || appId }}</div>
            <div class="bot-status">
              <el-tag v-if="probe.published" size="small" type="success">✅ 已上线</el-tag>
              <el-tag v-else size="small" type="warning">⚠️ 未上线</el-tag>
            </div>
          </div>
        </div>

        <!-- 错误展示 -->
        <el-alert v-if="probe && probe.error === 'auth_failed'"
          type="error" :closable="false" style="margin-top: 14px"
          title="❌ App Secret 错了"
          description="请检查飞书后台「应用凭证」是否填对，或者 App Secret 是否被刷新过。" />
        <el-alert v-else-if="probe && probe.error === 'app_not_published'"
          type="error" :closable="false" style="margin-top: 14px"
          title="❌ 应用还没上线" >
          <template #default>
            应用必须先「申请上线」并通过审核才能使用。
            <el-button size="small" type="primary" link @click="openAppPage('version')">前往版本管理 →</el-button>
          </template>
        </el-alert>
        <el-alert v-else-if="probe && probe.error === 'network'"
          type="warning" :closable="false" style="margin-top: 14px"
          title="网络问题"
          description="无法连接飞书服务器，请检查网络后重试。" />

        <el-divider />
        <div class="step-actions">
          <el-button @click="step = 0">← 上一步</el-button>
          <el-button v-if="probe && probe.bot.name" type="primary"
            @click="step = needsFixing ? 2 : 3">下一步 →</el-button>
        </div>
      </el-card>
    </div>

    <!-- ── Step 3: 自动修复（缺权限 / 缺事件订阅） ── -->
    <div v-else-if="step === 2">
      <el-card shadow="never" class="step-card">
        <h3 class="step-title">🛠 还差一点点</h3>
        <p class="step-hint">下面的项需要你回飞书后台勾选 / 启用。我们用深链接直接带你到对应页面。</p>

        <!-- 缺权限 -->
        <div v-if="probe && probe.permissions.missing.length > 0" class="fix-block">
          <div class="fix-header">
            <span class="fix-icon">🔓</span>
            <span class="fix-title">还缺 {{ probe.permissions.missing.length }} 个权限点</span>
          </div>
          <div class="fix-detail">
            <code v-for="s in probe.permissions.missing" :key="s" class="missing-scope">{{ s }}</code>
            <el-button size="small" plain @click="copy(probe.permissions.missing.join('\n'))" style="margin-left:8px">
              📋 复制全部
            </el-button>
          </div>
          <div class="fix-actions">
            <el-button type="primary" size="default" @click="openAppPage('auth')">
              <el-icon><Link /></el-icon> 打开「{{ probe.bot.name }}」的权限管理页
            </el-button>
            <el-text type="info" size="small" style="margin-left:10px">
              在「权限管理」搜索框逐个粘贴权限名 → 勾选 → 保存
            </el-text>
          </div>
        </div>

        <!-- 缺事件订阅 -->
        <div v-if="probe && (!probe.events.subscribed || !probe.events.longConnEnabled)" class="fix-block">
          <div class="fix-header">
            <span class="fix-icon">📡</span>
            <span class="fix-title">事件订阅未配好</span>
          </div>
          <div class="fix-detail">
            <ul style="margin:0;padding-left:20px">
              <li v-if="!probe.events.subscribed">❌ 未订阅事件 <code>im.message.receive_v1</code></li>
              <li v-if="!probe.events.longConnEnabled">❌ 未启用「使用长连接接收事件」</li>
            </ul>
          </div>
          <div class="fix-actions">
            <el-button type="primary" size="default" @click="openAppPage('event')">
              <el-icon><Link /></el-icon> 打开事件订阅页
            </el-button>
            <el-text type="info" size="small" style="margin-left:10px">
              事件订阅 → 添加 im.message.receive_v1 + 切换到「长连接」
            </el-text>
          </div>
        </div>

        <!-- 重新检测按钮 -->
        <el-divider />
        <div style="text-align:center">
          <el-button type="success" :loading="probing" @click="runProbe" size="default">
            <el-icon v-if="!probing"><Refresh /></el-icon>
            {{ probing ? probeStage : '⏱ 我已完成，重新检测' }}
          </el-button>
          <div style="margin-top:6px;font-size:12px;color:#94a3b8">
            在飞书后台保存修改后回来点这个，几秒钟就能确认。
          </div>
        </div>

        <el-divider />
        <div class="step-actions">
          <el-button @click="step = 1">← 上一步</el-button>
          <el-button v-if="probe && !needsFixing" type="primary" @click="step = 3">下一步 →</el-button>
        </div>
      </el-card>
    </div>

    <!-- ── Step 4: 启动测试 + 完成 ── -->
    <div v-else-if="step === 3">
      <el-card shadow="never" class="step-card">
        <h3 class="step-title">🚀 启动并测试</h3>
        <p class="step-hint">现在试着建立 WebSocket 长连接，确认机器人能正常上线。</p>

        <el-button v-if="!testResult" type="primary" size="default" :loading="testing" @click="runTest" style="width:100%">
          <el-icon v-if="!testing"><VideoPlay /></el-icon>
          {{ testing ? '建立连接 + 监听心跳...' : '🚀 启动并测试 (约 10 秒)' }}
        </el-button>

        <!-- 测试结果 -->
        <div v-if="testResult" class="test-result">
          <div v-if="testResult.ok" class="test-pass">
            <div class="test-icon">🎉</div>
            <div class="test-title">全部就绪</div>
            <div class="test-details">
              <div>✅ 连接已建立（{{ testResult.latencyMs }} ms）</div>
              <div>✅ Bot 已上线：{{ testResult.botName }}</div>
              <div>✅ 可以在飞书里给「{{ testResult.botName }}」发消息了</div>
            </div>
            <el-button type="primary" size="default" @click="probe && emit('done', { appId, appSecret, probeResult: probe })" style="margin-top:16px">
              完成绑定 ✓
            </el-button>
          </div>
          <div v-else class="test-fail">
            <div class="test-icon">❌</div>
            <div class="test-title">连接失败</div>
            <div class="test-details">{{ testResult.error }}</div>
            <el-button size="default" @click="testResult = null" style="margin-top:12px">重试</el-button>
          </div>
        </div>

        <!-- 加群列表预览 -->
        <div v-if="testResult?.ok && probe?.joinedChats && probe.joinedChats.length" style="margin-top:24px">
          <el-divider>当前已加入的群 ({{ probe.joinedChats.length }})</el-divider>
          <div class="chat-preview-list">
            <div v-for="ch in probe.joinedChats" :key="ch.chatId" class="chat-preview-item">
              <span class="chat-mode">{{ kindEmoji(ch.kind) }}</span>
              <span class="chat-name">{{ ch.name || ch.chatId }}</span>
            </div>
          </div>
        </div>

        <el-divider />
        <div class="step-actions">
          <el-button @click="step = 2">← 返回修复页</el-button>
        </div>
      </el-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { Link, Aim, Refresh, VideoPlay } from '@element-plus/icons-vue'
import { networkApi, type FeishuProbeResult } from '../api'

const props = defineProps<{
  initialAppId?: string
  defaultAppName?: string
  cloud?: 'cn' | 'lark'  // 'cn' = open.feishu.cn, 'lark' = open.larksuite.com
}>()

const emit = defineEmits<{
  (e: 'done', payload: { appId: string; appSecret: string; probeResult: FeishuProbeResult }): void
  (e: 'cancel'): void
}>()

const step = ref(0)
const appId = ref(props.initialAppId || '')
const appSecret = ref('')
const probing = ref(false)
const probeStage = ref('验证凭据中...')
const probe = ref<FeishuProbeResult | null>(null)

const testing = ref(false)
const testResult = ref<{ ok: boolean; botName?: string; latencyMs: number; error?: string } | null>(null)

const requiredScopes = [
  'im:message',
  'im:message:send_as_bot',
  'im:resource',
  'contact:user.base:readonly',
  'im:chat:readonly',
]

const defaultAppName = computed(() => props.defaultAppName || 'ZyHive AI 机器人')

const cloudDomain = computed(() => props.cloud === 'lark' ? 'open.larksuite.com' : 'open.feishu.cn')

const needsFixing = computed(() => {
  if (!probe.value) return false
  return probe.value.permissions.missing.length > 0
    || !probe.value.events.subscribed
    || !probe.value.events.longConnEnabled
})

function openCreatePage() {
  window.open(`https://${cloudDomain.value}/app`, '_blank')
}

function openAppPage(section: 'auth' | 'event' | 'version') {
  if (!appId.value) {
    ElMessage.warning('请先填写 App ID')
    return
  }
  window.open(`https://${cloudDomain.value}/app/${encodeURIComponent(appId.value)}/${section}`, '_blank')
}

function copy(text: string) {
  navigator.clipboard?.writeText(text).then(() => {
    ElMessage.success('已复制')
  }).catch(() => {
    ElMessage.error('复制失败，请手动选中')
  })
}

async function runProbe() {
  if (!appId.value.trim() || !appSecret.value.trim()) {
    ElMessage.warning('请填 App ID 和 App Secret')
    return
  }
  probing.value = true
  probeStage.value = '验证凭据中...'
  try {
    // Stage hints (fire-and-forget — real backend does it serially in one call)
    setTimeout(() => { if (probing.value) probeStage.value = '检查权限...' }, 1500)
    setTimeout(() => { if (probing.value) probeStage.value = '检查事件订阅...' }, 3000)
    setTimeout(() => { if (probing.value) probeStage.value = '列出已加入的群...' }, 4500)

    const res = await networkApi.feishuProbe(appId.value.trim(), appSecret.value.trim())
    probe.value = res.data
    if (probe.value.ok) {
      ElMessage.success('全部检查通过')
    } else if (probe.value.bot.name) {
      ElMessage.warning('凭据有效，但还有 ' + (
        probe.value.permissions.missing.length + (probe.value.events.subscribed ? 0 : 1)
      ) + ' 项要修')
    }
  } catch (e: any) {
    ElMessage.error('请求失败：' + (e?.message || ''))
    const fallback: FeishuProbeResult = {
      ok: false,
      error: 'network',
      bot: {},
      published: false,
      permissions: { granted: [], missing: [] },
      events: { subscribed: false, longConnEnabled: false },
    }
    probe.value = fallback
  } finally {
    probing.value = false
  }
}

async function runTest() {
  testing.value = true
  testResult.value = null
  try {
    const res = await networkApi.feishuTestConnect(appId.value.trim(), appSecret.value.trim())
    testResult.value = res.data
  } catch (e: any) {
    testResult.value = { ok: false, latencyMs: 0, error: e?.message || '未知错误' }
  } finally {
    testing.value = false
  }
}

function kindEmoji(kind: string): string {
  switch (kind) {
    case 'group': return '👥'
    case 'p2p': return '👤'
    case 'topic': return '💬'
    default: return '🗨'
  }
}
</script>

<style scoped>
.feishu-wizard { padding: 4px 8px 14px; }
.step-card { border: none; box-shadow: none; }
.step-title { margin: 0 0 6px 0; font-size: 16px; color: #1e293b; font-weight: 600; }
.step-hint { color: #64748b; font-size: 13px; margin: 0 0 14px 0; line-height: 1.6; }

.config-list { display: flex; flex-direction: column; gap: 12px; }
.config-item { background: #f8fafc; border-radius: 6px; padding: 10px 12px; }
.config-label { font-size: 12px; color: #64748b; font-weight: 600; margin-bottom: 4px; }
.config-row { display: flex; align-items: center; gap: 8px; }
.config-value { background: #fff; padding: 4px 8px; border-radius: 4px; border: 1px solid #e2e8f0; font-family: ui-monospace, monospace; font-size: 13px; flex: 1; }

.scope-grid { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
.scope-chip {
  display: inline-flex; align-items: center; gap: 4px;
  background: #fff; border: 1px solid #e2e8f0;
  border-radius: 4px; padding: 3px 8px;
  font-size: 12px;
}
.scope-chip code { font-size: 11px; }

.step-actions { display: flex; justify-content: space-between; gap: 8px; }

.bot-card {
  display: flex; gap: 14px; align-items: center;
  margin-top: 16px;
  padding: 12px 14px;
  background: #f0f9ff;
  border: 1px solid #bae6fd;
  border-radius: 8px;
}
.bot-avatar {
  width: 52px; height: 52px; border-radius: 50%;
  background: #6366f1; color: #fff;
  display: flex; align-items: center; justify-content: center;
  font-size: 22px; font-weight: 600;
  overflow: hidden; flex-shrink: 0;
}
.bot-avatar img { width: 100%; height: 100%; object-fit: cover; }
.bot-info { flex: 1; }
.bot-name { font-size: 15px; font-weight: 600; color: #0c4a6e; }
.bot-id { font-size: 11px; color: #64748b; font-family: ui-monospace, monospace; margin: 2px 0 4px 0; }

.fix-block {
  background: #fff7ec; border: 1px solid #fde68a;
  border-radius: 8px; padding: 14px;
  margin-bottom: 14px;
}
.fix-header { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
.fix-icon { font-size: 22px; }
.fix-title { font-weight: 600; color: #92400e; font-size: 14px; }
.fix-detail { margin-bottom: 12px; line-height: 1.8; }
.missing-scope {
  display: inline-block;
  background: #fff; border: 1px solid #fde68a;
  color: #92400e; padding: 2px 8px; margin: 0 4px 4px 0;
  border-radius: 4px; font-size: 12px;
}
.fix-actions { display: flex; align-items: center; }

.test-result { margin-top: 18px; padding: 18px; text-align: center; border-radius: 8px; }
.test-pass { background: #f0fdf4; border: 1px solid #bbf7d0; }
.test-fail { background: #fef2f2; border: 1px solid #fecaca; }
.test-icon { font-size: 36px; margin-bottom: 6px; }
.test-title { font-size: 16px; font-weight: 600; margin-bottom: 10px; }
.test-pass .test-title { color: #166534; }
.test-fail .test-title { color: #991b1b; }
.test-details { font-size: 13px; line-height: 1.8; color: #374151; text-align: left; max-width: 340px; margin: 0 auto; }

.chat-preview-list { max-height: 160px; overflow: auto; }
.chat-preview-item {
  display: flex; gap: 8px; padding: 6px 10px;
  border-bottom: 1px solid #f1f5f9;
  font-size: 13px;
}
.chat-mode { font-size: 14px; }
</style>
