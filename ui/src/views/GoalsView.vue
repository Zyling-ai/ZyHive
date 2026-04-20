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

        <div v-else
          class="gantt-wrap"
          :class="{ 'is-dragging': ganttDragging }"
          @wheel.prevent="handleGanttWheel"
          @mousedown="onGanttMouseDown"
          @mousemove="onGanttMouseMove"
          @mouseup.window="onGanttMouseUp"
          @mouseleave.self="onGanttMouseUp"
        >
          <!-- 时间刻度行（双层：年份 + 月/周） -->
          <div class="gantt-header">
            <div class="gantt-label-col">
              <span class="gantt-scale-hint">{{ tickStep.label }}/格 ↕缩放</span>
            </div>
            <div class="gantt-timeline-col" ref="ganttTimelineRef">
              <!-- 年份标记层：只在 labelTicks 里有 yearMark 的位置显示 -->
              <div class="gantt-years">
                <div v-for="t in labelTicks.filter(t => t.yearMark)" :key="'yr-' + t.ts" class="gantt-year-label" :style="{ left: t.left }">
                  {{ t.yearMark }}
                </div>
              </div>
              <!-- 月/周刻度层：稀疏化后的标签 -->
              <div class="gantt-months">
                <div v-for="t in labelTicks" :key="'lbl-' + t.ts" class="gantt-month-label" :style="{ left: t.left }">
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
                <div v-for="m in monthLabels" :key="'gv-' + m.ts" class="gantt-grid-line" :style="{ left: m.left }" />
                <div v-if="todayLeft !== null" class="gantt-today-line" :style="{ left: todayLeft }" />
              </div>
            </div>
            <!-- 目标行 -->
            <div v-for="g in filteredGoals" :key="g.id" class="gantt-row"
              :class="{ 'is-selected': ganttSelectedGoal?.id === g.id }"
              @click="onGanttBarClick(g)"
            >
              <div class="gantt-label-col">
                <div class="gantt-label-inner">
                  <div class="gantt-agent-avatars">
                    <div
                      v-for="id in (g.agentIds || []).slice(0, 2)" :key="id"
                      class="gantt-avatar" :style="{ background: agentColorMap[id] || '#409eff' }"
                    >{{ (agentNameMap[id] || id).slice(0, 1) }}</div>
                  </div>
                  <span class="gantt-goal-name">{{ g.title }}</span>
                  <span class="gantt-pct" :class="'s-' + g.status">{{ timeProgress(g) }}%</span>
                </div>
              </div>
              <div class="gantt-timeline-col">
                <template v-if="isValidBar(g)">
                  <div class="gantt-bar" :style="ganttBarStyle(g)">
                    <div class="gantt-bar-progress" :style="{ width: timeProgress(g) + '%' }" />
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

        <!-- 目标摘要面板（点击甘特条后弹出） -->
        <transition name="gsp-slide">
          <div v-if="ganttSelectedGoal" class="gantt-summary-panel">
            <div class="gsp-header">
              <div class="gsp-avatars">
                <div v-for="id in (ganttSelectedGoal.agentIds||[]).slice(0,3)" :key="id"
                  class="gsp-avatar" :style="{ background: agentColorMap[id]||'#409eff' }">
                  {{ (agentNameMap[id]||id).slice(0,1) }}
                </div>
              </div>
              <div class="gsp-title-block">
                <span class="gsp-title">{{ ganttSelectedGoal.title }}</span>
                <el-tag :type="statusTagType(ganttSelectedGoal.status)" size="small">
                  {{ statusLabel(ganttSelectedGoal.status) }}
                </el-tag>
              </div>
              <el-button type="primary" size="small" @click="selectGoal(ganttSelectedGoal); ganttSelectedGoal=null">
                查看详情 →
              </el-button>
              <el-icon class="gsp-close" @click="ganttSelectedGoal=null"><Close /></el-icon>
            </div>
            <div class="gsp-body">
              <div class="gsp-stat">
                <span class="gsp-stat-label">时间进度</span>
                <el-progress :percentage="timeProgress(ganttSelectedGoal)"
                  :color="progressColor(ganttSelectedGoal)" :stroke-width="8"
                  style="flex:1;margin:0 10px" />
                <span class="gsp-stat-val">{{ timeProgress(ganttSelectedGoal) }}%</span>
              </div>
              <div class="gsp-stat">
                <span class="gsp-stat-label">时间</span>
                <span class="gsp-stat-val">
                  {{ formatDate(ganttSelectedGoal.startAt) }} — {{ formatDate(ganttSelectedGoal.endAt) }}
                </span>
              </div>
              <div v-if="ganttSelectedGoal.milestones?.length" class="gsp-stat">
                <span class="gsp-stat-label">里程碑</span>
                <span class="gsp-stat-val">
                  {{ ganttSelectedGoal.milestones.filter(m=>m.done).length }}/{{ ganttSelectedGoal.milestones.length }} 已完成
                </span>
              </div>
              <div v-if="ganttSelectedGoal.description" class="gsp-desc">
                {{ ganttSelectedGoal.description }}
              </div>
            </div>
          </div>
        </transition>
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
            <el-button size="small" text @click="selectedGoal = null" class="crumb-back" title="返回甘特图">
              <el-icon><ArrowLeft /></el-icon> 甘特图
            </el-button>
            <span class="crumb-sep">/</span>
            <el-icon style="color:#909399"><Flag /></el-icon>
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
import { Plus, Refresh, Delete, Flag, Loading, ChatLineRound, DocumentChecked, Close, ArrowLeft } from '@element-plus/icons-vue'
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

// ── 甘特图：连续缩放 + 惯性平移（地图式操作）────────────────────────────
const GANTT_MIN_MS  = 2   * 60_000               // 最小可见时长：2 分钟
const GANTT_MAX_MS  = 20  * 365 * 86400_000       // 最大可见时长：20 年
const GANTT_INIT_MS = 30  * 86400_000             // 默认：30 天（天级别，避免 |0 溢出）

// 可见时长（连续值，取代离散 scale）
const ganttDuration = ref(GANTT_INIT_MS)
// 视口中心时刻：初始让今天出现在左侧约 10% 处
const viewCenterMs  = ref(Date.now() + GANTT_INIT_MS * 0.4)

// 刻度步长表（由可见时长自动决定，不需要手动切换）
// ⚠️ 不要对大数字用 | 0 (32-bit 截断会溢出导致负数)，直接用浮点即可
interface TickStep {
  ms: number; kind: 'minute' | 'hour' | 'day' | 'week' | 'month' | 'year'
  step: number; label: string
}
const MS_MONTH = Math.round(30.44 * 86400_000)  // ~2,630,016,000 — 浮点安全
const TICK_STEPS: TickStep[] = [
  { ms: 60_000,           kind:'minute', step:1,  label:'1分钟' },
  { ms: 5   * 60_000,     kind:'minute', step:5,  label:'5分钟' },
  { ms: 15  * 60_000,     kind:'minute', step:15, label:'15分钟' },
  { ms: 30  * 60_000,     kind:'minute', step:30, label:'30分钟' },
  { ms: 3600_000,         kind:'hour',   step:1,  label:'1小时' },
  { ms: 6   * 3600_000,   kind:'hour',   step:6,  label:'6小时' },
  { ms: 12  * 3600_000,   kind:'hour',   step:12, label:'12小时' },
  { ms: 86400_000,        kind:'day',    step:1,  label:'1天' },
  { ms: 7   * 86400_000,  kind:'week',   step:7,  label:'1周' },
  { ms: MS_MONTH,         kind:'month',  step:1,  label:'1月' },
  { ms: 3   * MS_MONTH,   kind:'month',  step:3,  label:'3月' },
  { ms: 6   * MS_MONTH,   kind:'month',  step:6,  label:'6月' },
  { ms: 365 * 86400_000,  kind:'year',   step:1,  label:'1年' },
  { ms: 2   * 365 * 86400_000, kind:'year', step:2, label:'2年' },
  { ms: 5   * 365 * 86400_000, kind:'year', step:5, label:'5年' },
]
// 自动选最近似「目标 8 格」的步长
const tickStep = computed<TickStep>(() => {
  const target = ganttDuration.value / 8
  return TICK_STEPS.reduce((b, s) => Math.abs(s.ms - target) < Math.abs(b.ms - target) ? s : b)
})

// 甘特图可见范围（无吸附，连续平滑）
const ganttRange = computed(() => ({
  start: new Date(viewCenterMs.value - ganttDuration.value / 2),
  end:   new Date(viewCenterMs.value + ganttDuration.value / 2),
}))

interface GridTick { label: string; yearMark?: string; left: string; ts: number }
const gridTicks = computed<GridTick[]>(() =>
  calcGridTicks(ganttRange.value.start, ganttRange.value.end, tickStep.value))

// 追踪时间轴容器宽度，用于动态过滤标签密度
const ganttTimelineW   = ref(700)
const ganttTimelineRef = ref<HTMLElement | null>(null)

const labelTicks = computed<GridTick[]>(() => {
  const ticks = gridTicks.value
  const w = ganttTimelineW.value
  if (!ticks.length || !w) return ticks
  const minPx = 50
  const maxLabels = Math.max(1, Math.floor(w / minPx))
  if (ticks.length <= maxLabels) return ticks
  const step = Math.ceil(ticks.length / maxLabels)
  return ticks.filter((_, i) => i % step === 0)
})

// 网格线使用全密度刻度
const monthLabels = gridTicks

// ── 拖拽平移（非响应式变量，避免 Vue 追踪开销）──────────────────────────
const ganttDragging = ref(false)
const ganttDragged  = ref(false)
let _gDragActive = false, _gDragMoved = false
let _gLastX = 0, _gLastT = 0, _gVel = 0, _gMomentumId = 0

function _cancelMomentum() { cancelAnimationFrame(_gMomentumId); _gVel = 0 }

function onGanttMouseDown(e: MouseEvent) {
  _cancelMomentum()
  _gDragActive = true; _gDragMoved = false
  ganttDragging.value = true; ganttDragged.value = false
  _gLastX = e.clientX; _gLastT = Date.now(); _gVel = 0
}
function onGanttMouseMove(e: MouseEvent) {
  if (!_gDragActive) return
  const dx = e.clientX - _gLastX
  const dt = Date.now() - _gLastT
  if (Math.abs(dx) > 3) { _gDragMoved = true; ganttDragged.value = true }
  if (_gDragMoved) {
    const w = Math.max(100, ganttTimelineW.value || 700)
    // maxV 正比于屏幕宽度：确保惯性距离与屏幕宽窄无关，始终 ≈ 1/3 × ganttDuration
    // 推导：total = (w/K)/w × ganttDuration × 16/0.12 → total/ganttDuration = 16/(K×0.12)
    // 取 K=400 → total ≈ 0.33 × ganttDuration（无论手机/桌面、30天/90天视图）
    const K = 400
    const maxV = Math.max(100, ganttTimelineW.value || 700) / K
    const rawV = dt > 0 ? dx / dt : 0
    const clampedV = Math.max(-maxV, Math.min(maxV, rawV))
    if (dt > 0) _gVel = _gVel * 0.65 + clampedV * 0.35
    viewCenterMs.value -= (dx / w) * ganttDuration.value
  }
  _gLastX = e.clientX; _gLastT = Date.now()
}
function onGanttMouseUp() {
  if (!_gDragActive) return
  _gDragActive = false; ganttDragging.value = false
  if (_gDragMoved && Math.abs(_gVel) > 0.05) {
    let v = _gVel, lt = Date.now()
    const run = () => {
      const now = Date.now(), dt = Math.min(now - lt, 64); lt = now  // dt 最多 64ms，防止帧延迟导致大跳
      const w = Math.max(100, ganttTimelineW.value || 700)          // 宽度至少 100px，防止除以超小值
      viewCenterMs.value -= v * dt / w * ganttDuration.value
      // viewCenterMs 夹在合理范围：前后各 5 年
      const fiveYears = 5 * 365 * 86400_000
      viewCenterMs.value = Math.max(Date.now() - fiveYears, Math.min(Date.now() + fiveYears, viewCenterMs.value))
      v *= Math.pow(0.88, dt / 16)   // 摩擦力：0.88/16ms，比 0.92 衰减更快，避免惯性飞太远
      if (Math.abs(v) > 0.008) _gMomentumId = requestAnimationFrame(run)
    }
    _gMomentumId = requestAnimationFrame(run)
  }
}

// ── 滚轮缩放（向光标位置锚定，指数线性）──────────────────────────────────
function handleGanttWheel(e: WheelEvent) {
  e.preventDefault()
  _cancelMomentum()
  const factor  = Math.exp(e.deltaY * 0.0015)   // 每 deltaY=100 约缩放 16%
  const newDur  = Math.max(GANTT_MIN_MS, Math.min(GANTT_MAX_MS, ganttDuration.value * factor))
  // 以光标所在时刻为锚点，保证缩放后光标下的时间不变
  const rect = ganttTimelineRef.value?.getBoundingClientRect()
  if (rect && rect.width > 0) {
    const ratio        = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
    const timeAtCursor = (viewCenterMs.value - ganttDuration.value / 2) + ratio * ganttDuration.value
    viewCenterMs.value = timeAtCursor + newDur * (0.5 - ratio)
  }
  ganttDuration.value = newDur
}

// ── 甘特图摘要选中 ──────────────────────────────────────────────────────
const ganttSelectedGoal = ref<GoalInfo | null>(null)
function onGanttBarClick(g: GoalInfo) {
  if (ganttDragged.value) return
  ganttSelectedGoal.value = ganttSelectedGoal.value?.id === g.id ? null : g
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
  } catch (e: any) {
    ElMessage.error('加载目标失败: ' + (e?.message || '未知错误'))
    return
  }
  try {
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
function ganttBarRange(g: GoalInfo) {
  const { start, end } = ganttRange.value
  const total = end.getTime() - start.getTime()
  const gS = new Date(g.startAt).getTime()
  const gE = new Date(g.endAt).getTime()
  // 关键：right 用右端位置减去左端夹住后的值，否则 gS 超出左侧时 width 不随滚动缩减
  const leftRaw  = (gS - start.getTime()) / total * 100
  const rightRaw = (gE - start.getTime()) / total * 100
  const left  = Math.max(0, leftRaw)
  const right = Math.max(left, rightRaw)          // right 不能小于 left
  const width = Math.max(1, right - left)         // 可见宽度（随滚动动态缩减）
  return { left, width }
}
function calcBarWidth(g: GoalInfo) {
  return ganttBarRange(g).width
}
function ganttBarStyle(g: GoalInfo) {
  const { left, width } = ganttBarRange(g)
  const c1 = (g.agentIds?.[0] && agentColorMap.value[g.agentIds[0]]) ? agentColorMap.value[g.agentIds[0]] : '#409eff'
  const c2 = (g.agentIds?.[1] && agentColorMap.value[g.agentIds[1]]) ? agentColorMap.value[g.agentIds[1]] : c1
  return { left: `${left}%`, width: `${width}%`, background: g.agentIds?.length > 1 ? `linear-gradient(90deg,${c1},${c2})` : c1 }
}
function milestoneLeft(ms: Milestone) {
  const { start, end } = ganttRange.value
  if (!isValidDate(ms.dueAt)) return '-100%'
  return `${((new Date(ms.dueAt).getTime() - start.getTime()) / (end.getTime() - start.getTime())) * 100}%`
}
function calcGridTicks(rangeStart: Date, rangeEnd: Date, tick: TickStep): GridTick[] {
  const ticks: GridTick[] = []
  const total = rangeEnd.getTime() - rangeStart.getTime()
  if (total <= 0 || !isFinite(total)) return ticks

  // 保护：最多渲染 300 条刻度线。若当前步长会超出，自动升档到更粗的步长
  const MAX_TICKS = 300
  const estimatedCount = total / tick.ms
  if (estimatedCount > MAX_TICKS) {
    const saferTick = [...TICK_STEPS].reverse().find(s => total / s.ms <= MAX_TICKS)
    if (saferTick) tick = saferTick
  }

  const pct = (d: Date) => ((d.getTime() - rangeStart.getTime()) / total * 100).toFixed(2) + '%'
  let seenParent = ''

  // 找到 rangeStart 之前最近的对齐刻度点
  let cur: Date
  if (tick.kind === 'minute') {
    cur = new Date(rangeStart); cur.setSeconds(0, 0)
    cur.setMinutes(Math.floor(cur.getMinutes() / tick.step) * tick.step)
  } else if (tick.kind === 'hour') {
    cur = new Date(rangeStart); cur.setMinutes(0, 0, 0)
    cur.setHours(Math.floor(cur.getHours() / tick.step) * tick.step)
  } else if (tick.kind === 'day') {
    cur = new Date(rangeStart.getFullYear(), rangeStart.getMonth(), rangeStart.getDate())
  } else if (tick.kind === 'week') {
    cur = new Date(rangeStart.getFullYear(), rangeStart.getMonth(), rangeStart.getDate())
    const dow = cur.getDay(); cur.setDate(cur.getDate() - (dow === 0 ? 6 : dow - 1))
  } else if (tick.kind === 'month') {
    const mo = rangeStart.getMonth()
    cur = new Date(rangeStart.getFullYear(), Math.floor(mo / tick.step) * tick.step, 1)
  } else { // year
    cur = new Date(Math.floor(rangeStart.getFullYear() / tick.step) * tick.step, 0, 1)
  }

  while (cur.getTime() <= rangeEnd.getTime()) {
    const yr = cur.getFullYear(), mo = cur.getMonth() + 1
    const d  = cur.getDate(),     h  = cur.getHours(), m = cur.getMinutes()

    // 上层父标签（日期/月份/年份变化时显示）
    let parentKey = '', parentLabel = ''
    if (tick.kind === 'minute' || tick.kind === 'hour') {
      parentKey = `${yr}-${mo}-${d}`
      parentLabel = `${mo}/${d}`
    } else if (tick.kind === 'day' || tick.kind === 'week') {
      parentKey = `${yr}-${mo}`
      parentLabel = mo === 1 ? `${yr}年` : `${mo}月`
    } else if (tick.kind === 'month') {
      parentKey = String(yr); parentLabel = String(yr)
    }
    const yearMark = (parentKey && parentKey !== seenParent) ? parentLabel : undefined
    if (yearMark) seenParent = parentKey

    // 刻度主标签
    let label = ''
    if      (tick.kind === 'minute') label = `${String(h).padStart(2,'0')}:${String(m).padStart(2,'0')}`
    else if (tick.kind === 'hour')   label = `${String(h).padStart(2,'0')}时`
    else if (tick.kind === 'day')    label = String(d)
    else if (tick.kind === 'week')   label = `${mo}/${d}`
    else if (tick.kind === 'month')  label = `${mo}月`
    else                             label = String(yr)

    ticks.push({ label, yearMark, left: pct(cur), ts: cur.getTime() })

    // 步进到下一刻度
    if      (tick.kind === 'minute') cur.setMinutes(cur.getMinutes() + tick.step)
    else if (tick.kind === 'hour')   cur.setHours(cur.getHours() + tick.step)
    else if (tick.kind === 'day' || tick.kind === 'week') cur.setDate(cur.getDate() + tick.step)
    else if (tick.kind === 'month')  cur.setMonth(cur.getMonth() + tick.step)
    else cur.setFullYear(cur.getFullYear() + tick.step)
  }
  return ticks
}
// 时间进度：(今天 - 开始日) / (结束日 - 开始日) × 100，夹在 0~100
function timeProgress(g: GoalInfo): number {
  if (g.status === 'completed') return 100
  if (!isValidDate(g.startAt) || !isValidDate(g.endAt)) return g.progress
  const now   = Date.now()
  const start = new Date(g.startAt).getTime()
  const end   = new Date(g.endAt).getTime()
  if (end <= start) return g.progress
  return Math.min(100, Math.max(0, Math.round((now - start) / (end - start) * 100)))
}

function progressColor(g: GoalInfo) {
  if (g.status === 'completed') return '#67c23a'
  const tp = timeProgress(g)
  if (tp >= 80) return '#409eff'
  if (tp >= 40) return '#e6a23c'
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
  border-right: 1px solid #ececec;
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
  background: #ececec;
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
  border-right: 1px solid #ececec;
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
.crumb-sep  { color: #c0c4cc; }
.crumb-name { font-weight: 600; color: #303133; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.crumb-back { padding: 0 6px !important; color: #409eff !important; font-size: 12px !important; flex-shrink: 0; }
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

/* gantt-wrap：可滚动区域（overflow-x hidden 防止超长 bar 撑宽容器） */
.gantt-wrap {
  flex: 1;
  overflow-x: hidden;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  min-width: 0;
  cursor: grab;
  user-select: none;
}
.gantt-wrap.is-dragging { cursor: grabbing; }

/* gantt-body：行容器，相对定位供覆盖层使用；overflow:hidden 防止 overlay 随超宽内容扩张 */
.gantt-body {
  position: relative;
  flex: 1;
  overflow: hidden;
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
  border-bottom: 1px solid #ececec;
}
.gantt-label-col {
  width: 200px;
  flex-shrink: 0;
  border-right: 1px solid #ececec;
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
.gantt-row.is-selected { background: rgba(64,158,255,0.08); }
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

/* ── 甘特摘要面板 ─────────────────────────────────────────────────────── */
.gantt-summary-panel {
  flex-shrink: 0;
  border-top: 2px solid #409eff;
  background: #fff;
  padding: 12px 16px;
  box-shadow: 0 -4px 16px rgba(0,0,0,0.08);
}
.gsp-slide-enter-active,
.gsp-slide-leave-active { transition: all 0.2s ease; }
.gsp-slide-enter-from,
.gsp-slide-leave-to { opacity: 0; transform: translateY(12px); }

.gsp-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}
.gsp-avatars { display: flex; gap: 2px; }
.gsp-avatar {
  width: 22px; height: 22px;
  border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  font-size: 10px; color: #fff; font-weight: 600;
}
.gsp-title-block { flex: 1; display: flex; align-items: center; gap: 8px; min-width: 0; }
.gsp-title {
  font-weight: 600; font-size: 14px; color: #303133;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.gsp-close {
  cursor: pointer; color: #909399; font-size: 16px;
  flex-shrink: 0;
  transition: color 0.15s;
}
.gsp-close:hover { color: #303133; }
.gsp-body { display: flex; flex-direction: column; gap: 6px; }
.gsp-stat {
  display: flex; align-items: center; gap: 8px;
  font-size: 13px;
}
.gsp-stat-label { color: #909399; width: 32px; flex-shrink: 0; }
.gsp-stat-val { color: #303133; }
.gsp-desc {
  font-size: 12px; color: #606266;
  background: #f5f7fa; border-radius: 4px;
  padding: 6px 10px; margin-top: 4px;
  white-space: pre-wrap; line-height: 1.5;
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
