<template>
  <div class="cron-page">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px">
      <h2 style="margin: 0"><el-icon style="vertical-align:-2px;margin-right:6px"><Timer /></el-icon>定时任务</h2>
      <div style="display:flex;gap:8px;">
        <el-button @click="openMorningRoutine">
          <span style="margin-right:4px">🌅</span> 晨间例行
        </el-button>
        <el-button type="primary" @click="openCreate">
          <el-icon><Plus /></el-icon> 新建任务
        </el-button>
      </div>
    </div>

    <!-- Filter bar -->
    <div style="margin-bottom: 12px; display: flex; gap: 10px; align-items: center; flex-wrap: wrap">
      <el-text type="info" size="small" style="margin-right: 2px">筛选成员：</el-text>
      <el-radio-group v-model="filterAgentId" size="small" @change="loadJobs">
        <el-radio-button value="">全部</el-radio-button>
        <el-radio-button value="__global__">全局任务</el-radio-button>
        <el-radio-button v-for="ag in agentList" :key="ag.id" :value="ag.id">{{ ag.name }}</el-radio-button>
      </el-radio-group>
    </div>

    <el-card shadow="hover">
      <el-table :data="jobs" stripe>
        <el-table-column prop="name" label="名称" min-width="150" />
        <el-table-column label="所属成员" width="120">
          <template #default="{ row }">
            <el-tag v-if="row.agentId" size="small" type="primary" style="cursor:pointer" @click="goToAgent(row)">
              {{ agentNameMap[row.agentId] || row.agentId }}
            </el-tag>
            <el-tag v-else size="small" type="info">全局</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="备注" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.remark" style="font-size: 13px; color: #606266;">{{ row.remark }}</span>
            <span v-else style="color: #c0c4cc; font-size: 12px;">—</span>
          </template>
        </el-table-column>
        <el-table-column label="调度" min-width="160">
          <template #default="{ row }">
            <span style="font-size: 12px; font-family: monospace;">{{ row.schedule?.expr }}</span>
            <el-text type="info" size="small" style="margin-left: 4px;">({{ row.schedule?.tz }})</el-text>
          </template>
        </el-table-column>
        <el-table-column label="最近运行" width="170">
          <template #default="{ row }">
            <template v-if="row.state?.lastRunAtMs">
              <el-tag :type="row.state?.lastStatus === 'ok' ? 'success' : 'danger'" size="small">
                {{ row.state?.lastStatus }}
              </el-tag>
              <el-text type="info" size="small" style="margin-left: 4px">
                {{ formatTime(row.state?.lastRunAtMs) }}
              </el-text>
            </template>
            <el-text v-else type="info" size="small">未运行</el-text>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="70">
          <template #default="{ row }">
            <el-switch
              v-model="row.enabled"
              @change="toggleCron(row)"
              size="small"
              :disabled="isMemoryJob(row)"
            />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="240">
          <template #default="{ row }">
            <template v-if="isMemoryJob(row)">
              <el-tag type="info" size="small" style="margin-right: 6px;">记忆管理</el-tag>
              <el-button size="small" @click="goToAgent(row)">查看</el-button>
            </template>
            <template v-else>
              <el-button size="small" @click="runNow(row)">立即运行</el-button>
              <el-button size="small" type="info" @click="openLogs(row)">日志</el-button>
              <el-button size="small" type="danger" @click="deleteCron(row)">删除</el-button>
            </template>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="jobs.length === 0" description="暂无定时任务" />
    </el-card>

    <!-- Run Logs Dialog -->
    <el-dialog v-model="showLogs" :title="`执行日志 — ${currentJob?.name}`" width="780px">
      <div style="margin-bottom: 10px; display: flex; align-items: center; gap: 8px;">
        <el-text type="info" size="small">最近 50 条执行记录</el-text>
        <el-button size="small" @click="openLogs(currentJob!)" :loading="loadingLogs">刷新</el-button>
      </div>
      <el-table :data="runLogs" stripe size="small" v-loading="loadingLogs" max-height="460">
        <el-table-column label="运行时间" width="170">
          <template #default="{ row }">{{ formatTime(row.startedAt) }}</template>
        </el-table-column>
        <el-table-column label="耗时" width="80">
          <template #default="{ row }">
            <el-text size="small">{{ ((row.endedAt - row.startedAt) / 1000).toFixed(1) }}s</el-text>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="75">
          <template #default="{ row }">
            <el-tag :type="row.status === 'ok' ? 'success' : 'danger'" size="small">
              {{ row.status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="推送" width="60">
          <template #default="{ row }">
            <el-tag v-if="row.announced" type="success" size="small" effect="plain">已推</el-tag>
            <el-text v-else type="info" size="small">—</el-text>
          </template>
        </el-table-column>
        <el-table-column label="输出 / 错误" min-width="200">
          <template #default="{ row }">
            <div v-if="row.status === 'error'" style="color: #f56c6c; font-size: 12px; white-space: pre-wrap; max-height: 80px; overflow: auto;">
              {{ row.error }}
            </div>
            <div v-else style="font-size: 12px; color: #606266; white-space: pre-wrap; max-height: 80px; overflow: auto;">
              {{ row.output || '—' }}
            </div>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!loadingLogs && runLogs.length === 0" description="暂无执行记录" />
      <template #footer>
        <el-button @click="showLogs = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- Create Dialog -->
    <el-dialog v-model="showCreate" title="新建定时任务" width="520px">
      <el-form :model="form" label-width="110px">
        <el-form-item label="所属成员">
          <el-select v-model="form.agentId" placeholder="不选则为全局任务" clearable style="width: 100%">
            <el-option v-for="ag in agentList" :key="ag.id" :label="ag.name" :value="ag.id" />
          </el-select>
          <el-text type="info" size="small" style="display:block;margin-top:4px">
            <el-icon style="vertical-align:-2px;margin-right:4px"><InfoFilled /></el-icon>不选则为全局任务；选择成员后该任务只在成员的「定时任务」Tab 显示
          </el-text>
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="任务名称" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" placeholder="可选，说明这个任务的用途" />
        </el-form-item>
        <el-form-item label="Cron 表达式">
          <el-input v-model="form.expr" placeholder="0 9 * * *" />
          <el-text type="info" size="small" style="margin-top: 4px; display: block;">
            格式：秒(可选) 分 时 日 月 周。例：0 0 9 * * * = 每天09:00
          </el-text>
        </el-form-item>
        <el-form-item label="时区">
          <el-select v-model="form.tz" style="width: 100%">
            <el-option label="Asia/Shanghai" value="Asia/Shanghai" />
            <el-option label="UTC" value="UTC" />
            <el-option label="America/New_York" value="America/New_York" />
          </el-select>
        </el-form-item>
        <el-form-item label="消息内容">
          <el-input v-model="form.message" type="textarea" :rows="3" placeholder="发送给 Agent 的消息内容" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="createCron">创建</el-button>
      </template>
    </el-dialog>

    <!-- Morning Routine Dialog -->
    <el-dialog v-model="showMorning" title="🌅 晨间例行（一键模板）" width="560px">
      <div style="font-size: 13px; color: #64748b; line-height: 1.7; margin-bottom: 16px;">
        给选中的 AI 成员每天早晨自动"醒一次"：<br>
        整理昨日对话要点、检查愿望清单进展、给你留张便条。
        <br>
        <span style="color:#94a3b8">若当天没有值得汇报的，AI 会自动静默（回 <code>NO_ALERT</code> 不打扰你）。</span>
      </div>
      <el-form label-width="90px" size="default">
        <el-form-item label="所属成员" required>
          <el-select v-model="morning.agentId" placeholder="选择 AI 成员" style="width:100%">
            <el-option v-for="ag in agentList" :key="ag.id" :label="ag.name" :value="ag.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="时间">
          <el-time-picker
            v-model="morning.timeStr"
            format="HH:mm"
            value-format="HH:mm"
            placeholder="HH:mm"
            :clearable="false"
            style="width: 160px"
          />
          <el-text type="info" size="small" style="margin-left:10px">每天执行一次</el-text>
        </el-form-item>
        <el-form-item label="时区">
          <el-select v-model="morning.tz" style="width: 220px">
            <el-option label="Asia/Shanghai（UTC+8）" value="Asia/Shanghai" />
            <el-option label="UTC" value="UTC" />
            <el-option label="America/New_York" value="America/New_York" />
            <el-option label="Europe/London" value="Europe/London" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showMorning = false">取消</el-button>
        <el-button type="primary" @click="createMorningRoutine">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { cron as cronApi, agents as agentsApi, type CronJob, type AgentInfo } from '../api'

const router = useRouter()
const jobs = ref<CronJob[]>([])
const agentList = ref<AgentInfo[]>([])
const filterAgentId = ref('')
const showCreate = ref(false)
const showMorning = ref(false)

// Logs
const showLogs = ref(false)
const currentJob = ref<CronJob | null>(null)
const runLogs = ref<any[]>([])
const loadingLogs = ref(false)

const agentNameMap = computed(() => {
  const m: Record<string, string> = {}
  for (const ag of agentList.value) m[ag.id] = ag.name
  return m
})

const form = reactive({
  agentId: '',
  name: '',
  remark: '',
  expr: '0 0 9 * * *',
  tz: 'Asia/Shanghai',
  message: '',
  enabled: true,
})

// 晨间例行表单
const morning = reactive({
  agentId: '',
  timeStr: '08:00',
  tz: 'Asia/Shanghai',
})

onMounted(async () => {
  const res = await agentsApi.list().catch(() => ({ data: [] as AgentInfo[] }))
  agentList.value = res.data || []
  loadJobs()
})

async function loadJobs() {
  try {
    const res = await cronApi.list(filterAgentId.value || undefined)
    jobs.value = res.data || []
  } catch (e: any) {
    ElMessage.error('加载定时任务失败: ' + (e?.message || '未知错误'))
  }
}

function formatTime(ms: number) {
  return ms ? new Date(ms).toLocaleString('zh-CN') : ''
}

function isMemoryJob(row: CronJob): boolean {
  return row.payload?.message === '__MEMORY_CONSOLIDATE__'
}

function goToAgent(row: CronJob) {
  if (row.agentId) {
    router.push({ path: `/agents/${row.agentId}`, query: { tab: 'cron' } })
  }
}

function openCreate() {
  form.agentId = ''
  form.name = ''
  form.remark = ''
  form.expr = '0 0 9 * * *'
  form.tz = 'Asia/Shanghai'
  form.message = ''
  form.enabled = true
  showCreate.value = true
}

// 打开晨间例行对话框：默认填选中的 filter 或第一个成员
function openMorningRoutine() {
  // 若筛选栏已选中某成员，默认填入；否则第一个
  if (filterAgentId.value && filterAgentId.value !== '__global__') {
    morning.agentId = filterAgentId.value
  } else {
    morning.agentId = agentList.value[0]?.id || ''
  }
  morning.timeStr = '08:00'
  morning.tz = 'Asia/Shanghai'
  showMorning.value = true
}

// 晨间例行 prompt 模板（末尾 NO_ALERT 指令对接 cron engine SilentToken 机制）
const MORNING_PROMPT = `晨间例行（每日自动唤醒）：

1. 扫描昨天的对话历史（conversations/INDEX.md），把值得长期记住的要点整理到 memory/core/ 或 memory/daily/ 相应文件。
2. 检查 WISHLIST.md 与 GOALS，看有没有进展或新的机会点。
3. 若发现世界状态相关（时事、价格、版本等）需要更新，可用 web_search / web_fetch 查一下。
4. 若有值得主动告诉用户的事（进展、风险、提醒），追加到 memory/daily/notes-to-user.md 并在本次回复中简要汇报。
5. 若今天没有任何值得汇报的事，请只回一个单词：NO_ALERT（系统会静默处理，不打扰用户）。

保持简洁、克制、有用。不要为了汇报而汇报。`

async function createMorningRoutine() {
  if (!morning.agentId) {
    ElMessage.warning('请选择 AI 成员')
    return
  }
  // 解析 HH:mm → 标准 5 字段 cron：分 时 日 月 周
  const m = /^(\d{1,2}):(\d{1,2})$/.exec(morning.timeStr || '')
  if (!m || !m[1] || !m[2]) {
    ElMessage.error('时间格式错误，应为 HH:mm')
    return
  }
  const HH = parseInt(m[1], 10)
  const MM = parseInt(m[2], 10)
  const expr = `${MM} ${HH} * * *`
  const agentName = agentNameMap.value[morning.agentId] || morning.agentId
  try {
    await cronApi.create({
      name: '晨间例行',
      remark: `每天 ${morning.timeStr} 自动唤醒 ${agentName}：整理记忆 · 检查愿望 · 给你留便条`,
      agentId: morning.agentId,
      enabled: true,
      schedule: { kind: 'cron', expr, tz: morning.tz },
      payload: { kind: 'agentTurn', message: MORNING_PROMPT },
      delivery: { mode: 'announce' },
    } as any)
    ElMessage.success(`已为 ${agentName} 创建晨间例行（每天 ${morning.timeStr}）`)
    showMorning.value = false
    loadJobs()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '创建失败')
  }
}

// #16 fix: validate cron expression before submit
function isValidCronExpr(expr: string): boolean {
  if (!expr || !expr.trim()) return false
  const parts = expr.trim().split(/\s+/)
  if (parts.length < 5 || parts.length > 6) return false
  // basic field range check (loose)
  return parts.every(p => /^[\d,\-\*\/]+$/.test(p) || p === '?')
}

async function createCron() {
  if (!form.name?.trim()) {
    ElMessage.warning('请填写任务名称')
    return
  }
  if (!isValidCronExpr(form.expr)) {
    ElMessage.error('Cron 表达式格式错误，格式为：分 时 日 月 周（如 0 9 * * 1）')
    return
  }
  if (!form.message?.trim()) {
    ElMessage.warning('请填写任务内容')
    return
  }
  try {
    await cronApi.create({
      name: form.name,
      remark: form.remark || undefined,
      agentId: form.agentId || undefined,
      enabled: form.enabled,
      schedule: { kind: 'cron', expr: form.expr.trim(), tz: form.tz },
      payload: { kind: 'agentTurn', message: form.message },
      delivery: { mode: 'announce' },
    } as any)
    ElMessage.success('创建成功')
    showCreate.value = false
    loadJobs()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '创建失败')
  }
}

async function toggleCron(job: CronJob) {
  try { await cronApi.update(job.id, job as any) } catch { ElMessage.error('更新失败') }
}

async function runNow(job: CronJob) {
  try {
    await cronApi.run(job.id)
    ElMessage.success('已触发')
    setTimeout(loadJobs, 2000)
  } catch { ElMessage.error('触发失败') }
}

async function deleteCron(job: CronJob) {
  try {
    await cronApi.delete(job.id)
    ElMessage.success('已删除')
    loadJobs()
  } catch { ElMessage.error('删除失败') }
}

async function openLogs(job: CronJob) {
  currentJob.value = job
  showLogs.value = true
  loadingLogs.value = true
  try {
    const res = await cronApi.runs(job.id)
    runLogs.value = (res.data || []).slice().reverse() // newest first
  } catch {
    ElMessage.error('获取日志失败')
    runLogs.value = []
  } finally {
    loadingLogs.value = false
  }
}
</script>

<style scoped>
.cron-page {
  /* 外层 padding 由 .app-main 统一提供 */
}
</style>
