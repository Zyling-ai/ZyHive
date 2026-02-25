<template>
  <div class="goals-studio">

    <!-- ── 左：目标列表 ── -->
    <div class="gs-sidebar" :style="{ width: sideW + 'px' }">
      <div class="sidebar-top">
        <span class="sidebar-title">目标规划</span>
        <div class="sidebar-acts">
          <el-button size="small" circle @click="loadGoals">
            <el-icon><Refresh /></el-icon>
          </el-button>
          <el-button size="small" type="primary" circle @click="openCreate">
            <el-icon><Plus /></el-icon>
          </el-button>
        </div>
      </div>

      <!-- 过滤 -->
      <div class="gs-filter">
        <el-select v-model="filterStatus" placeholder="所有状态" clearable size="small" class="filter-sel">
          <el-option label="草稿" value="draft" />
          <el-option label="进行中" value="active" />
          <el-option label="已完成" value="completed" />
          <el-option label="已取消" value="cancelled" />
        </el-select>
        <el-select v-model="filterAgentId" placeholder="所有成员" clearable size="small" class="filter-sel">
          <el-option v-for="ag in agentList" :key="ag.id" :label="ag.name" :value="ag.id" />
        </el-select>
      </div>

      <!-- 目标列表 -->
      <div class="goal-list">
        <!-- 新建占位条目 -->
        <div v-if="creating" class="goal-item goal-item-new active">
          <div class="gi-top">
            <span class="gi-title gi-title-new">{{ form.title.trim() || '新目标…' }}</span>
            <el-tag type="info" size="small" effect="plain">草稿</el-tag>
          </div>
          <div class="gi-new-hint">尚未保存</div>
        </div>

        <div v-if="filteredGoals.length === 0 && !creating" class="list-empty">暂无目标</div>
        <div
          v-for="g in filteredGoals" :key="g.id"
          :class="['goal-item', { active: selectedGoal?.id === g.id }]"
          @click="selectGoal(g)"
        >
          <!-- 顶行 -->
          <div class="gi-top">
            <span class="gi-title">{{ g.title }}</span>
            <el-tag :type="statusTagType(g.status)" size="small" effect="plain">
              {{ statusLabel(g.status) }}
            </el-tag>
          </div>
          <!-- 进度条 -->
          <div class="gi-progress-wrap">
            <div class="gi-progress-bar" :style="{ width: g.progress + '%', background: progressColor(g) }" />
            <span class="gi-progress-num">{{ g.progress }}%</span>
          </div>
          <!-- 底行：成员头像 + 日期 -->
          <div class="gi-bottom">
            <div class="gi-avatars">
              <div
                v-for="id in (g.agentIds || []).slice(0, 3)" :key="id"
                class="gi-avatar" :style="{ background: agentColorMap[id] || '#6366f1' }"
              >{{ (agentNameMap[id] || id)[0] }}</div>
              <span v-if="(g.agentIds || []).length > 3" class="gi-avatar-more">+{{ g.agentIds.length - 3 }}</span>
            </div>
            <span class="gi-dates">
              {{ formatDate(g.startAt) }} — {{ formatDate(g.endAt) }}
            </span>
          </div>
        </div>
      </div>
    </div>

    <!-- 拖拽手柄 1 -->
    <div class="gs-handle" @mousedown="startResize($event, 'side')" :class="{ dragging: dragging === 'side' }">
      <div class="gs-handle-bar" />
    </div>

    <!-- ── 中：编辑/详情 ── -->
    <div class="gs-editor">

      <!-- 空态：显示甘特图总览 -->
      <div v-if="!selectedGoal && !creating" class="editor-gantt-overview">
        <div class="overview-header">
          <span class="overview-title">
            <el-icon style="vertical-align:-2px;margin-right:4px"><Flag /></el-icon>
            目标总览 · 甘特图
          </span>
          <el-button type="primary" size="small" @click="openCreate">
            <el-icon><Plus /></el-icon> 新建目标
          </el-button>
        </div>

        <div v-if="filteredGoals.length === 0" class="gantt-empty">
          <el-icon size="48" color="#c0c4cc"><Flag /></el-icon>
          <p>暂无目标，点击新建开始规划</p>
        </div>

        <div v-else class="gantt-wrap" @wheel.prevent="handleGanttWheel">
          <!-- 时间刻度行（双层：年份 + 月/周） -->
          <div class="gantt-header">
            <div class="gantt-label-col">
              <span class="gantt-scale-hint">{{ { quarter:'季', month:'月', biweek:'双周', week:'周' }[ganttScale] }} ↕滚轮</span>
            </div>
            <div class="gantt-timeline-col" ref="ganttTimelineRef">
              <!-- 年份标记层：只在 labelTicks 里有 yearMark 的位置显示 -->
              <div class="gantt-years">
                <div v-for="t in labelTicks.filter(t => t.yearMark)" :key="'yr-' + t.left" class="gantt-year-label" :style="{ left: t.left }">
                  {{ t.yearMark }}
                </div>
              </div>
              <!-- 月/周刻度层：稀疏化后的标签 -->
              <div class="gantt-months">
                <div v-for="t in labelTicks" :key="t.label + t.left" class="gantt-month-label" :style="{ left: t.left }">
                  {{ t.label }}
                </div>
              </div>
            </div>
          </div>
          <!-- 目标行区域（含网格线+今日线覆盖） -->
          <div class="gantt-body">
            <!-- 网格+今日线覆盖层（绝对定位，不遮点击） -->
            <div class="gantt-overlay" aria-hidden="true">
              <div class="gantt-label-col" />
              <div class="gantt-timeline-col" style="position:relative">
                <div v-for="m in monthLabels" :key="'gv-' + m.label" class="gantt-grid-line" :style="{ left: m.left }" />
                <div v-if="todayLeft !== null" class="gantt-today-line" :style="{ left: todayLeft }" />
              </div>
            </div>
            <!-- 目标行 -->
            <div v-for="g in filteredGoals" :key="g.id" class="gantt-row" @click="selectGoal(g)">
              <div class="gantt-label-col">
                <div class="gantt-label-inner">
                  <div class="gantt-agent-avatars">
                    <div
                      v-for="id in (g.agentIds || []).slice(0, 2)" :key="id"
                      class="gantt-avatar" :style="{ background: agentColorMap[id] || '#409eff' }"
                    >{{ (agentNameMap[id] || id).slice(0, 1) }}</div>
                  </div>
                  <span class="gantt-goal-name">{{ g.title }}</span>
                  <span class="gantt-pct" :class="'s-' + g.status">{{ g.progress }}%</span>
                </div>
              </div>
              <div class="gantt-timeline-col">
                <template v-if="isValidBar(g)">
                  <div class="gantt-bar" :style="ganttBarStyle(g)">
                    <div class="gantt-bar-progress" :style="{ width: g.progress + '%' }" />
                    <span v-if="calcBarWidth(g) > 8" class="gantt-bar-label">{{ g.title }}</span>
                  </div>
                  <template v-for="ms in (g.milestones || [])" :key="ms.id">
                    <div
                      v-if="isValidDate(ms.dueAt)"
                      class="gantt-milestone"
                      :class="{ done: ms.done }"
                      :style="{ left: milestoneLeft(ms) }"
                      :title="ms.title"
                    />
                  </template>
                </template>
                <div v-else class="gantt-no-date">未设置时间范围</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 新建表单 -->
      <template v-else-if="creating">
        <div class="editor-toolbar">
          <div class="editor-breadcrumb">
            <el-icon style="color:#909399"><Flag /></el-icon>
            <span class="crumb-sep">新建</span>
            <span class="crumb-name">目标</span>
          </div>
          <div class="toolbar-acts">
            <el-button size="small" @click="creating = false">取消</el-button>
            <el-button size="small" type="primary" :loading="saving" @click="saveGoal">
              <el-icon><DocumentChecked /></el-icon> 创建
            </el-button>
          </div>
        </div>
        <div class="editor-form">
          <el-form :model="form" label-width="90px" size="small" class="goal-inner-form">
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
              <el-date-picker v-model="form.startAt" type="datetime" placeholder="选择开始时间"
                style="width:100%" value-format="YYYY-MM-DDTHH:mm:ssZ" />
            </el-form-item>
            <el-form-item label="结束时间">
              <el-date-picker v-model="form.endAt" type="datetime" placeholder="选择结束时间"
                style="width:100%" value-format="YYYY-MM-DDTHH:mm:ssZ" />
            </el-form-item>
            <el-form-item v-if="selectedGoal" label="进度">
              <el-slider v-model="form.progress" :min="0" :max="100" show-input />
            </el-form-item>
            <el-form-item label="里程碑">
              <div style="width:100%">
                <div v-for="(ms, idx) in form.milestones" :key="idx" class="milestone-row">
                  <el-checkbox v-model="ms.done" />
                  <el-input v-model="ms.title" placeholder="里程碑标题" size="small" style="flex:1" />
                  <el-date-picker v-model="ms.dueAt" type="date" placeholder="截止日"
                    size="small" style="width:130px" value-format="YYYY-MM-DDTHH:mm:ssZ" />
                  <el-button type="danger" size="small" circle @click="form.milestones.splice(idx, 1)">
                    <el-icon><Delete /></el-icon>
                  </el-button>
                </div>
                <el-button size="small" @click="addMilestone" style="margin-top:4px">
                  <el-icon><Plus /></el-icon> 添加里程碑
                </el-button>
              </div>
            </el-form-item>
          </el-form>
        </div>
      </template>

      <!-- 目标详情/编辑 -->
      <template v-else-if="selectedGoal">
        <div class="editor-toolbar">
          <div class="editor-breadcrumb">
            <el-icon style="color:#909399"><Flag /></el-icon>
            <span class="crumb-sep">目标</span>
            <span class="crumb-name">{{ selectedGoal.title }}</span>
          </div>
          <div class="toolbar-acts">
            <el-button size="small" type="primary" :loading="saving" @click="saveGoal">
              <el-icon><DocumentChecked /></el-icon> 保存
            </el-button>
            <el-popconfirm :title="`确认删除「${selectedGoal.title}」？`" @confirm="deleteGoal">
              <template #reference>
                <el-button size="small" type="danger" plain><el-icon><Delete /></el-icon></el-button>
              </template>
            </el-popconfirm>
          </div>
        </div>

        <!-- 三 Tab -->
        <el-tabs v-model="editorTab" class="editor-tabs">

          <!-- Tab 1: 基本信息 -->
          <el-tab-pane label="基本信息" name="basic">
            <div class="editor-form">
          <el-form :model="form" label-width="90px" size="small" class="goal-inner-form">
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
              <el-date-picker v-model="form.startAt" type="datetime" placeholder="选择开始时间"
                style="width:100%" value-format="YYYY-MM-DDTHH:mm:ssZ" />
            </el-form-item>
            <el-form-item label="结束时间">
              <el-date-picker v-model="form.endAt" type="datetime" placeholder="选择结束时间"
                style="width:100%" value-format="YYYY-MM-DDTHH:mm:ssZ" />
            </el-form-item>
            <el-form-item v-if="selectedGoal" label="进度">
              <el-slider v-model="form.progress" :min="0" :max="100" show-input />
            </el-form-item>
            <el-form-item label="里程碑">
              <div style="width:100%">
                <div v-for="(ms, idx) in form.milestones" :key="idx" class="milestone-row">
                  <el-checkbox v-model="ms.done" />
                  <el-input v-model="ms.title" placeholder="里程碑标题" size="small" style="flex:1" />
                  <el-date-picker v-model="ms.dueAt" type="date" placeholder="截止日"
                    size="small" style="width:130px" value-format="YYYY-MM-DDTHH:mm:ssZ" />
                  <el-button type="danger" size="small" circle @click="form.milestones.splice(idx, 1)">
                    <el-icon><Delete /></el-icon>
                  </el-button>
                </div>
                <el-button size="small" @click="addMilestone" style="margin-top:4px">
                  <el-icon><Plus /></el-icon> 添加里程碑
                </el-button>
              </div>
            </el-form-item>
          </el-form>
            </div>
          </el-tab-pane>

          <!-- Tab 2: 定期检查 -->
          <el-tab-pane label="定期检查" name="checks">
            <div class="tab-panel">
              <div class="tab-panel-head">
                <el-button type="primary" size="small" @click="openAddCheckDialog">
                  <el-icon><Plus /></el-icon> 添加检查
                </el-button>
              </div>
              <el-table :data="selectedGoal.checks" size="small" stripe class="checks-table">
                <el-table-column prop="name" label="名称" min-width="120" />
                <el-table-column label="频率" min-width="140">
                  <template #default="{ row }">
                    <code class="code-cell">{{ row.schedule }}</code>
                    <el-text type="info" size="small" style="margin-left:4px">{{ row.tz || 'Asia/Shanghai' }}</el-text>
                  </template>
                </el-table-column>
                <el-table-column label="成员" width="90">
                  <template #default="{ row }">{{ agentNameMap[row.agentId] || row.agentId }}</template>
                </el-table-column>
                <el-table-column label="启用" width="60">
                  <template #default="{ row }">
                    <el-switch v-model="row.enabled" size="small" @change="toggleCheck(row)" />
                  </template>
                </el-table-column>
                <el-table-column label="操作" width="130">
                  <template #default="{ row }">
                    <el-button size="small" link @click="runCheckNow(row)">立即运行</el-button>
                    <el-button size="small" link type="danger" @click="removeCheck(row)">删除</el-button>
                  </template>
                </el-table-column>
              </el-table>
              <el-empty v-if="!selectedGoal.checks?.length" description="暂无检查计划" :image-size="60" />
            </div>
          </el-tab-pane>

          <!-- Tab 3: 检查记录 -->
          <el-tab-pane label="检查记录" name="records">
            <div class="tab-panel">
              <div v-if="checkRecordsLoading" style="text-align:center;padding:20px">
                <el-icon class="is-loading" size="24"><Loading /></el-icon>
              </div>
              <el-timeline v-else-if="checkRecords.length" class="records-timeline">
                <el-timeline-item
                  v-for="rec in checkRecords"
                  :key="rec.id"
                  :timestamp="formatDateTime(rec.runAt)"
                  :type="rec.status === 'ok' ? 'success' : 'danger'"
                  placement="top"
                >
                  <div class="record-card">
                    <div class="record-header">
                      <div class="rec-avatar" :style="{ background: agentColorMap[rec.agentId] || '#409eff' }">
                        {{ (agentNameMap[rec.agentId] || '?')[0] }}
                      </div>
                      <span class="rec-name">{{ agentNameMap[rec.agentId] || rec.agentId }}</span>
                      <el-tag :type="rec.status === 'ok' ? 'success' : 'danger'" size="small">{{ rec.status }}</el-tag>
                    </div>
                    <div class="rec-output">{{ rec.output || '（无输出）' }}</div>
                  </div>
                </el-timeline-item>
              </el-timeline>
              <el-empty v-else description="暂无检查记录" :image-size="60" />
            </div>
          </el-tab-pane>

        </el-tabs>
      </template>

    </div>

    <!-- 拖拽手柄 2 -->
    <div class="gs-handle" @mousedown="startResize($event, 'chat')" :class="{ dragging: dragging === 'chat' }">
      <div class="gs-handle-bar" />
    </div>

    <!-- ── 右：AI 对话 ── -->
    <div class="gs-chat" :style="{ width: chatW + 'px' }">
      <div class="chat-panel-head">
        <el-icon><ChatLineRound /></el-icon>
        <span>AI 目标助手</span>
        <el-select
          v-model="selectedChatAgentId"
          size="small"
          style="margin-left:auto;width:110px"
          placeholder="选择成员"
        >
          <el-option v-for="ag in agentList" :key="ag.id" :label="ag.name" :value="ag.id" />
        </el-select>
      </div>
      <div class="chat-wrap">
        <AiChat
          v-if="selectedChatAgentId"
          :key="goalChatSessionId"
          :agent-id="selectedChatAgentId"
          :session-id="goalChatSessionId"
          :context="goalChatContext"
          :welcome-message="selectedGoal
            ? `正在查看目标「${selectedGoal.title}」，我可以帮你修改目标信息、添加里程碑或设置定期检查。`
            : '你好！我可以帮你创建目标，说一下需求即可，我会自动填写表单。'"
          :examples="selectedGoal
            ? ['帮我把进度更新到 60%', '添加3个里程碑', '每周一检查这个目标']
            : ['帮我创建一个团队目标：Q2用户增长，3月1日到6月30日', '个人目标：学习 Go 语言，本月完成']"
          height="100%"
          @response="onAiResponse"
        />
      </div>
    </div>

    <!-- 添加检查 Dialog -->
    <el-dialog v-model="checkDialogVisible" title="添加定期检查" width="480px">
      <el-form :model="checkForm" label-width="90px" size="small">
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
        <el-form-item v-if="checkFreqPreset === 'custom'" label="Cron">
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
          <el-input v-model="checkForm.prompt" type="textarea" :rows="3"
            placeholder="可用变量：{goal.title} {goal.progress} {goal.endAt}" />
          <div style="font-size:11px;color:#94a3b8;margin-top:3px">
            变量：{goal.title} {goal.progress} {goal.endAt} {goal.startAt} {goal.status}
          </div>
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
import { Plus, Refresh, Delete, Flag, Loading, ChatLineRound, DocumentChecked } from '@element-plus/icons-vue'
import {
  goalsApi, agents as agentsApi,
  type GoalInfo, type AgentInfo, type GoalCheck, type CheckRecord, type Milestone,
} from '../api'
import AiChat from '../components/AiChat.vue'

// ── 布局状态 ─────────────────────────────────────────────────────────────
const sideW    = ref(260)
const chatW    = ref(360)
const dragging = ref<'side' | 'chat' | ''>('')
let startX = 0, startW2 = 0

function startResize(e: MouseEvent, target: 'side' | 'chat') {
  dragging.value = target
  startX = e.clientX
  startW2 = target === 'side' ? sideW.value : chatW.value
  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
  e.preventDefault()
}
function onMouseMove(e: MouseEvent) {
  const d = e.clientX - startX
  if (dragging.value === 'side') sideW.value = Math.max(200, Math.min(400, startW2 + d))
  else chatW.value = Math.max(280, Math.min(560, startW2 - d))
}
function onMouseUp() {
  dragging.value = ''
  document.removeEventListener('mousemove', onMouseMove)
  document.removeEventListener('mouseup', onMouseUp)
}

// ── 数据状态 ─────────────────────────────────────────────────────────────
const goals       = ref<GoalInfo[]>([])
const agentList   = ref<AgentInfo[]>([])
const filterStatus  = ref('')
const filterAgentId = ref('')

const selectedGoal = ref<GoalInfo | null>(null)
const creating     = ref(false)
const saving       = ref(false)
const editorTab    = ref('basic')

const checkDialogVisible  = ref(false)
const checkRecords        = ref<CheckRecord[]>([])
const checkRecordsLoading = ref(false)
const checkFreqPreset     = ref('0 9 * * 1')

const selectedChatAgentId = ref('')

// ── 表单 ─────────────────────────────────────────────────────────────────
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

// ── 计算属性 ─────────────────────────────────────────────────────────────
const agentNameMap = computed(() => {
  const m: Record<string, string> = {}
  agentList.value.forEach(a => { m[a.id] = a.name })
  return m
})
const agentColorMap = computed(() => {
  const m: Record<string, string> = {}
  agentList.value.forEach(a => { m[a.id] = a.avatarColor || '#409eff' })
  return m
})

const filteredGoals = computed(() => {
  let list = [...goals.value]
  if (filterStatus.value)  list = list.filter(g => g.status === filterStatus.value)
  if (filterAgentId.value) list = list.filter(g => (g.agentIds || []).includes(filterAgentId.value))
  return list
})

// 甘特图范围
const ganttRange = computed(() => {
  const valid = filteredGoals.value.filter(g => isValidDate(g.startAt) && isValidDate(g.endAt))
  if (!valid.length) {
    const now = new Date()
    return { start: new Date(now.getFullYear(), now.getMonth(), 1), end: new Date(now.getFullYear(), now.getMonth() + 3, 1) }
  }
  const starts = valid.map(g => new Date(g.startAt).getTime())
  const ends   = valid.map(g => new Date(g.endAt).getTime())
  const minS = Math.min(...starts), maxE = Math.max(...ends)
  // 固定 14 天留白，避免因百分比 padding 导致范围延伸到下一年
  const pad = 14 * 24 * 60 * 60 * 1000
  return { start: new Date(minS - pad), end: new Date(maxE + pad) }
})
// 甘特图时间颗粒度：季度 → 月 → 双周 → 周
const SCALES = ['quarter', 'month', 'biweek', 'week'] as const
type GanttScale = typeof SCALES[number]
const ganttScale = ref<GanttScale>('month')

interface GridTick { label: string; yearMark?: string; left: string }

// 所有刻度（用于网格线）
const gridTicks = computed<GridTick[]>(() =>
  calcGridTicks(ganttRange.value.start, ganttRange.value.end, ganttScale.value))

// 追踪时间轴容器宽度，用于动态过滤标签密度
const ganttTimelineW = ref(700)
const ganttTimelineRef = ref<HTMLElement | null>(null)

// 过滤后的标签刻度：保证相邻标签间距 ≥ minPx 像素，防止重叠
const labelTicks = computed<GridTick[]>(() => {
  const ticks = gridTicks.value
  const w = ganttTimelineW.value
  if (!ticks.length || !w) return ticks
  const minPx = 48 // 标签最小间距（px）
  const maxLabels = Math.max(1, Math.floor(w / minPx))
  if (ticks.length <= maxLabels) return ticks
  const step = Math.ceil(ticks.length / maxLabels)
  return ticks.filter((_, i) => i % step === 0)
})

// backward compat alias — grid lines still use all gridTicks
const monthLabels = gridTicks

function handleGanttWheel(e: WheelEvent) {
  e.preventDefault()
  const idx = SCALES.indexOf(ganttScale.value)
  if (e.deltaY < 0 && idx < SCALES.length - 1) ganttScale.value = SCALES[idx + 1] as GanttScale
  else if (e.deltaY > 0 && idx > 0) ganttScale.value = SCALES[idx - 1] as GanttScale
}
const todayLeft   = computed(() => {
  const { start, end } = ganttRange.value
  const now = Date.now()
  if (now < start.getTime() || now > end.getTime()) return null
  return `${((now - start.getTime()) / (end.getTime() - start.getTime())) * 100}%`
})

// AI 聊天上下文
const goalChatContext = computed(() => {
  const token = localStorage.getItem('aipanel_token') || 'TOKEN'
  const base  = `${window.location.protocol}//${window.location.host}`
  const agentCtx = agentList.value.map(a => `- ${a.id}: ${a.name}`).join('\n')
  const currentGoalCtx = selectedGoal.value
    ? `\n### 当前选中目标\nID: ${selectedGoal.value.id}\n标题: ${selectedGoal.value.title}\n状态: ${selectedGoal.value.status}\n进度: ${selectedGoal.value.progress}%\n开始: ${selectedGoal.value.startAt || '未设置'}\n结束: ${selectedGoal.value.endAt || '未设置'}`
    : ''

  return `## 目标规划助手

你是团队的目标规划助手。

### 🎯 核心能力：填写表单（优先使用）

当用户描述目标信息时，**直接输出 JSON 填充表单**，格式如下：

\`\`\`json
{"action":"fill_goal","data":{"title":"目标标题","description":"描述（可选）","type":"team","agentIds":["agentId1"],"status":"active","startAt":"2026-03-01T00:00:00Z","endAt":"2026-06-30T00:00:00Z","progress":0,"milestones":[{"title":"里程碑1","dueAt":"2026-04-01T00:00:00Z","done":false}]}}
\`\`\`

- type: "personal"（个人）或 "team"（团队）
- status: "draft" / "active" / "completed" / "cancelled"
- agentIds: 参与成员的 ID 列表
- 时间格式：ISO 8601（如 "2026-03-01T00:00:00Z"）
- 输出 JSON 后，页面会自动填充表单，用户确认后保存

### API 操作（如需直接更新已有目标）

**更新进度：**
\`\`\`bash
curl -s -X PATCH ${base}/api/goals/{id}/progress -H "Authorization: Bearer ${token}" -H "Content-Type: application/json" -d '{"progress":50}'
\`\`\`

**添加定期检查：**
\`\`\`bash
curl -s -X POST ${base}/api/goals/{id}/checks -H "Authorization: Bearer ${token}" -H "Content-Type: application/json" -d '{"name":"每周检查","schedule":"0 9 * * 1","agentId":"agentId","tz":"Asia/Shanghai","prompt":"请检查目标「{goal.title}」本周进展","enabled":true}'
\`\`\`

### 当前团队成员
${agentCtx}
${currentGoalCtx}

**工作流程：**
1. 用户描述目标 → 你输出 fill_goal JSON → 页面自动填表
2. 用户确认内容后自行点击「创建」或「保存」按钮
3. 如需更新已存在目标的进度/检查，使用 API 操作`.trim()
})

// ── 生命周期 ─────────────────────────────────────────────────────────────
onMounted(async () => {
  const res = await agentsApi.list().catch(() => ({ data: [] as AgentInfo[] }))
  agentList.value = (res.data || []).filter(a => !a.system)
  selectedChatAgentId.value = agentList.value[0]?.id || ''
  await loadGoals()
})

// ganttTimelineRef 在 v-else 条件块内，DOM 渲染后才会挂载
// 用 watch 监听 ref 变化，避免 onMounted 时 ref 为 null
watch(ganttTimelineRef, (el) => {
  if (!el) return
  const ro = new ResizeObserver(entries => {
    if (entries[0]) ganttTimelineW.value = entries[0].contentRect.width
  })
  ro.observe(el)
  // 立即读取一次当前宽度
  ganttTimelineW.value = el.getBoundingClientRect().width
}, { immediate: true })

watch(editorTab, async (tab) => {
  if (tab === 'records' && selectedGoal.value) {
    await loadCheckRecords(selectedGoal.value.id)
  }
})

// ── 数据加载 ─────────────────────────────────────────────────────────────
async function loadGoals() {
  try {
    const res = await goalsApi.list()
    goals.value = res.data || []
    // Refresh selectedGoal if still present
    if (selectedGoal.value) {
      const updated = goals.value.find(g => g.id === selectedGoal.value!.id)
      if (updated) selectedGoal.value = updated
    }
  } catch {}
}

async function loadCheckRecords(goalId: string) {
  checkRecordsLoading.value = true
  try {
    const res = await goalsApi.listCheckRecords(goalId)
    checkRecords.value = (res.data || []).slice().reverse()
  } catch { checkRecords.value = [] }
  finally { checkRecordsLoading.value = false }
}

// ── 选择/新建 ─────────────────────────────────────────────────────────────
function selectGoal(g: GoalInfo) {
  selectedGoal.value = g
  creating.value = false
  editorTab.value = 'basic'
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
}

function openCreate() {
  selectedGoal.value = null
  creating.value = true
  editorTab.value = 'basic'
  createSessionStamp.value = Date.now() // 每次新建都刷新 session
  Object.assign(form, {
    title: '', description: '', type: 'team', agentIds: [],
    status: 'draft', progress: 0, startAt: '', endAt: '', milestones: [],
  })
}

// ── 保存/删除 ─────────────────────────────────────────────────────────────
async function saveGoal() {
  if (!form.title.trim()) { ElMessage.warning('请填写目标标题'); return }
  saving.value = true
  const payload: any = {
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
    })),
  }
  try {
    if (selectedGoal.value) {
      await goalsApi.update(selectedGoal.value.id, payload)
      ElMessage.success('保存成功')
      await loadGoals()
    } else {
      const res = await goalsApi.create(payload)
      ElMessage.success('创建成功')
      creating.value = false
      await loadGoals()
      // 自动选中刚创建的目标
      const newGoal = goals.value.find(g => g.id === res.data.id) || res.data
      selectGoal(newGoal)
    }
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '操作失败')
  } finally {
    saving.value = false
  }
}

async function deleteGoal() {
  if (!selectedGoal.value) return
  try {
    await goalsApi.delete(selectedGoal.value.id)
    ElMessage.success('已删除')
    selectedGoal.value = null
    await loadGoals()
  } catch { ElMessage.error('删除失败') }
}

function addMilestone() {
  form.milestones.push({
    id: 'ms-' + Math.random().toString(36).slice(2, 10),
    title: '', dueAt: '', done: false, agentIds: [],
  })
}

// ── 定期检查 ─────────────────────────────────────────────────────────────
function openAddCheckDialog() {
  Object.assign(checkForm, {
    name: '', agentId: agentList.value[0]?.id || '',
    schedule: '0 9 * * 1', tz: 'Asia/Shanghai',
    prompt: '请检查目标「{goal.title}」的进展情况（当前进度 {goal.progress}），距截止日期 {goal.endAt} 还有一段时间，请总结近期进展并建议下一步行动。',
    enabled: true,
  })
  checkFreqPreset.value = '0 9 * * 1'
  checkDialogVisible.value = true
}

function onPresetChange(val: string) {
  if (val !== 'custom') checkForm.schedule = val
}

async function submitAddCheck() {
  if (!checkForm.name.trim()) { ElMessage.warning('请填写检查名称'); return }
  if (!checkForm.agentId) { ElMessage.warning('请选择执行成员'); return }
  if (!selectedGoal.value) return
  try {
    await goalsApi.addCheck(selectedGoal.value.id, { ...checkForm })
    ElMessage.success('添加成功')
    checkDialogVisible.value = false
    const res = await goalsApi.get(selectedGoal.value.id)
    selectedGoal.value = res.data
    await loadGoals()
  } catch (e: any) { ElMessage.error(e.response?.data?.error || '添加失败') }
}

async function toggleCheck(check: GoalCheck) {
  if (!selectedGoal.value) return
  try {
    await goalsApi.updateCheck(selectedGoal.value.id, check.id, { enabled: check.enabled } as any)
  } catch { ElMessage.error('更新失败') }
}

async function runCheckNow(check: GoalCheck) {
  if (!selectedGoal.value) return
  try {
    await goalsApi.runCheck(selectedGoal.value.id, check.id)
    ElMessage.success('已触发检查')
  } catch (e: any) { ElMessage.error(e.response?.data?.error || '触发失败') }
}

async function removeCheck(check: GoalCheck) {
  if (!selectedGoal.value) return
  try {
    await ElMessageBox.confirm(`确定删除检查计划「${check.name}」？`, '删除确认', { type: 'warning' })
    await goalsApi.removeCheck(selectedGoal.value.id, check.id)
    ElMessage.success('已删除')
    const res = await goalsApi.get(selectedGoal.value.id)
    selectedGoal.value = res.data
    await loadGoals()
  } catch (e: any) { if (e !== 'cancel') ElMessage.error('删除失败') }
}

// 每次打开"新建目标"时生成独立 session，防止复用历史对话
const createSessionStamp = ref(0)

// 每个目标独立的对话 session（切换目标自动切换历史）
const goalChatSessionId = computed(() => {
  if (!selectedChatAgentId.value) return ''
  if (selectedGoal.value) return `goal-${selectedGoal.value.id}-${selectedChatAgentId.value}`
  // 新建目标：每次 openCreate() 都会更新 stamp，保证 session 全新
  return `goals-new-${createSessionStamp.value}-${selectedChatAgentId.value}`
})

// AI 输出 JSON 后自动填充表单
function onAiResponse(text: string) {
  // 刷新目标列表
  setTimeout(() => loadGoals(), 2000)
  // 尝试解析 JSON fill 指令
  // 支持两种格式：
  //   {"action":"fill_goal","data":{...}}
  //   ```json\n{"action":"fill_goal",...}\n```
  const tryFill = (jsonStr: string) => {
    try {
      const obj = JSON.parse(jsonStr)
      if (obj.action === 'fill_goal' && obj.data) {
        applyFormFill(obj.data)
        return true
      }
    } catch {}
    return false
  }

  // 先尝试代码块内
  const codeBlock = text.match(/```(?:json)?\s*(\{[\s\S]*?\})\s*```/)
  if (codeBlock?.[1] && tryFill(codeBlock[1])) return

  // 再尝试裸 JSON
  const bare = text.match(/(\{"action"\s*:\s*"fill_goal"[\s\S]*?\})/)
  if (bare?.[1] && tryFill(bare[1])) return
}

function applyFormFill(data: any) {
  if (!creating.value && !selectedGoal.value) {
    // 先进入新建状态
    openCreate()
  }
  if (data.title)       form.title       = data.title
  if (data.description) form.description = data.description
  if (data.type)        form.type        = data.type as any
  if (data.status)      form.status      = data.status as any
  if (typeof data.progress === 'number') form.progress = data.progress
  if (data.agentIds && Array.isArray(data.agentIds)) form.agentIds = data.agentIds
  if (data.startAt)     form.startAt     = data.startAt
  if (data.endAt)       form.endAt       = data.endAt
  if (data.milestones && Array.isArray(data.milestones)) {
    form.milestones = data.milestones.map((m: any) => ({
      id: m.id || 'ms-' + Math.random().toString(36).slice(2, 10),
      title: m.title || '',
      dueAt: m.dueAt || '',
      done: !!m.done,
      agentIds: m.agentIds || [],
    }))
  }
  ElMessage.success('AI 已填写表单，确认后点击保存')
}

// ── 甘特图辅助 ────────────────────────────────────────────────────────────
function isValidDate(val?: string) {
  if (!val) return false
  const d = new Date(val)
  return !isNaN(d.getTime()) && d.getFullYear() > 1970
}
function isValidBar(g: GoalInfo) {
  return isValidDate(g.startAt) && isValidDate(g.endAt)
}
function calcBarWidth(g: GoalInfo) {
  const { start, end } = ganttRange.value
  const total = end.getTime() - start.getTime()
  return Math.max(1, ((new Date(g.endAt).getTime() - new Date(g.startAt).getTime()) / total) * 100)
}
function ganttBarStyle(g: GoalInfo) {
  const { start, end } = ganttRange.value
  const total = end.getTime() - start.getTime()
  const gS = new Date(g.startAt).getTime()
  const gE = new Date(g.endAt).getTime()
  const left  = Math.max(0, ((gS - start.getTime()) / total) * 100)
  const width = Math.max(1, ((gE - gS) / total) * 100)
  const c1 = (g.agentIds?.[0] && agentColorMap.value[g.agentIds[0]]) ? agentColorMap.value[g.agentIds[0]] : '#409eff'
  const c2 = (g.agentIds?.[1] && agentColorMap.value[g.agentIds[1]]) ? agentColorMap.value[g.agentIds[1]] : c1
  return { left: `${left}%`, width: `${width}%`, background: g.agentIds?.length > 1 ? `linear-gradient(90deg,${c1},${c2})` : c1 }
}
function milestoneLeft(ms: Milestone) {
  const { start, end } = ganttRange.value
  if (!isValidDate(ms.dueAt)) return '-100%'
  return `${((new Date(ms.dueAt).getTime() - start.getTime()) / (end.getTime() - start.getTime())) * 100}%`
}
function calcGridTicks(rangeStart: Date, rangeEnd: Date, scale: GanttScale): GridTick[] {
  const ticks: GridTick[] = []
  const total = rangeEnd.getTime() - rangeStart.getTime()
  if (total <= 0) return ticks
  let seenYear = -1

  if (scale === 'quarter' || scale === 'month') {
    const step = scale === 'quarter' ? 3 : 1
    const cur = new Date(rangeStart.getFullYear(), rangeStart.getMonth(), 1)
    while (cur <= rangeEnd) {
      const left = ((cur.getTime() - rangeStart.getTime()) / total) * 100
      if (left >= -0.01 && left <= 100.01) {
        const yr = cur.getFullYear()
        const mo = cur.getMonth() + 1
        const yearMark = yr !== seenYear ? String(yr) : undefined
        if (yearMark) seenYear = yr
        ticks.push({ label: String(mo), yearMark, left: `${left}%` })
      }
      cur.setMonth(cur.getMonth() + step)
    }
  } else {
    // week (7d) or biweek (14d)
    const step = scale === 'week' ? 7 : 14
    // align to nearest Monday on/before rangeStart
    const cur = new Date(rangeStart)
    const dow = cur.getDay() // 0=Sun
    cur.setDate(cur.getDate() - (dow === 0 ? 6 : dow - 1))
    while (cur <= rangeEnd) {
      const left = ((cur.getTime() - rangeStart.getTime()) / total) * 100
      if (left >= -0.01 && left <= 100.01) {
        const yr = cur.getFullYear()
        const mo = cur.getMonth() + 1
        const d = cur.getDate()
        const yearMark = yr !== seenYear ? String(yr) : undefined
        if (yearMark) seenYear = yr
        ticks.push({ label: `${mo}/${d}`, yearMark, left: `${left}%` })
      }
      cur.setDate(cur.getDate() + step)
    }
  }
  return ticks
}
function progressColor(g: GoalInfo) {
  if (g.status === 'completed') return '#67c23a'
  if (g.progress >= 80) return '#409eff'
  if (g.progress >= 40) return '#e6a23c'
  return '#909399'
}

// ── 辅助 ─────────────────────────────────────────────────────────────────
function statusLabel(s: string) {
  return ({ draft: '草稿', active: '进行中', completed: '已完成', cancelled: '已取消' } as Record<string,string>)[s] ?? s
}
function statusTagType(s: string): '' | 'info' | 'success' | 'danger' | 'warning' {
  return ({ draft: 'info', active: '', completed: 'success', cancelled: 'danger' } as Record<string, '' | 'info' | 'success' | 'danger' | 'warning'>)[s] ?? 'info'
}
function formatDate(val?: string) {
  if (!isValidDate(val)) return '—'
  return new Date(val!).toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' })
}
function formatDateTime(val?: string) {
  if (!val) return ''
  const d = new Date(val)
  return isNaN(d.getTime()) ? '' : d.toLocaleString('zh-CN')
}
</script>

<style scoped>
/* ── 三栏容器 ────────────────────────────────────────────────────────── */
.goals-studio {
  display: flex;
  /* 逃脱 app-main 的 padding(20px 24px)，撑满视口高度 */
  height: calc(100vh - 44px);
  margin: -20px -24px;
  overflow: hidden;
  background: #f5f7fa;
  user-select: none;
}

/* ── 左侧边栏 ────────────────────────────────────────────────────────── */
.gs-sidebar {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: #fff;
  border-right: 1px solid #e4e7ed;
  overflow: hidden;
}
.sidebar-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 12px 8px;
  border-bottom: 1px solid #f0f0f0;
  flex-shrink: 0;
}
.sidebar-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
}
.sidebar-acts { display: flex; gap: 4px; }

.gs-filter {
  display: flex;
  gap: 6px;
  padding: 8px 10px;
  border-bottom: 1px solid #f0f0f0;
  flex-shrink: 0;
}
.filter-sel { flex: 1; }

.goal-list {
  flex: 1;
  overflow-y: auto;
  padding: 4px 0;
}
.list-empty {
  text-align: center;
  padding: 32px 12px;
  font-size: 13px;
  color: #94a3b8;
}

/* 目标条目 */
.goal-item {
  padding: 10px 12px;
  cursor: pointer;
  transition: background 0.15s;
  border-left: 3px solid transparent;
  user-select: none;
}
.goal-item:hover { background: #f5f7fa; }
.goal-item.active { background: #ecf5ff; border-left-color: #409eff; }

/* 新建占位条目 */
.goal-item-new {
  border-left-color: #409eff;
  background: #ecf5ff;
  border-bottom: 1px dashed #c6e2ff;
}
.gi-title-new {
  color: #409eff;
  font-style: italic;
}
.gi-new-hint {
  font-size: 11px;
  color: #a0cfff;
  margin-top: 4px;
}

.gi-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  margin-bottom: 6px;
}
.gi-title {
  font-size: 13px;
  font-weight: 600;
  color: #303133;
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.gi-progress-wrap {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
  background: #f0f2f5;
  border-radius: 6px;
  height: 6px;
  position: relative;
}
.gi-progress-bar {
  height: 100%;
  border-radius: 6px;
  transition: width 0.4s;
  min-width: 4px;
}
.gi-progress-num {
  position: absolute;
  right: 0;
  top: -14px;
  font-size: 10px;
  color: #94a3b8;
}
.gi-bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.gi-avatars { display: flex; gap: 2px; align-items: center; }
.gi-avatar {
  width: 18px;
  height: 18px;
  border-radius: 50%;
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
}
.gi-avatar-more { font-size: 10px; color: #94a3b8; margin-left: 2px; }
.gi-dates { font-size: 11px; color: #94a3b8; }

/* ── 拖拽手柄 ────────────────────────────────────────────────────────── */
.gs-handle {
  width: 4px;
  background: #e4e7ed;
  cursor: col-resize;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s;
  z-index: 10;
}
.gs-handle:hover, .gs-handle.dragging { background: #409eff; }
.gs-handle-bar {
  width: 2px; height: 28px;
  background: rgba(255,255,255,0.6);
  border-radius: 2px;
}

/* ── 中：编辑区 ──────────────────────────────────────────────────────── */
.gs-editor {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: #fff;
  border-right: 1px solid #e4e7ed;
}

/* 工具栏 */
.editor-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  border-bottom: 1px solid #f0f0f0;
  background: #fafafa;
  flex-shrink: 0;
}
.editor-breadcrumb {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  overflow: hidden;
}
.crumb-sep  { color: #909399; }
.crumb-name { font-weight: 600; color: #303133; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.toolbar-acts { display: flex; gap: 6px; flex-shrink: 0; }

/* 表单 */
.editor-form {
  flex: 1;
  overflow-y: auto;
  padding: 16px 20px;
}
.goal-inner-form :deep(.el-form-item) {
  margin-bottom: 14px;
}
.milestone-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
}

/* Tabs */
.editor-tabs {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.editor-tabs :deep(.el-tabs__header) {
  margin: 0;
  padding: 0 16px;
  flex-shrink: 0;
}
.editor-tabs :deep(.el-tabs__content) {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
.editor-tabs :deep(.el-tab-pane) {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.tab-panel {
  flex: 1;
  overflow-y: auto;
  padding: 12px 16px;
}
.tab-panel-head {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 10px;
}
.checks-table { font-size: 12px; }
.code-cell { font-family: monospace; font-size: 11px; }

/* 检查记录 */
.records-timeline { padding: 0 8px; }
.record-card {
  background: #f5f7fa;
  border-radius: 8px;
  padding: 10px 12px;
}
.record-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}
.rec-avatar {
  width: 22px; height: 22px;
  border-radius: 50%;
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.rec-name { font-size: 13px; font-weight: 600; }
.rec-output { font-size: 12px; color: #606266; white-space: pre-wrap; line-height: 1.6; }

/* 甘特图总览（空态时中栏显示） */
.editor-gantt-overview {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.overview-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 20px;
  border-bottom: 1px solid #f0f0f0;
  background: #fafafa;
  flex-shrink: 0;
}
.overview-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
}
.gantt-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: #94a3b8;
  font-size: 13px;
}

/* gantt-wrap：可滚动区域 */
.gantt-wrap {
  flex: 1;
  overflow: auto;
  display: flex;
  flex-direction: column;
  min-width: 0;
}

/* gantt-body：行容器，相对定位供覆盖层使用 */
.gantt-body {
  position: relative;
  flex: 1;
}

/* 网格线+今日线覆盖层：绝对铺满 gantt-body，pointer-events:none 不挡点击 */
.gantt-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  pointer-events: none;
  z-index: 0;
}
.gantt-overlay .gantt-label-col {
  border: none;
  background: transparent;
}
.gantt-overlay .gantt-timeline-col {
  flex: 1;
  position: relative;
  overflow: hidden;
}

.gantt-header {
  display: flex;
  align-items: stretch;
  height: 42px;
  margin-bottom: 0;
  border-bottom: 1px solid #e4e7ed;
}
.gantt-label-col {
  width: 200px;
  flex-shrink: 0;
  border-right: 1px solid #e4e7ed;
  display: flex;
  align-items: center;
}
.gantt-scale-hint {
  font-size: 10px;
  color: #c0c4cc;
  padding-left: 8px;
  user-select: none;
}
.gantt-timeline-col {
  flex: 1;
  position: relative;
  overflow: hidden;
}
/* 年份行（顶层，较小字体） */
.gantt-years { position: relative; height: 16px; }
.gantt-year-label {
  position: absolute;
  font-size: 10px;
  font-weight: 600;
  color: #c0c4cc;
  transform: translateX(2px);
  white-space: nowrap;
  top: 1px;
  letter-spacing: 0.3px;
}
/* 月/周刻度行（主标签） */
.gantt-months { position: relative; height: 22px; }
.gantt-month-label {
  position: absolute;
  font-size: 12px;
  color: #606266;
  transform: translateX(-50%);
  white-space: nowrap;
  font-weight: 500;
  top: 3px;
}
.gantt-grid-line {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 1px;
  background: rgba(0,0,0,0.07);
}
.gantt-today-line {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 2px;
  background: #f56c6c;
  opacity: 0.7;
  z-index: 5;
}

.gantt-row {
  display: flex;
  align-items: stretch;
  height: 44px;
  border-bottom: 1px solid #f0f0f0;
  cursor: pointer;
  transition: background 0.15s;
  position: relative;
  z-index: 1;
}
.gantt-row:hover { background: rgba(64,158,255,0.04); }
.gantt-row .gantt-label-col {
  display: flex;
  align-items: center;
}
.gantt-row .gantt-timeline-col {
  position: relative;
  flex: 1;
  overflow: hidden;
}

/* 进度百分比标签 */
.gantt-pct {
  font-size: 10px;
  font-weight: 600;
  padding: 1px 5px;
  border-radius: 8px;
  flex-shrink: 0;
  margin-left: 3px;
}
.gantt-pct.s-active    { background: #ecf5ff; color: #409eff; }
.gantt-pct.s-completed { background: #f0f9eb; color: #67c23a; }
.gantt-pct.s-draft     { background: #f4f4f5; color: #909399; }
.gantt-pct.s-cancelled { background: #fef0f0; color: #f56c6c; }

.gantt-label-inner {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 10px;
  overflow: hidden;
}
.gantt-agent-avatars { display: flex; gap: 2px; flex-shrink: 0; }
.gantt-avatar {
  width: 22px; height: 22px;
  border-radius: 50%;
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
}
.gantt-goal-name {
  font-size: 12px;
  color: #303133;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
}

.gantt-bar {
  position: absolute;
  height: 24px;
  border-radius: 6px;
  top: 50%;
  transform: translateY(-50%);
  overflow: hidden;
  min-width: 6px;
  opacity: 0.9;
  transition: opacity 0.15s, box-shadow 0.15s;
  box-shadow: 0 1px 4px rgba(0,0,0,0.12);
}
.gantt-bar:hover {
  opacity: 1;
  box-shadow: 0 2px 8px rgba(0,0,0,0.18);
}
.gantt-bar-progress {
  height: 100%;
  background: rgba(255,255,255,0.25);
  border-radius: 6px;
  transition: width 0.4s;
}
.gantt-bar-label {
  position: absolute;
  left: 8px;
  top: 50%;
  transform: translateY(-50%);
  font-size: 11px;
  color: #fff;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: calc(100% - 16px);
  text-shadow: 0 1px 2px rgba(0,0,0,0.3);
}
.gantt-milestone {
  position: absolute;
  width: 10px; height: 10px;
  border: 2px solid #e6a23c;
  background: #fff;
  transform: translateY(-50%) translateX(-50%) rotate(45deg);
  top: 50%;
  z-index: 4;
}
.gantt-milestone.done { background: #67c23a; border-color: #67c23a; }
.gantt-no-date {
  font-size: 11px;
  color: #c0c4cc;
  padding: 0 12px;
  line-height: 40px;
}

/* ── 右：AI 对话 ──────────────────────────────────────────────────────── */
.gs-chat {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: #fff;
  overflow: hidden;
}
.chat-panel-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 11px 14px;
  font-size: 13px;
  font-weight: 600;
  color: #303133;
  border-bottom: 1px solid #f0f0f0;
  background: #fafafa;
  flex-shrink: 0;
}
.chat-wrap {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
</style>
