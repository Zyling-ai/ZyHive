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
            :duration="10"
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

function startPolling() {
  if (pollTimer) clearInterval(pollTimer)
  let restartWaitStart: number | null = null

  pollTimer = setInterval(async () => {
    try {
      const res = await updateApi.status()
      updateStatus.value = res.data

      const stage = res.data.stage

      if (stage === 'done') {
        updateRunning.value = false

        if (!restartWaitStart) {
          restartWaitStart = Date.now()
        }

        // 服务会 SIGTERM 重启，等新版本上线（轮询 /api/version）
        try {
          const vRes = await axios.get<{ version: string }>('/api/version', { timeout: 3000 })
          const newVer = vRes.data.version
          if (newVer && newVer !== currentVersion.value) {
            currentVersion.value = newVer
            restartDetected.value = true
            stopPolling()
          } else if (restartWaitStart && Date.now() - restartWaitStart > 60000) {
            // 超过 60s 仍未检测到新版本
            restartDetected.value = true
            stopPolling()
          }
        } catch {
          // 服务正在重启中，忽略连接错误
        }

      } else if (stage === 'failed' || stage === 'rolledback') {
        updateRunning.value = false
        stopPolling()
      }
    } catch {
      // 服务重启过程中会断连，继续等待
    }
  }, 1500)
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

onBeforeUnmount(() => stopPolling())

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
.settings-page { padding: 24px; }

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
