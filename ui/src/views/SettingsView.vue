<template>
  <div class="settings-page">
    <h2 style="margin: 0 0 20px"><el-icon style="vertical-align:-2px;margin-right:6px"><Setting /></el-icon>系统设置</h2>

    <!-- 基本设置 -->
    <el-card shadow="hover" style="max-width: 600px; margin-bottom: 20px">
      <template #header><span style="font-weight:600">基本设置</span></template>
      <el-form label-width="120px">
        <el-form-item label="面板端口">
          <el-input-number v-model="port" :min="1024" :max="65535" />
        </el-form-item>
        <el-form-item label="访问令牌">
          <el-input v-model="token" type="password" show-password placeholder="留空保持不变" style="max-width: 300px" />
        </el-form-item>
        <el-form-item label="语言">
          <el-select v-model="lang" style="width: 200px">
            <el-option label="中文" value="zh" />
            <el-option label="English" value="en" disabled />
          </el-select>
        </el-form-item>
        <el-form-item label="主题">
          <el-select v-model="theme" style="width: 200px">
            <el-option label="浅色" value="light" />
            <el-option label="深色" value="dark" disabled />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="save" :loading="saving">保存设置</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 版本与更新 -->
    <el-card shadow="hover" style="max-width: 600px">
      <template #header>
        <span style="font-weight:600">版本与更新</span>
      </template>

      <div class="version-section">
        <!-- 版本信息行 -->
        <div class="version-row">
          <div class="version-info">
            <div class="version-label">当前版本</div>
            <div class="version-value">
              <el-tag type="info" size="small">{{ currentVersion || '…' }}</el-tag>
            </div>
          </div>
          <div v-if="checkResult" class="version-info">
            <div class="version-label">最新版本</div>
            <div class="version-value">
              <el-tag :type="checkResult.hasUpdate ? 'success' : 'info'" size="small">
                {{ checkResult.latest }}
              </el-tag>
            </div>
          </div>
        </div>

        <!-- 检查结果提示 -->
        <div v-if="checkResult && !checkResult.hasUpdate && !updateRunning" class="check-tip ok">
          <el-icon><CircleCheckFilled /></el-icon>
          当前已是最新版本，无需升级
        </div>
        <div v-if="checkResult && checkResult.hasUpdate && !updateRunning" class="check-tip new">
          <el-icon><InfoFilled /></el-icon>
          发现新版本 {{ checkResult.latest }}，可立即升级
          <a :href="checkResult.releaseUrl" target="_blank" style="margin-left:8px;font-size:12px">查看更新日志 →</a>
        </div>

        <!-- 升级进度 -->
        <div v-if="updateRunning || updateStatus" class="update-progress">
          <div class="progress-header">
            <span class="stage-label">{{ stageLabel }}</span>
            <span class="progress-pct">{{ updateStatus?.progress ?? 0 }}%</span>
          </div>
          <el-progress
            :percentage="updateStatus?.progress ?? 0"
            :status="progressStatus"
            :striped="updateRunning"
            :striped-flow="updateRunning"
            :duration="1"
          />
          <div class="progress-msg">{{ updateStatus?.message }}</div>

          <!-- 升级完成后提示刷新 -->
          <div v-if="updateStatus?.stage === 'done' && restartDetected" class="restart-tip">
            <el-icon><CircleCheckFilled /></el-icon>
            服务已重启，新版本 {{ updateStatus.newVersion }} 运行中
            <el-button type="primary" size="small" style="margin-left:12px" @click="reloadPage">刷新页面</el-button>
          </div>
          <div v-else-if="updateStatus?.stage === 'done'" class="restart-tip waiting">
            <el-icon class="spin"><Loading /></el-icon>
            等待服务重启…
          </div>
          <div v-if="updateStatus?.stage === 'failed'" class="fail-tip">
            <el-icon><CircleCloseFilled /></el-icon>
            {{ updateStatus.message }}
          </div>
          <div v-if="updateStatus?.stage === 'rolledback'" class="rollback-tip">
            <el-icon><WarningFilled /></el-icon>
            升级失败，已自动回滚到旧版本
          </div>
        </div>

        <!-- 操作按钮 -->
        <div class="update-actions">
          <el-button
            :loading="checking"
            :disabled="updateRunning"
            @click="checkUpdate"
          >
            <el-icon style="margin-right:4px"><Refresh /></el-icon>
            检查更新
          </el-button>
          <el-button
            v-if="checkResult?.hasUpdate"
            type="primary"
            :loading="updateRunning"
            :disabled="updateRunning"
            @click="applyUpdate"
          >
            <el-icon style="margin-right:4px"><Upload /></el-icon>
            升级到 {{ checkResult.latest }}
          </el-button>
        </div>

        <div class="data-safe-tip">
          <el-icon><Lock /></el-icon>
          升级仅替换程序文件，成员数据、对话记录、配置文件全部保留
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Setting, Refresh, Upload, CircleCheckFilled, CircleCloseFilled,
  InfoFilled, WarningFilled, Loading, Lock
} from '@element-plus/icons-vue'
import { config as configApi, updateApi, type UpdateCheckResult, type UpdateStatus } from '../api'
import axios from 'axios'

// ── 基本设置 ─────────────────────────────────────────────────────────────────
const port = ref(8080)
const token = ref('')
const lang = ref('zh')
const theme = ref('light')
const saving = ref(false)

onMounted(async () => {
  try {
    const res = await configApi.get()
    port.value = res.data.gateway?.port || 8080
  } catch {}
  // 获取当前版本
  fetchCurrentVersion()

  // 若页面加载时后端已有进行中的升级任务, 自动挂上 polling,
  // 避免 "刷新页面后进度条不跑" 的错觉
  try {
    const res = await updateApi.status()
    const stage = res.data.stage
    updateStatus.value = res.data
    if (stage === 'downloading' || stage === 'verifying' || stage === 'applying') {
      updateRunning.value = true
      startPolling()
    } else if (stage === 'done') {
      // 已完成但页面刚打开: 直接进入等重启分支
      waitForRestart()
    }
  } catch {
    // auth 失败或首次无状态, 忽略
  }
})

async function save() {
  saving.value = true
  try {
    const patch: any = { gateway: { port: port.value } }
    if (token.value) patch.auth = { mode: 'token', token: token.value }
    await configApi.patch(patch)
    ElMessage.success('设置已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

// ── 版本与更新 ────────────────────────────────────────────────────────────────
const currentVersion = ref('')
const checking = ref(false)
const checkResult = ref<UpdateCheckResult | null>(null)
const updateStatus = ref<UpdateStatus | null>(null)
const updateRunning = ref(false)
const restartDetected = ref(false)

let pollTimer: ReturnType<typeof setInterval> | null = null

async function fetchCurrentVersion() {
  try {
    const res = await axios.get<{ version: string }>('/api/version')
    currentVersion.value = res.data.version
  } catch {}
}

async function checkUpdate() {
  checking.value = true
  checkResult.value = null
  try {
    const res = await updateApi.check()
    checkResult.value = res.data
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '检查更新失败，请检查网络')
  } finally {
    checking.value = false
  }
}

async function applyUpdate() {
  if (!checkResult.value?.hasUpdate) return
  try {
    await ElMessageBox.confirm(
      `确认将 ZyHive 从 ${currentVersion.value} 升级到 ${checkResult.value.latest}？\n\n升级过程中服务将短暂重启（约 10-30 秒），成员数据和配置文件不受影响。`,
      '确认升级',
      { confirmButtonText: '立即升级', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return  // 用户取消
  }

  updateRunning.value = true
  restartDetected.value = false
  updateStatus.value = null

  try {
    await updateApi.apply(checkResult.value.latest)
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '启动升级失败')
    updateRunning.value = false
    return
  }

  // 开始轮询状态
  startPolling()
}

// 轮询 /api/update/status 取进度。
// 修复:
//   1. 立即首次拉取 (原先 setInterval 首次触发要等 1.5s, 用户看不到进度动起来)
//   2. 进行中间隔 500ms (原 1500ms 太慢, 后端 verify/apply 阶段往往 1-2s 跑完 -> UI 错过中间态)
//   3. stage='done' 时进度已经到 100 -> 主 polling 立即停止, 改由独立的 restart-wait 循环
//      去等新版本上线, 不再依赖主 polling 继续活跃
let restartWaitTimer: ReturnType<typeof setInterval> | null = null

function startPolling() {
  stopPolling()

  const tick = async () => {
    try {
      const res = await updateApi.status()
      updateStatus.value = res.data

      const stage = res.data.stage

      if (stage === 'done') {
        // 主 polling 任务结束: 进度条已到 100, 立即停止拉取主状态,
        // 开启独立的 restart-wait 循环等待服务重启 (新版本上线)
        stopPolling()
        updateRunning.value = false
        waitForRestart()
        return
      }

      if (stage === 'failed' || stage === 'rolledback') {
        stopPolling()
        updateRunning.value = false
      }
    } catch {
      // 服务重启过程中会断连, 吞掉继续等
    }
  }

  // 修复 1: 立即首次拉取
  tick()
  // 修复 2: 500ms 快速轮询
  pollTimer = setInterval(tick, 500)
}

// 等待服务重启后的新版本号出现, 独立于主 polling,
// 失败/超时不影响已完成的升级状态.
function waitForRestart() {
  const started = Date.now()
  if (restartWaitTimer) clearInterval(restartWaitTimer)
  const tick = async () => {
    try {
      const vRes = await axios.get<{ version: string }>('/api/version', { timeout: 3000 })
      const newVer = vRes.data.version
      if (newVer && newVer !== currentVersion.value) {
        currentVersion.value = newVer
        restartDetected.value = true
        if (restartWaitTimer) { clearInterval(restartWaitTimer); restartWaitTimer = null }
        return
      }
    } catch {
      // 重启中 502 / 断连 正常, 继续等
    }
    // 90s 兜底: 无论如何停止等待避免前端死转
    if (Date.now() - started > 90000) {
      restartDetected.value = true
      if (restartWaitTimer) { clearInterval(restartWaitTimer); restartWaitTimer = null }
    }
  }
  tick()
  restartWaitTimer = setInterval(tick, 1500)
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

function reloadPage() {
  window.location.reload()
}

onBeforeUnmount(() => {
  stopPolling()
  if (restartWaitTimer) { clearInterval(restartWaitTimer); restartWaitTimer = null }
})

// ── computed ──────────────────────────────────────────────────────────────────
const stageLabel = computed(() => {
  const map: Record<string, string> = {
    idle: '空闲',
    downloading: '下载中',
    verifying: '验证中',
    applying: '替换文件',
    done: '升级完成',
    failed: '升级失败',
    rolledback: '已回滚',
  }
  return map[updateStatus.value?.stage ?? 'idle'] ?? ''
})

const progressStatus = computed(() => {
  const s = updateStatus.value?.stage
  if (s === 'done') return 'success'
  if (s === 'failed') return 'exception'
  if (s === 'rolledback') return 'warning'
  return undefined
})
</script>

<style scoped>
.settings-page { /* 外层 padding 由 .app-main 统一提供 */ }

.version-section { display: flex; flex-direction: column; gap: 16px; }

.version-row { display: flex; gap: 32px; }
.version-info { display: flex; flex-direction: column; gap: 4px; }
.version-label { font-size: 12px; color: #909399; }
.version-value { display: flex; align-items: center; gap: 6px; }

.check-tip {
  display: flex; align-items: center; gap: 6px;
  font-size: 13px; padding: 8px 12px; border-radius: 6px;
}
.check-tip.ok { background: #f0f9eb; color: #67c23a; }
.check-tip.new { background: #ecf5ff; color: #409eff; }

.update-progress { display: flex; flex-direction: column; gap: 8px; }
.progress-header { display: flex; justify-content: space-between; font-size: 13px; color: #606266; }
.progress-msg { font-size: 12px; color: #909399; min-height: 18px; }

.restart-tip {
  display: flex; align-items: center; gap: 6px;
  font-size: 13px; color: #67c23a; padding: 6px 0;
}
.restart-tip.waiting { color: #909399; }
.fail-tip { display: flex; align-items: center; gap: 6px; font-size: 13px; color: #f56c6c; }
.rollback-tip { display: flex; align-items: center; gap: 6px; font-size: 13px; color: #e6a23c; }

.update-actions { display: flex; gap: 10px; }

.data-safe-tip {
  display: flex; align-items: center; gap: 6px;
  font-size: 12px; color: #909399;
  padding: 8px 12px; background: #f5f7fa; border-radius: 6px;
}

.spin { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
</style>
