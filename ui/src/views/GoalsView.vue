<template>
  <div class="goals-page">
    <!-- ── 顶部标题栏 ─────────────────────────────────────────────── -->
    <div class="page-header">
      <h2 style="margin:0">
        <el-icon style="vertical-align:-2px;margin-right:6px"><Flag /></el-icon>目标规划
      </h2>
      <div style="display:flex;gap:8px;align-items:center">
        <el-radio-group v-model="viewMode" size="small">
          <el-radio-button value="gantt">甘特图</el-radio-button>
          <el-radio-button value="list">列表</el-radio-button>
        </el-radio-group>
        <el-button type="primary" @click="openCreate">
          <el-icon><Plus /></el-icon> 新建目标
        </el-button>
      </div>
    </div>

    <!-- ── 成员过滤栏 ─────────────────────────────────────────────── -->
    <div class="filter-bar">
      <el-text type="info" size="small" style="margin-right:4px">筛选成员：</el-text>
      <el-radio-group v-model="filterAgentId" size="small" @change="loadGoals">
        <el-radio-button value="">全部</el-radio-button>
        <el-radio-button v-for="ag in agentList" :key="ag.id" :value="ag.id">{{ ag.name }}</el-radio-button>
      </el-radio-group>
    </div>

    <!-- ══════════ 甘特图视图 ════════════════════════════════════════ -->
    <el-card v-if="viewMode === 'gantt'" shadow="hover" class="gantt-card">
      <div v-if="goalsFiltered.length === 0">
        <el-empty description="暂无目标，点击「新建目标」开始规划" />
      </div>
      <div v-else class="gantt-container">
        <!-- 月份标签行 -->
        <div class="gantt-header">
          <div class="gantt-label-col"></div>
          <div class="gantt-timeline-col">
            <div class="gantt-months">
              <div
                v-for="m in monthLabels"
                :key="m.label"
                class="gantt-month-label"
                :style="{ left: m.left }"
              >{{ m.label }}</div>
            </div>
            <!-- 月份分割线 -->
            <div class="gantt-grid-lines">
              <div
                v-for="m in monthLabels"
                :key="'line-' + m.label"
                class="gantt-grid-line"
                :style="{ left: m.left }"
              ></div>
            </div>
          </div>
        </div>
        <!-- 今日线 -->
        <div class="gantt-header" style="position:relative;height:0">
          <div class="gantt-label-col"></div>
          <div class="gantt-timeline-col" style="position:relative">
            <div
              v-if="todayLeft !== null"
              class="gantt-today-line"
              :style="{ left: todayLeft }"
            ></div>
          </div>
        </div>
        <!-- 目标行 -->
        <div
          v-for="g in goalsFiltered"
          :key="g.id"
          class="gantt-row"
        >
          <!-- 左侧标签 -->
          <div class="gantt-label-col">
            <div class="gantt-label-inner">
              <div class="gantt-agent-avatars">
                <el-tooltip
                  v-for="aid in g.agentIds.slice(0, 3)"
                  :key="aid"
                  :content="agentNameMap[aid] || aid"
                  placement="top"
                >
                  <div
                    class="gantt-avatar"
                    :style="{ background: agentColorMap[aid] || '#409eff' }"
                  >{{ (agentNameMap[aid] || aid).slice(0, 1) }}</div>
                </el-tooltip>
                <div v-if="g.agentIds.length > 3" class="gantt-avatar gantt-avatar-more">
                  +{{ g.agentIds.length - 3 }}
                </div>
              </div>
              <el-tooltip :content="g.title" placement="top" :show-after="300">
                <span class="gantt-title-text">{{ g.title }}</span>
              </el-tooltip>
            </div>
          </div>
          <!-- 时间轴 -->
          <div class="gantt-timeline-col">
            <div
              v-if="isValidBar(g)"
              class="gantt-bar"
              :style="ganttBarStyle(g)"
              @click="openEdit(g)"
              :title="g.title"
            >
              <!-- 进度覆盖层 -->
              <div class="gantt-bar-progress" :style="{ width: g.progress + '%' }"></div>
              <!-- 进度文字 -->
              <span v-if="calcBarWidth(g) > 8" class="gantt-bar-label">
                <span v-if="g.status === 'active'" class="bar-breathe-dot"></span>
                {{ g.title }}
                <span style="opacity:0.7;font-size:10px">{{ g.progress }}%</span>
              </span>
            </div>
            <!-- 里程碑钻石 -->
            <template v-if="isValidBar(g)">
              <el-tooltip
                v-for="ms in g.milestones"
                :key="ms.id"
                :content="`${ms.title}${ms.done ? ' ✓' : ''}`"
                placement="top"
              >
                <div
                  class="gantt-milestone"
                  :class="{ 'gantt-milestone-done': ms.done }"
                  :style="{ left: milestoneLeft(ms) }"
                ></div>
              </el-tooltip>
            </template>
          </div>
          <!-- 右侧状态 -->
          <div class="gantt-status-col">
            <el-tag :type="statusTagType(g.status)" size="small" :class="{ 'status-active': g.status === 'active' }">
              {{ statusLabel(g.status) }}
            </el-tag>
          </div>
        </div>
      </div>
    </el-card>

    <!-- ══════════ 列表视图 ════════════════════════════════════════ -->
    <el-card v-else shadow="hover">
      <el-table :data="goalsFiltered" stripe>
        <el-table-column prop="title" label="目标" min-width="160" />
        <el-table-column label="类型" width="80">
          <template #default="{ row }">
            <el-tag size="small" :type="row.type === 'team' ? 'warning' : 'info'">
              {{ row.type === 'team' ? '团队' : '个人' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="参与成员" min-width="140">
          <template #default="{ row }">
            <el-tag
              v-for="aid in row.agentIds"
              :key="aid"
              size="small"
              style="margin:1px"
            >{{ agentNameMap[aid] || aid }}</el-tag>
            <span v-if="!row.agentIds.length" style="color:#c0c4cc;font-size:12px">—</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)" size="small" :class="{ 'status-active': row.status === 'active' }">
              {{ statusLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="进度" width="120">
          <template #default="{ row }">
            <div style="display:flex;align-items:center;gap:6px">
              <el-progress :percentage="row.progress" :stroke-width="6" style="flex:1;min-width:60px" />
            </div>
          </template>
        </el-table-column>
        <el-table-column label="时间范围" min-width="180">
          <template #default="{ row }">
            <span style="font-size:12px;color:#606266">
              {{ formatDate(row.startAt) }} ~ {{ formatDate(row.endAt) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="deleteGoal(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="goalsFiltered.length === 0" description="暂无目标" />
    </el-card>

    <!-- ══════════ 编辑/新建抽屉 ══════════════════════════════════ -->
    <el-drawer
      v-model="drawerVisible"
      :title="editingGoal ? '编辑目标' : '新建目标'"
      size="580px"
      destroy-on-close
    >
      <el-tabs v-model="drawerTab">
        <!-- ── Tab 1: 基本信息 ── -->
        <el-tab-pane label="基本信息" name="basic">
          <el-form :model="form" label-width="100px">
            <el-form-item label="标题" required>
              <el-input v-model="form.title" placeholder="目标标题" />
            </el-form-item>
            <el-form-item label="描述">
              <el-input v-model="form.description" type="textarea" :rows="2" placeholder="（可选）" />
            </el-form-item>
            <el-form-item label="类型">
              <el-radio-group v-model="form.type">
                <el-radio-button value="personal">个人</el-radio-button>
                <el-radio-button value="team">团队</el-radio-button>
              </el-radio-group>
            </el-form-item>
            <el-form-item label="参与成员">
              <el-select v-model="form.agentIds" multiple placeholder="选择成员" style="width:100%">
                <el-option v-for="ag in agentList" :key="ag.id" :label="ag.name" :value="ag.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="状态">
              <el-select v-model="form.status" style="width:100%">
                <el-option label="草稿" value="draft" />
                <el-option label="进行中" value="active" />
                <el-option label="已完成" value="completed" />
                <el-option label="已取消" value="cancelled" />
              </el-select>
            </el-form-item>
            <el-form-item label="开始时间">
              <el-date-picker
                v-model="form.startAt"
                type="datetime"
                placeholder="选择开始时间"
                style="width:100%"
                value-format="YYYY-MM-DDTHH:mm:ssZ"
              />
            </el-form-item>
            <el-form-item label="结束时间">
              <el-date-picker
                v-model="form.endAt"
                type="datetime"
                placeholder="选择结束时间"
                style="width:100%"
                value-format="YYYY-MM-DDTHH:mm:ssZ"
              />
            </el-form-item>
            <el-form-item v-if="editingGoal" label="进度">
              <el-slider v-model="form.progress" :min="0" :max="100" show-input />
            </el-form-item>

            <!-- 里程碑 -->
            <el-form-item label="里程碑">
              <div style="width:100%">
                <div
                  v-for="(ms, idx) in form.milestones"
                  :key="idx"
                  class="milestone-row"
                >
                  <el-checkbox v-model="ms.done" />
                  <el-input
                    v-model="ms.title"
                    placeholder="里程碑标题"
                    size="small"
                    style="flex:1"
                  />
                  <el-date-picker
                    v-model="ms.dueAt"
                    type="date"
                    placeholder="截止日"
                    size="small"
                    style="width:130px"
                    value-format="YYYY-MM-DDTHH:mm:ssZ"
                  />
                  <el-button
                    type="danger"
                    size="small"
                    circle
                    @click="form.milestones.splice(idx, 1)"
                  ><el-icon><Delete /></el-icon></el-button>
                </div>
                <el-button size="small" @click="addMilestone" style="margin-top:4px">
                  <el-icon><Plus /></el-icon> 添加里程碑
                </el-button>
              </div>
            </el-form-item>
          </el-form>
          <div style="text-align:right;margin-top:16px">
            <el-button @click="drawerVisible = false">取消</el-button>
            <el-button type="primary" @click="saveGoal">{{ editingGoal ? '保存' : '创建' }}</el-button>
          </div>
        </el-tab-pane>

        <!-- ── Tab 2: 定期检查（仅编辑时显示） ── -->
        <el-tab-pane v-if="editingGoal" label="定期检查" name="checks">
          <div style="text-align:right;margin-bottom:12px">
            <el-button type="primary" size="small" @click="openAddCheckDialog">
              <el-icon><Plus /></el-icon> 添加检查
            </el-button>
          </div>
          <el-table :data="editingGoal.checks" size="small" stripe>
            <el-table-column prop="name" label="名称" min-width="120" />
            <el-table-column label="频率" min-width="140">
              <template #default="{ row }">
                <span style="font-family:monospace;font-size:12px">{{ row.schedule }}</span>
                <el-text type="info" size="small" style="margin-left:4px">({{ row.tz || 'Asia/Shanghai' }})</el-text>
              </template>
            </el-table-column>
            <el-table-column label="执行成员" width="100">
              <template #default="{ row }">
                <span>{{ agentNameMap[row.agentId] || row.agentId }}</span>
              </template>
            </el-table-column>
            <el-table-column label="启用" width="70">
              <template #default="{ row }">
                <el-switch v-model="row.enabled" size="small" @change="toggleCheck(row)" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="140">
              <template #default="{ row }">
                <el-button size="small" @click="runCheckNow(row)">立即运行</el-button>
                <el-button size="small" type="danger" @click="removeCheck(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-empty v-if="!editingGoal.checks.length" description="暂无检查计划" />
        </el-tab-pane>

        <!-- ── Tab 3: 检查记录（仅编辑时显示） ── -->
        <el-tab-pane v-if="editingGoal" label="检查记录" name="records">
          <div v-if="checkRecordsLoading" style="text-align:center;padding:20px">
            <el-icon class="is-loading"><Loading /></el-icon>
          </div>
          <el-timeline v-else-if="checkRecords.length">
            <el-timeline-item
              v-for="rec in checkRecords"
              :key="rec.id"
              :timestamp="formatDateTime(rec.runAt)"
              :type="rec.status === 'ok' ? 'success' : 'danger'"
              placement="top"
            >
              <el-card shadow="never" style="padding:0">
                <div style="display:flex;align-items:center;gap:8px;margin-bottom:6px">
                  <div
                    class="gantt-avatar"
                    :style="{ background: agentColorMap[rec.agentId] || '#409eff' }"
                  >{{ (agentNameMap[rec.agentId] || rec.agentId || '?').slice(0, 1) }}</div>
                  <span style="font-size:13px;font-weight:500">{{ agentNameMap[rec.agentId] || rec.agentId }}</span>
                  <el-tag :type="rec.status === 'ok' ? 'success' : 'danger'" size="small">{{ rec.status }}</el-tag>
                </div>
                <el-text style="font-size:12px;white-space:pre-wrap">{{ rec.output || '（无输出）' }}</el-text>
              </el-card>
            </el-timeline-item>
          </el-timeline>
          <el-empty v-else description="暂无检查记录" />
        </el-tab-pane>
      </el-tabs>
    </el-drawer>

    <!-- ══════════ 添加检查 Dialog ══════════════════════════════════ -->
    <el-dialog v-model="checkDialogVisible" title="添加定期检查" width="500px">
      <el-form :model="checkForm" label-width="100px">
        <el-form-item label="名称" required>
          <el-input v-model="checkForm.name" placeholder="如：每周进度检查" />
        </el-form-item>
        <el-form-item label="执行成员" required>
          <el-select v-model="checkForm.agentId" style="width:100%">
            <el-option v-for="ag in agentList" :key="ag.id" :label="ag.name" :value="ag.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="检查频率">
          <el-select v-model="checkFreqPreset" style="width:100%" @change="onPresetChange">
            <el-option label="每天上午9点" value="0 9 * * *" />
            <el-option label="每周一上午9点" value="0 9 * * 1" />
            <el-option label="每周五下午5点" value="0 17 * * 5" />
            <el-option label="每月1日上午9点" value="0 9 1 * *" />
            <el-option label="自定义" value="custom" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="checkFreqPreset === 'custom'" label="Cron 表达式">
          <el-input v-model="checkForm.schedule" placeholder="0 9 * * 1" />
        </el-form-item>
        <el-form-item label="时区">
          <el-select v-model="checkForm.tz" style="width:100%">
            <el-option label="Asia/Shanghai" value="Asia/Shanghai" />
            <el-option label="UTC" value="UTC" />
            <el-option label="America/New_York" value="America/New_York" />
          </el-select>
        </el-form-item>
        <el-form-item label="检查提示词">
          <el-input
            v-model="checkForm.prompt"
            type="textarea"
            :rows="4"
            placeholder="可用变量：{goal.title} {goal.progress} {goal.endAt}"
          />
          <el-text type="info" size="small" style="display:block;margin-top:4px">
            可用变量：{goal.title} {goal.progress} {goal.endAt} {goal.startAt} {goal.status}
          </el-text>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="checkForm.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="checkDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitAddCheck">添加</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, reactive, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Delete, Flag, Loading } from '@element-plus/icons-vue'
import {
  goalsApi,
  agents as agentsApi,
  type GoalInfo,
  type AgentInfo,
  type GoalCheck,
  type CheckRecord,
  type Milestone,
} from '../api'

// ── 状态 ────────────────────────────────────────────────────────────────────

const goals = ref<GoalInfo[]>([])
const agentList = ref<AgentInfo[]>([])
const filterAgentId = ref('')
const viewMode = ref<'gantt' | 'list'>('gantt')

const drawerVisible = ref(false)
const drawerTab = ref('basic')
const editingGoal = ref<GoalInfo | null>(null)

const checkDialogVisible = ref(false)
const checkRecords = ref<CheckRecord[]>([])
const checkRecordsLoading = ref(false)
const checkFreqPreset = ref('0 9 * * 1')

// ── 表单 ────────────────────────────────────────────────────────────────────

const form = reactive({
  title: '',
  description: '',
  type: 'team' as 'personal' | 'team',
  agentIds: [] as string[],
  status: 'draft' as GoalInfo['status'],
  progress: 0,
  startAt: '' as string,
  endAt: '' as string,
  milestones: [] as Array<{ id: string; title: string; dueAt: string; done: boolean; agentIds: string[] }>,
})

const checkForm = reactive({
  name: '',
  agentId: '',
  schedule: '0 9 * * 1',
  tz: 'Asia/Shanghai',
  prompt: '请检查目标「{goal.title}」的进展情况（当前进度 {goal.progress}），距截止日期 {goal.endAt} 还有一段时间，请总结近期进展并建议下一步行动。',
  enabled: true,
})

// ── 计算属性 ────────────────────────────────────────────────────────────────

const agentNameMap = computed(() => {
  const m: Record<string, string> = {}
  for (const ag of agentList.value) m[ag.id] = ag.name
  return m
})

const agentColorMap = computed(() => {
  const m: Record<string, string> = {}
  for (const ag of agentList.value) m[ag.id] = ag.avatarColor || '#409eff'
  return m
})

const goalsFiltered = computed(() => {
  if (!filterAgentId.value) return goals.value
  return goals.value.filter(g => g.agentIds.includes(filterAgentId.value))
})

// 甘特图时间范围
const ganttRange = computed(() => {
  const validGoals = goalsFiltered.value.filter(g => isValidDate(g.startAt) && isValidDate(g.endAt))
  if (!validGoals.length) {
    const now = new Date()
    return { start: new Date(now.getFullYear(), now.getMonth(), 1), end: new Date(now.getFullYear(), now.getMonth() + 3, 1) }
  }
  const starts = validGoals.map(g => new Date(g.startAt).getTime())
  const ends = validGoals.map(g => new Date(g.endAt).getTime())
  const minStart = Math.min(...starts)
  const maxEnd = Math.max(...ends)
  // Add 5% padding
  const total = maxEnd - minStart
  const pad = total * 0.05
  return {
    start: new Date(minStart - pad),
    end: new Date(maxEnd + pad),
  }
})

const monthLabels = computed(() => {
  return calcMonthLabels(ganttRange.value.start, ganttRange.value.end)
})

const todayLeft = computed(() => {
  const { start, end } = ganttRange.value
  const now = Date.now()
  if (now < start.getTime() || now > end.getTime()) return null
  const total = end.getTime() - start.getTime()
  return `${((now - start.getTime()) / total) * 100}%`
})

// ── 初始化 ──────────────────────────────────────────────────────────────────

onMounted(async () => {
  const [agRes] = await Promise.all([
    agentsApi.list().catch(() => ({ data: [] as AgentInfo[] })),
  ])
  agentList.value = agRes.data || []
  await loadGoals()
})

// 切换到检查记录 Tab 时加载记录
watch(drawerTab, async (tab) => {
  if (tab === 'records' && editingGoal.value) {
    await loadCheckRecords(editingGoal.value.id)
  }
})

// ── 数据加载 ────────────────────────────────────────────────────────────────

async function loadGoals() {
  try {
    const res = await goalsApi.list(filterAgentId.value || undefined)
    goals.value = res.data || []
  } catch {}
}

async function loadCheckRecords(goalId: string) {
  checkRecordsLoading.value = true
  try {
    const res = await goalsApi.listCheckRecords(goalId)
    checkRecords.value = (res.data || []).slice().reverse() // newest first
  } catch {
    checkRecords.value = []
  } finally {
    checkRecordsLoading.value = false
  }
}

// ── 目标 CRUD ────────────────────────────────────────────────────────────────

function openCreate() {
  editingGoal.value = null
  drawerTab.value = 'basic'
  Object.assign(form, {
    title: '',
    description: '',
    type: 'team',
    agentIds: [],
    status: 'draft',
    progress: 0,
    startAt: '',
    endAt: '',
    milestones: [],
  })
  drawerVisible.value = true
}

function openEdit(g: GoalInfo) {
  editingGoal.value = g
  drawerTab.value = 'basic'
  Object.assign(form, {
    title: g.title,
    description: g.description || '',
    type: g.type,
    agentIds: [...(g.agentIds || [])],
    status: g.status,
    progress: g.progress,
    startAt: isValidDate(g.startAt) ? g.startAt : '',
    endAt: isValidDate(g.endAt) ? g.endAt : '',
    milestones: (g.milestones || []).map(m => ({ ...m })),
  })
  drawerVisible.value = true
}

async function saveGoal() {
  if (!form.title.trim()) {
    ElMessage.warning('请填写目标标题')
    return
  }
  const payload: Partial<GoalInfo> = {
    title: form.title,
    description: form.description || undefined,
    type: form.type,
    agentIds: form.agentIds,
    status: form.status,
    progress: form.progress,
    startAt: form.startAt || undefined,
    endAt: form.endAt || undefined,
    milestones: form.milestones.map(m => ({
      ...m,
      id: m.id || 'ms-' + Math.random().toString(36).slice(2, 10),
    })) as Milestone[],
  } as any

  try {
    if (editingGoal.value) {
      await goalsApi.update(editingGoal.value.id, payload)
      ElMessage.success('保存成功')
    } else {
      await goalsApi.create(payload)
      ElMessage.success('创建成功')
    }
    drawerVisible.value = false
    await loadGoals()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '操作失败')
  }
}

async function deleteGoal(g: GoalInfo) {
  try {
    await ElMessageBox.confirm(`确定删除目标「${g.title}」？`, '删除确认', { type: 'warning' })
    await goalsApi.delete(g.id)
    ElMessage.success('已删除')
    await loadGoals()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error('删除失败')
  }
}

function addMilestone() {
  form.milestones.push({
    id: 'ms-' + Math.random().toString(36).slice(2, 10),
    title: '',
    dueAt: '',
    done: false,
    agentIds: [],
  })
}

// ── 定期检查 ────────────────────────────────────────────────────────────────

function openAddCheckDialog() {
  Object.assign(checkForm, {
    name: '',
    agentId: agentList.value[0]?.id || '',
    schedule: '0 9 * * 1',
    tz: 'Asia/Shanghai',
    prompt: '请检查目标「{goal.title}」的进展情况（当前进度 {goal.progress}），距截止日期 {goal.endAt} 还有一段时间，请总结近期进展并建议下一步行动。',
    enabled: true,
  })
  checkFreqPreset.value = '0 9 * * 1'
  checkDialogVisible.value = true
}

function onPresetChange(val: string) {
  if (val !== 'custom') {
    checkForm.schedule = val
  }
}

async function submitAddCheck() {
  if (!checkForm.name.trim()) { ElMessage.warning('请填写检查名称'); return }
  if (!checkForm.agentId) { ElMessage.warning('请选择执行成员'); return }
  if (!checkForm.schedule.trim()) { ElMessage.warning('请设置检查频率'); return }
  if (!editingGoal.value) return

  try {
    await goalsApi.addCheck(editingGoal.value.id, {
      name: checkForm.name,
      agentId: checkForm.agentId,
      schedule: checkForm.schedule,
      tz: checkForm.tz,
      prompt: checkForm.prompt,
      enabled: checkForm.enabled,
    })
    ElMessage.success('添加成功')
    checkDialogVisible.value = false
    // Refresh goal
    const res = await goalsApi.get(editingGoal.value.id)
    editingGoal.value = res.data
    await loadGoals()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '添加失败')
  }
}

async function toggleCheck(check: GoalCheck) {
  if (!editingGoal.value) return
  try {
    await goalsApi.updateCheck(editingGoal.value.id, check.id, { enabled: check.enabled } as any)
  } catch { ElMessage.error('更新失败') }
}

async function runCheckNow(check: GoalCheck) {
  if (!editingGoal.value) return
  try {
    await goalsApi.runCheck(editingGoal.value.id, check.id)
    ElMessage.success('已触发检查')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '触发失败')
  }
}

async function removeCheck(check: GoalCheck) {
  if (!editingGoal.value) return
  try {
    await ElMessageBox.confirm(`确定删除检查计划「${check.name}」？`, '删除确认', { type: 'warning' })
    await goalsApi.removeCheck(editingGoal.value.id, check.id)
    ElMessage.success('已删除')
    const res = await goalsApi.get(editingGoal.value.id)
    editingGoal.value = res.data
    await loadGoals()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error('删除失败')
  }
}

// ── 甘特图辅助函数 ──────────────────────────────────────────────────────────

function isValidDate(val: string | undefined): boolean {
  if (!val) return false
  const d = new Date(val)
  return !isNaN(d.getTime()) && d.getFullYear() > 1970
}

function isValidBar(g: GoalInfo): boolean {
  return isValidDate(g.startAt) && isValidDate(g.endAt)
}

function calcBarWidth(g: GoalInfo): number {
  const { start, end } = ganttRange.value
  const total = end.getTime() - start.getTime()
  const gStart = new Date(g.startAt).getTime()
  const gEnd = new Date(g.endAt).getTime()
  return Math.max(1, ((gEnd - gStart) / total) * 100)
}

function ganttBarStyle(g: GoalInfo): Record<string, string> {
  const { start, end } = ganttRange.value
  const total = end.getTime() - start.getTime()
  const gStart = new Date(g.startAt).getTime()
  const gEnd = new Date(g.endAt).getTime()
  const left = Math.max(0, ((gStart - start.getTime()) / total) * 100)
  const width = Math.max(1, ((gEnd - gStart) / total) * 100)

  // Determine bar color from first agent or default
  let color = '#409eff'
  const firstId = g.agentIds?.[0]
  if (firstId && agentColorMap.value[firstId]) {
    color = agentColorMap.value[firstId]
  }
  // Team goal: use gradient if multiple agents
  let background = color
  if (g.type === 'team' && g.agentIds?.length > 1) {
    const secondId = g.agentIds[1]
    const c2 = (secondId && agentColorMap.value[secondId]) ? agentColorMap.value[secondId] : '#67c23a'
    background = `linear-gradient(90deg, ${color}, ${c2})`
  }

  return {
    left: `${left}%`,
    width: `${width}%`,
    background,
  }
}

function milestoneLeft(ms: Milestone): string {
  const { start, end } = ganttRange.value
  if (!isValidDate(ms.dueAt)) return '-100%'
  const total = end.getTime() - start.getTime()
  const pos = new Date(ms.dueAt).getTime()
  const left = ((pos - start.getTime()) / total) * 100
  return `${left}%`
}

function calcMonthLabels(rangeStart: Date, rangeEnd: Date): Array<{ label: string; left: string }> {
  const months: Array<{ label: string; left: string }> = []
  const cur = new Date(rangeStart.getFullYear(), rangeStart.getMonth(), 1)
  const total = rangeEnd.getTime() - rangeStart.getTime()
  while (cur <= rangeEnd) {
    const left = ((cur.getTime() - rangeStart.getTime()) / total) * 100
    if (left >= 0 && left <= 100) {
      months.push({
        label: `${cur.getFullYear()}/${cur.getMonth() + 1}`,
        left: `${left}%`,
      })
    }
    cur.setMonth(cur.getMonth() + 1)
  }
  return months
}

// ── 工具函数 ────────────────────────────────────────────────────────────────

function statusLabel(status: string): string {
  const m: Record<string, string> = {
    draft: '草稿', active: '进行中', completed: '已完成', cancelled: '已取消',
  }
  return m[status] || status
}

function statusTagType(status: string): '' | 'info' | 'success' | 'danger' | 'warning' {
  const m: Record<string, '' | 'info' | 'success' | 'danger' | 'warning'> = {
    draft: 'info', active: '', completed: 'success', cancelled: 'danger',
  }
  return m[status] || 'info'
}

function formatDate(val: string | undefined): string {
  if (!isValidDate(val!)) return '—'
  return new Date(val!).toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' })
}

function formatDateTime(val: string | undefined): string {
  if (!val) return ''
  const d = new Date(val)
  return isNaN(d.getTime()) ? '' : d.toLocaleString('zh-CN')
}
</script>

<style scoped>
.goals-page { padding: 20px; }

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.filter-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 14px;
}

/* ── 甘特图 ───────────────────────────────────────────────────────── */
.gantt-card { overflow: hidden; }

.gantt-container {
  overflow-x: auto;
  min-width: 0;
}

.gantt-header,
.gantt-row {
  display: flex;
  align-items: stretch;
  min-height: 36px;
}

.gantt-header {
  min-height: 28px;
  border-bottom: 1px solid #ebeef5;
}

.gantt-label-col {
  width: 200px;
  min-width: 200px;
  flex-shrink: 0;
  border-right: 1px solid #ebeef5;
  padding: 4px 8px;
  display: flex;
  align-items: center;
  font-size: 13px;
}

.gantt-timeline-col {
  flex: 1;
  position: relative;
  min-height: 36px;
  overflow: hidden;
}

.gantt-status-col {
  width: 80px;
  min-width: 80px;
  flex-shrink: 0;
  border-left: 1px solid #ebeef5;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 4px;
}

.gantt-months {
  position: relative;
  height: 100%;
  width: 100%;
}

.gantt-month-label {
  position: absolute;
  top: 4px;
  font-size: 11px;
  color: #909399;
  transform: translateX(-50%);
  white-space: nowrap;
}

.gantt-grid-lines {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.gantt-grid-line {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 1px;
  background: #f0f0f0;
}

.gantt-today-line {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 2px;
  background: #f56c6c;
  z-index: 5;
  pointer-events: none;
}

/* 每行 */
.gantt-row {
  border-bottom: 1px solid #f5f5f5;
  min-height: 44px;
}
.gantt-row:hover { background: #fafafa; }

.gantt-label-inner {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  overflow: hidden;
}

.gantt-agent-avatars {
  display: flex;
  flex-shrink: 0;
}

.gantt-avatar {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  color: #fff;
  font-size: 10px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-left: -4px;
  border: 1px solid #fff;
  flex-shrink: 0;
}
.gantt-avatar:first-child { margin-left: 0; }
.gantt-avatar-more { background: #c0c4cc; font-size: 9px; }

.gantt-title-text {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #303133;
}

/* 色条 */
.gantt-bar {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  height: 24px;
  border-radius: 4px;
  cursor: pointer;
  overflow: hidden;
  display: flex;
  align-items: center;
  padding: 0 8px;
  transition: filter 0.15s;
  z-index: 2;
  box-shadow: 0 1px 4px rgba(0,0,0,0.1);
}
.gantt-bar:hover { filter: brightness(1.1); }

.gantt-bar-progress {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  background: rgba(255,255,255,0.25);
  pointer-events: none;
}

.gantt-bar-label {
  position: relative;
  z-index: 1;
  color: #fff;
  font-size: 11px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  display: flex;
  align-items: center;
  gap: 3px;
}

/* 里程碑 */
.gantt-milestone {
  position: absolute;
  top: 50%;
  transform: translate(-50%, -50%) rotate(45deg);
  width: 10px;
  height: 10px;
  background: #e6a23c;
  border: 2px solid #fff;
  z-index: 4;
  cursor: pointer;
  border-radius: 1px;
}
.gantt-milestone-done { background: #67c23a; }

/* 呼吸灯 */
:global(.status-active .el-tag__content) {
  animation: breathe-text 1.5s ease-in-out infinite;
}
@keyframes breathe-text {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}
@keyframes breathe-dot {
  0%, 100% { opacity: 1; box-shadow: 0 0 4px #409eff; }
  50% { opacity: 0.4; box-shadow: none; }
}
.bar-breathe-dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: rgba(255,255,255,0.9);
  margin-right: 3px;
  animation: breathe-dot 1.5s ease-in-out infinite;
  vertical-align: middle;
  flex-shrink: 0;
}

/* 里程碑行 */
.milestone-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
}
</style>
