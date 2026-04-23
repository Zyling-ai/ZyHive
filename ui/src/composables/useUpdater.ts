// ui/src/composables/useUpdater.ts
//
// 复用在线升级的完整状态机：check → apply → poll status → waitForRestart。
// Settings 页面和顶栏版本按钮共用同一个实例，保证只有一个升级任务在跑
// + 两处 UI 的进度条永远同步。
//
// 使用方式:
//   const updater = useUpdater()
//   updater.initFromBackend()  // onMounted 时调一次，自动接管进行中的任务
//   updater.startUpgrade(targetVersion)
//
// 实例是 module-singleton（不随组件生命周期销毁），所以跨路由不丢状态。
import { ref } from 'vue'
import axios from 'axios'
import { updateApi, type UpdateStatus } from '../api'

// —— module-level singleton state ——————————————————————————————————
const currentVersion = ref<string>('')
const updateStatus = ref<UpdateStatus | null>(null)
const updateRunning = ref(false)
const restartDetected = ref(false)
const checkResult = ref<{ hasUpdate: boolean; latest: string; releaseUrl?: string } | null>(null)

let pollTimer: ReturnType<typeof setInterval> | null = null
let restartWaitTimer: ReturnType<typeof setInterval> | null = null
let initialized = false

async function fetchCurrentVersion(): Promise<string> {
  try {
    const res = await axios.get<{ version: string }>('/api/version', { timeout: 5000 })
    currentVersion.value = res.data.version
    return res.data.version
  } catch {
    return ''
  }
}

function stopPolling() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}
function stopRestartWait() {
  if (restartWaitTimer) { clearInterval(restartWaitTimer); restartWaitTimer = null }
}

function startPolling() {
  stopPolling()
  const tick = async () => {
    try {
      const res = await updateApi.status()
      updateStatus.value = res.data
      const stage = res.data.stage
      if (stage === 'done') {
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
      // 服务重启过程中会断连，吞掉继续等
    }
  }
  tick()
  pollTimer = setInterval(tick, 500)
}

function waitForRestart() {
  stopRestartWait()
  const started = Date.now()
  const tick = async () => {
    try {
      const vRes = await axios.get<{ version: string }>('/api/version', { timeout: 3000 })
      const newVer = vRes.data.version
      if (newVer && newVer !== currentVersion.value) {
        currentVersion.value = newVer
        restartDetected.value = true
        stopRestartWait()
        return
      }
    } catch {
      // 重启中 502 / 断连正常，继续等
    }
    if (Date.now() - started > 90_000) {
      restartDetected.value = true
      stopRestartWait()
    }
  }
  tick()
  restartWaitTimer = setInterval(tick, 1500)
}

// —— public API —————————————————————————————————————————————————————

/**
 * 应在有 agent-token 后（onMounted）调用一次，自动：
 *   - 读当前 /api/version
 *   - 若后端已有进行中的升级任务 → 自动接管 polling（刷新页面不丢状态）
 */
async function initFromBackend() {
  if (initialized) return
  initialized = true
  await fetchCurrentVersion()
  try {
    const res = await updateApi.status()
    const stage = res.data.stage
    updateStatus.value = res.data
    if (stage === 'downloading' || stage === 'verifying' || stage === 'applying') {
      updateRunning.value = true
      startPolling()
    } else if (stage === 'done') {
      waitForRestart()
    }
  } catch {
    // 未登录或无状态，忽略
  }
}

/** 主动检查一次更新 */
async function checkForUpdate(): Promise<{ hasUpdate: boolean; latest: string; releaseUrl?: string }> {
  const res = await updateApi.check()
  checkResult.value = res.data as any
  return res.data as any
}

/**
 * 启动一次升级。传入目标版本（通常来自 checkForUpdate.latest）。
 * 失败时抛异常让调用方 toast。成功后 polling 自动接管，UI 反映在
 * updateStatus / updateRunning / restartDetected 三个 ref 上。
 */
async function startUpgrade(targetVersion: string) {
  if (updateRunning.value) return  // 已经在跑，防重入
  updateRunning.value = true
  restartDetected.value = false
  updateStatus.value = null
  try {
    await updateApi.apply(targetVersion)
  } catch (e) {
    updateRunning.value = false
    throw e
  }
  startPolling()
}

function reloadPage() {
  window.location.reload()
}

export function useUpdater() {
  return {
    // state (readonly refs)
    currentVersion,
    updateStatus,
    updateRunning,
    restartDetected,
    checkResult,
    // actions
    initFromBackend,
    checkForUpdate,
    startUpgrade,
    reloadPage,
  }
}
