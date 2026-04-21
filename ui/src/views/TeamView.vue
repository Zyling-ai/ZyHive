<template>
  <div class="team-view">
    <!-- Header -->
    <div class="page-header">
      <h2>
        📇 通讯录
        <el-text type="info" size="small" style="margin-left:8px;font-weight:400;">
          成员网络 · 联系人档案
        </el-text>
      </h2>
      <div style="display:flex;gap:8px;">
        <el-button v-if="tab === 'graph'" size="small" @click="autoArrange">
          <el-icon><Grid /></el-icon> 整理
        </el-button>
        <el-button size="small" @click="refreshAll">
          <el-icon><Refresh /></el-icon> 刷新
        </el-button>
        <el-button v-if="tab === 'graph'" size="small" type="danger" plain @click="clearAllRelations">
          <el-icon><Delete /></el-icon> 清空关系
        </el-button>
      </div>
    </div>

    <!-- Tab switch -->
    <div class="tab-bar">
      <button :class="['tab-btn', { active: tab === 'graph' }]" @click="tab = 'graph'">
        🧑‍🤝‍🧑 AI 成员网络
        <span class="tab-count">{{ graph.nodes.length }}</span>
      </button>
      <button :class="['tab-btn', { active: tab === 'contacts' }]" @click="tab = 'contacts'">
        👥 联系人
        <span class="tab-count">{{ totalContactCount }}</span>
      </button>
    </div>

    <!-- Graph card -->
    <el-card v-show="tab === 'graph'" v-loading="loading" class="graph-card">
      <!-- Empty: no members -->
      <div v-if="!loading && !graph.nodes.length" class="empty-state">
        <el-icon style="font-size:64px;color:#c0c4cc;display:block;margin:0 auto 16px"><Share /></el-icon>
        <p style="color:#909399;text-align:center;margin:0">暂无成员数据</p>
      </div>

      <!-- Graph -->
      <div v-else class="graph-container" ref="graphContainerRef">
        <!-- Connect-mode banner + node edit panel -->
        <div v-if="selectedNode" style="display:flex;gap:10px;align-items:stretch;margin-bottom:10px;flex-wrap:wrap">
          <div class="connect-banner" style="flex:1;margin-bottom:0">
            <el-icon style="margin-right:6px"><Link /></el-icon>
            已选中 <strong style="margin:0 4px;">{{ nodeName(selectedNode) }}</strong>，点击另一个成员建立关系
            <el-button size="small" text style="margin-left:8px" @click="selectedNode = null">取消</el-button>
          </div>
          <!-- 节点信息编辑（头像颜色） -->
          <div class="node-edit-panel">
            <span style="font-size:12px;color:#606266;margin-right:8px">头像颜色</span>
            <input type="color" v-model="editingColor" class="color-picker-input" title="选择颜色" />
            <el-button size="small" type="primary" :loading="savingColor" @click="saveNodeColor" style="margin-left:8px">保存</el-button>
          </div>
        </div>

        <svg ref="svgRef" :width="svgW" :height="svgH" class="graph-svg"
          @mousemove="onSvgMouseMove"
          @click.self="onSvgBgClick"
          style="display:block;width:100%;overflow:visible;">

          <!-- Grid background + arrowhead markers -->
          <defs>
            <pattern id="smallGrid" width="20" height="20" patternUnits="userSpaceOnUse">
              <path d="M 20 0 L 0 0 0 20" fill="none" stroke="#e8ecf0" stroke-width="0.5"/>
            </pattern>
            <pattern id="grid" width="100" height="100" patternUnits="userSpaceOnUse">
              <rect width="100" height="100" fill="url(#smallGrid)"/>
              <path d="M 100 0 L 0 0 0 100" fill="none" stroke="#dde1e7" stroke-width="1"/>
            </pattern>
            <!-- 上下级：紫色箭头（from=上级 → to=下级） -->
            <marker id="arrow-上下级" markerWidth="10" markerHeight="10"
              refX="9" refY="5" orient="auto" markerUnits="userSpaceOnUse">
              <path d="M1,2 L1,8 L9,5 z" fill="#7c3aed" fill-opacity="0.85"/>
            </marker>
            <!-- 向后兼容旧数据 -->
            <marker id="arrow-上级" markerWidth="10" markerHeight="10"
              refX="9" refY="5" orient="auto" markerUnits="userSpaceOnUse">
              <path d="M1,2 L1,8 L9,5 z" fill="#7c3aed" fill-opacity="0.85"/>
            </marker>
            <marker id="arrow-下级" markerWidth="10" markerHeight="10"
              refX="9" refY="5" orient="auto" markerUnits="userSpaceOnUse">
              <path d="M1,2 L1,8 L9,5 z" fill="#7c3aed" fill-opacity="0.85"/>
            </marker>
          </defs>
          <rect width="100%" height="100%" fill="url(#grid)" rx="0"/>

          <!-- Connection preview line (dashed, from selected node to cursor) — 拖拽中不显示 -->
          <line v-if="selectedNode && !dragState"
            :x1="effPos(selectedNode).x" :y1="effPos(selectedNode).y"
            :x2="mousePos.x" :y2="mousePos.y"
            stroke="#409eff" stroke-width="2" stroke-dasharray="6,4"
            stroke-opacity="0.7" pointer-events="none" />

          <!-- Edges -->
          <g v-for="edge in graph.edges" :key="`${edge.from}|${edge.to}`">
            <!-- Invisible wide hit area for easy clicking -->
            <line
              :x1="edgePt(edge.from, edge.to, 'start').x"
              :y1="edgePt(edge.from, edge.to, 'start').y"
              :x2="edgePt(edge.from, edge.to, 'end').x"
              :y2="edgePt(edge.from, edge.to, 'end').y"
              stroke="transparent" stroke-width="14"
              style="cursor:pointer"
              @click.stop="openEditEdge(edge)" />
            <!-- Visible line（上级/下级带箭头，平级/协作无箭头） -->
            <line
              :x1="edgePt(edge.from, edge.to, 'start').x"
              :y1="edgePt(edge.from, edge.to, 'start').y"
              :x2="edgePt(edge.from, edge.to, 'end').x"
              :y2="edgePt(edge.from, edge.to, 'end').y"
              :stroke="edgeColor(edge.type)"
              :stroke-width="edgeWidth(edge.strength)"
              stroke-opacity="0.7"
              stroke-linecap="round"
              :marker-end="isDirectional(edge.type) ? `url(#arrow-${edge.type})` : undefined"
              pointer-events="none"
              class="graph-edge" />
            <!-- Edge label (relation type) -->
            <text
              :x="(effPos(edge.from).x + effPos(edge.to).x) / 2"
              :y="(effPos(edge.from).y + effPos(edge.to).y) / 2 - 6"
              text-anchor="middle" font-size="10" :fill="edgeColor(edge.type)"
              pointer-events="none" paint-order="stroke" stroke="#f5f7fa" stroke-width="3">
              {{ edge.type }}
            </text>
          </g>

          <!-- Nodes -->
          <g
            v-for="node in graph.nodes"
            :key="node.id"
            :transform="`translate(${effPos(node.id).x}, ${effPos(node.id).y})`"
            :class="['graph-node',
              { 'node-selected': selectedNode === node.id },
              { 'node-target': !!selectedNode && selectedNode !== node.id }]"
            style="cursor:grab"
            @mousedown.stop="(e: MouseEvent) => onNodeMouseDown(e, node.id)"
            @click.stop="() => onNodeClick(node.id)">
            <!-- Selection ring (pulse when selected) -->
            <circle v-if="selectedNode === node.id" r="37"
              fill="none" stroke="#409eff" stroke-width="2.5" stroke-dasharray="7,3"
              class="selection-ring" />
            <!-- Connect-target hover ring -->
            <circle v-else-if="!!selectedNode" r="33"
              fill="rgba(64,158,255,0.06)" stroke="#409eff" stroke-width="1.5" stroke-opacity="0.5" />
            <!-- Shadow -->
            <circle r="30" fill="rgba(0,0,0,0.07)" transform="translate(2,3)" />
            <!-- Main circle -->
            <circle r="28" :fill="nodeColor(node.id)" stroke="#fff" stroke-width="2.5" />
            <!-- Initials -->
            <text text-anchor="middle" dominant-baseline="central" fill="#fff"
              font-weight="700" font-size="15" font-family="system-ui, sans-serif">
              {{ nodeInitial(node.id) }}
            </text>
            <!-- Status dot -->
            <circle cx="20" cy="-20" r="6"
              :fill="node.status === 'running' ? '#67C23A' : '#c0c4cc'"
              stroke="#fff" stroke-width="1.5" />
            <!-- Name -->
            <text text-anchor="middle" y="46" font-size="12" fill="#303133"
              font-family="system-ui, sans-serif" paint-order="stroke"
              stroke="#f5f7fa" stroke-width="3">{{ node.name }}</text>
            <text text-anchor="middle" y="58" font-size="10" fill="#909399"
              font-family="system-ui, monospace">{{ node.id }}</text>
          </g>
        </svg>

        <!-- No-relation hint -->
        <div v-if="!graph.edges.length" class="no-edge-hint">
          点击任意成员选中，再点击另一个成员即可创建关系连线
        </div>
      </div>
    </el-card>

    <!-- Suggestions: 未建立关系的成员对 -->
    <el-card v-show="tab === 'graph'" v-if="suggestions.length" class="suggest-card">
      <div class="suggest-head" @click="suggestOpen = !suggestOpen">
        <span class="suggest-title">
          💡 建议连接
          <span class="suggest-count">{{ suggestions.length }} 组成员尚未建立关系</span>
        </span>
        <el-icon class="suggest-toggle"><component :is="suggestOpen ? 'ArrowUp' : 'ArrowDown'" /></el-icon>
      </div>
      <div v-if="suggestOpen" class="suggest-body">
        <div v-for="(s, idx) in suggestions.slice(0, 5)" :key="idx" class="suggest-row">
          <span class="suggest-pair">
            <span class="suggest-name">{{ s.fromName }}</span>
            <span class="suggest-arrow">↔</span>
            <span class="suggest-name">{{ s.toName }}</span>
          </span>
          <div class="suggest-actions">
            <el-button size="small" @click="openCreateRel(s.from, s.to)">自定义…</el-button>
            <el-button size="small" type="primary" :loading="suggestSaving === `${s.from}|${s.to}`"
              @click="quickConnect(s.from, s.to)">
              建立平级关系
            </el-button>
          </div>
        </div>
        <div v-if="suggestions.length > 5" class="suggest-more">
          还有 {{ suggestions.length - 5 }} 组未显示（先处理前几组即可）
        </div>
      </div>
    </el-card>

    <!-- Legend -->
    <el-card v-show="tab === 'graph'" v-if="graph.nodes.length" class="legend-card">
      <div class="legend">
        <span class="legend-title">布局规则：</span>
        <span class="legend-item"><el-icon><ArrowUp /></el-icon> 上方 = 上级（箭头指下）</span>
        <span class="legend-item">— 同层 = 平级/协作</span>
        <span class="legend-item"><el-icon><ArrowDown /></el-icon> 下方 = 下级</span>
        <span class="legend-divider">|</span>
        <span class="legend-title">线粗：</span>
        <span v-for="(w, s) in strengthWidths" :key="s" class="legend-item">
          <svg width="28" height="8"><line x1="0" y1="4" x2="28" y2="4" stroke="#64748b" :stroke-width="w" stroke-linecap="round" /></svg>
          {{ s }}
        </span>
        <span class="legend-divider">|</span>
        <span class="legend-item">
          <svg width="12" height="12"><circle cx="6" cy="6" r="5" fill="#67C23A" /></svg> 运行中
        </span>
        <span class="legend-item">
          <svg width="12" height="12"><circle cx="6" cy="6" r="5" fill="#c0c4cc" /></svg> 空闲
        </span>
      </div>
    </el-card>

    <!-- ══ Contacts Tab — 跨 agent 联系人聚合视图 ═══════════════════════════ -->
    <div v-show="tab === 'contacts'" class="contacts-pane">
      <!-- Filter bar -->
      <div class="contact-filter-bar">
        <el-input
          v-model="contactSearch"
          placeholder="搜索 姓名 / ID / 标签 / 来源"
          size="default"
          style="max-width: 320px"
          clearable
        >
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <el-radio-group v-model="contactSource" size="small">
          <el-radio-button value="">全部来源</el-radio-button>
          <el-radio-button value="feishu">飞书</el-radio-button>
          <el-radio-button value="telegram">Telegram</el-radio-button>
          <el-radio-button value="web">Web</el-radio-button>
        </el-radio-group>
        <el-radio-group v-model="contactAgentFilter" size="small">
          <el-radio-button value="">全部成员</el-radio-button>
          <el-radio-button
            v-for="ag in graph.nodes"
            :key="ag.id"
            :value="ag.id"
          >{{ ag.name }}</el-radio-button>
        </el-radio-group>
      </div>

      <!-- Empty -->
      <el-card v-if="!contactsLoading && !filteredContacts.length" class="contacts-empty">
        <div style="padding: 40px 20px; text-align: center; color: #94a3b8;">
          <div style="font-size: 40px; margin-bottom: 10px;">📭</div>
          <div>{{ contacts.length ? '当前筛选无结果' : '还没有联系人。对话一次就会自动出现。' }}</div>
        </div>
      </el-card>

      <!-- Contact rows -->
      <div v-else class="contact-list" v-loading="contactsLoading">
        <div
          v-for="c in filteredContacts"
          :key="c.agentId + '|' + c.id"
          class="contact-row"
          @click="openContactDrawer(c)"
        >
          <div class="contact-avatar" :style="{ background: avatarColor(c.displayName || c.id) }">
            {{ (c.displayName || c.id).slice(0, 1) }}
          </div>
          <div class="contact-main">
            <div class="contact-name-row">
              <span class="contact-name">{{ c.displayName || c.id }}</span>
              <el-tag size="small" :type="sourceTagType(c.source)" effect="plain" style="margin-left: 6px">
                {{ sourceLabel(c.source) }}
              </el-tag>
              <el-tag v-if="c.isOwner" size="small" type="warning" style="margin-left: 4px">主人</el-tag>
            </div>
            <div class="contact-meta">
              <span class="contact-id">{{ c.id }}</span>
              <span v-if="c.tags && c.tags.length" class="contact-tags">
                <span v-for="t in c.tags" :key="t" class="contact-tag">#{{ t }}</span>
              </span>
              <span class="contact-msgcount">💬 {{ c.msgCount }}</span>
              <span v-if="c.lastSeenAt" class="contact-lastseen">最后 {{ formatLastSeen(c.lastSeenAt) }}</span>
            </div>
            <div class="contact-agent-chip">
              <el-text type="info" size="small">
                📍 {{ agentNameById[c.agentId] || c.agentId }} 的联系人
              </el-text>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ── Contact Drawer (edit profile) ── -->
    <el-drawer v-model="contactDrawerOpen" :title="drawerTitle" direction="rtl" size="540px" destroy-on-close>
      <div v-if="drawerContact" class="contact-drawer">
        <div class="cd-head">
          <div class="cd-avatar" :style="{ background: avatarColor(drawerContact.displayName || drawerContact.id) }">
            {{ (drawerContact.displayName || drawerContact.id).slice(0, 1) }}
          </div>
          <div class="cd-title">
            <el-input
              v-model="drawerContact.displayName"
              placeholder="显示名"
              size="default"
              style="max-width: 280px"
            />
            <div class="cd-sub">
              <el-tag size="small" :type="sourceTagType(drawerContact.source)" effect="plain">
                {{ sourceLabel(drawerContact.source) }}
              </el-tag>
              <span class="cd-id">{{ drawerContact.id }}</span>
            </div>
          </div>
        </div>

        <div class="cd-field">
          <label>标签</label>
          <div class="cd-tags">
            <el-tag
              v-for="(t, i) in (drawerContact.tags || [])"
              :key="t + i"
              closable
              size="small"
              @close="(drawerContact.tags || []).splice(i, 1)"
              style="margin-right: 4px; margin-bottom: 4px"
            >{{ t }}</el-tag>
            <el-input
              v-if="addingTag"
              v-model="newTagText"
              size="small"
              style="width: 110px; margin-right: 4px"
              @keyup.enter="commitTag"
              @blur="commitTag"
              placeholder="回车确认"
              ref="tagInputRef"
            />
            <el-button v-else size="small" @click="beginAddTag">+ 标签</el-button>
            <el-button
              v-for="preset in presetTags"
              :key="preset"
              size="small"
              plain
              @click="addPresetTag(preset)"
              style="margin-right: 4px; margin-bottom: 4px"
            >#{{ preset }}</el-button>
          </div>
        </div>

        <div class="cd-field">
          <el-checkbox v-model="drawerContact.isOwner">这是「主人」本人在该渠道的身份</el-checkbox>
          <el-text type="info" size="small" style="display:block;margin-top:4px">
            勾选后，AI 收到这位发送的消息时使用 owner-profile 档案，不会重复注入本联系人档案。
          </el-text>
        </div>

        <div class="cd-field">
          <label>档案（Markdown）</label>
          <el-input
            v-model="drawerContact.body"
            type="textarea"
            :rows="12"
            placeholder="# 姓名&#10;&#10;## 事实&#10;- 公司/角色&#10;- ...&#10;&#10;## 偏好（AI 观察）&#10;-&#10;&#10;## 待跟进&#10;-"
            style="font-family: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, monospace; font-size: 13px;"
          />
          <el-text type="info" size="small" style="display:block;margin-top:4px">
            AI 也会通过 <code>network_note</code> 工具自动追加。你可随时手改。
          </el-text>
        </div>

        <div class="cd-actions">
          <el-button @click="contactDrawerOpen = false">取消</el-button>
          <el-button type="danger" plain :loading="drawerSaving" @click="deleteContact">删除</el-button>
          <el-button type="primary" :loading="drawerSaving" @click="saveContact">保存</el-button>
        </div>
      </div>
    </el-drawer>

    <!-- ── Create Relation Dialog ── -->
    <el-dialog v-model="createRelDialog" title="建立关系" width="460px" :close-on-click-modal="false">
      <RelTypeForm
        :from-name="nodeName(relForm.from)"
        :to-name="nodeName(relForm.to)"
        v-model:type="relForm.type"
        v-model:strength="relForm.strength"
        v-model:desc="relForm.desc"
        @swap="() => { const t = relForm.from; relForm.from = relForm.to; relForm.to = t }"
      />
      <template #footer>
        <el-button @click="createRelDialog = false">取消</el-button>
        <el-button type="primary" :loading="savingRel" @click="saveCreateRel">建立</el-button>
      </template>
    </el-dialog>

    <!-- ── Edit Relation Dialog ── -->
    <el-dialog v-model="editRelDialog" title="编辑关系" width="460px" :close-on-click-modal="false">
      <RelTypeForm
        :from-name="nodeName(editForm.from)"
        :to-name="nodeName(editForm.to)"
        v-model:type="editForm.type"
        v-model:strength="editForm.strength"
        v-model:desc="editForm.desc"
        @swap="() => { const t = editForm.from; editForm.from = editForm.to; editForm.to = t }"
      />
      <template #footer>
        <el-button type="danger" plain :loading="savingRel" @click="confirmDeleteEdge">删除关系</el-button>
        <el-button @click="editRelDialog = false">取消</el-button>
        <el-button type="primary" :loading="savingRel" @click="saveEditRel">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, reactive, watch, nextTick } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import { relationsApi, agents as agentsApi, networkApi, type TeamGraph, type TeamGraphEdge, type TeamGraphNode, type ContactSummary, type Contact } from '../api'
import RelTypeForm from '../components/RelTypeForm.vue'

const svgRef = ref<SVGSVGElement>()
const graphContainerRef = ref<HTMLDivElement>()
const loading = ref(false)
const graph = ref<TeamGraph>({ nodes: [], edges: [] })

// ── Layout constants ───────────────────────────────────────────────────────
const svgW = ref(860)   // updated by ResizeObserver
const NODE_R = 28
const LEVEL_H = 160
const PAD_TOP = 90
const PAD_X = 80

const strengthWidths: Record<string, number> = { '核心': 4, '常用': 2.5, '偶尔': 1.5 }

const typeColors: Record<string, string> = {
  '上下级': '#7c3aed',
  // 向后兼容旧数据（Graph() 已将其转换，但保留以防万一）
  '上级': '#7c3aed', '下级': '#7c3aed',
  '平级协作': '#409eff', '支持': '#67c23a', '其他': '#909399',
}
function edgeColor(type: string) { return typeColors[type] ?? '#94a3b8' }
function isDirectional(type: string) { return type === '上下级' || type === '上级' || type === '下级' }

// ── Hierarchy layout ───────────────────────────────────────────────────────
function computeLevels(nodes: TeamGraphNode[], edges: TeamGraphEdge[]): Record<string, number> {
  const levels: Record<string, number> = {}
  nodes.forEach(n => { levels[n.id] = 0 })
  const maxLevel = nodes.length + 1  // 防止循环依赖导致层级无限增长
  const maxIter = nodes.length + 2
  for (let iter = 0; iter < maxIter; iter++) {
    let changed = false
    for (const edge of edges) {
      const lf = Math.min(levels[edge.from] ?? 0, maxLevel)
      const lt = levels[edge.to] ?? 0
      if (edge.type === '上下级') {
        const want = Math.min(lf + 1, maxLevel)
        if (lt < want) { levels[edge.to] = want; changed = true }
      } else if (edge.type === '上级') {
        const want = lf - 1
        if (lt > want) { levels[edge.to] = want; changed = true }
      } else if (edge.type === '下级') {
        const want = Math.min(lf + 1, maxLevel)
        if (lt < want) { levels[edge.to] = want; changed = true }
      }
    }
    if (!changed) break
  }
  const vals = Object.values(levels)
  const minL = vals.length ? Math.min(...vals) : 0
  nodes.forEach(n => { levels[n.id] = (levels[n.id] ?? 0) - minL })
  return levels
}

const levelMap = computed(() => computeLevels(graph.value.nodes, graph.value.edges))

// ── 建议连接：未建立关系的 agent 对 ────────────────────────────────────────
const suggestOpen = ref(true)
const suggestSaving = ref<string>('')

const suggestions = computed<{ from: string; to: string; fromName: string; toName: string }[]>(() => {
  const nodes = graph.value.nodes
  if (nodes.length < 2) return []
  const edgeSet = new Set<string>()
  for (const e of graph.value.edges) {
    // 用 "小id|大id" 做无向归一化 key
    const a = e.from < e.to ? e.from : e.to
    const b = e.from < e.to ? e.to : e.from
    edgeSet.add(`${a}|${b}`)
  }
  const out: { from: string; to: string; fromName: string; toName: string }[] = []
  for (let i = 0; i < nodes.length; i++) {
    for (let j = i + 1; j < nodes.length; j++) {
      const na = nodes[i]!
      const nb = nodes[j]!
      const key = na.id < nb.id ? `${na.id}|${nb.id}` : `${nb.id}|${na.id}`
      if (!edgeSet.has(key)) {
        out.push({ from: na.id, to: nb.id, fromName: na.name, toName: nb.name })
      }
    }
  }
  return out
})

// 一键建立平级协作关系（默认 strength=常用, desc 空）
async function quickConnect(from: string, to: string) {
  const key = `${from}|${to}`
  if (suggestSaving.value) return
  suggestSaving.value = key
  try {
    await relationsApi.putEdge(from, to, '平级协作', '常用', '')
    ElMessage.success('关系已建立（平级协作）')
    await loadGraph()
  } catch {
    ElMessage.error('建立失败')
  } finally {
    suggestSaving.value = ''
  }
}

const svgH = computed(() => {
  const maxLevel = Object.values(levelMap.value).reduce((m, v) => Math.max(m, v), 0)
  return Math.max(600, PAD_TOP + maxLevel * LEVEL_H + 160)
})

const posMap = computed<Record<string, { x: number; y: number }>>(() => {
  const nodes = graph.value.nodes
  const levels = levelMap.value
  const w = svgW.value
  const byLevel: Record<number, string[]> = {}
  nodes.forEach(n => {
    const lv = levels[n.id] ?? 0
    if (!byLevel[lv]) byLevel[lv] = []
    byLevel[lv].push(n.id)
  })
  const map: Record<string, { x: number; y: number }> = {}
  for (const [lvStr, ids] of Object.entries(byLevel)) {
    const lv = Number(lvStr)
    const y = PAD_TOP + lv * LEVEL_H
    const usableW = w - PAD_X * 2
    const gap = ids.length > 1 ? usableW / (ids.length - 1) : 0
    ids.forEach((id, i) => {
      map[id] = {
        x: Math.round(ids.length === 1 ? w / 2 : PAD_X + i * gap),
        y: Math.round(y),
      }
    })
  }
  return map
})

// ── Drag ──────────────────────────────────────────────────────────────────
interface DragState {
  id: string
  startClientX: number; startClientY: number
  // 鼠标点击时，节点中心相对于鼠标的 SVG 坐标偏移（保持跟手）
  offsetX: number; offsetY: number
  moved: boolean
}
const dragPositions = ref<Record<string, { x: number; y: number }>>({})
const dragState = ref<DragState | null>(null)
const mousePos = ref({ x: 400, y: PAD_TOP })
// 记录上一次是否为拖拽结束（mouseup 先于 click 触发，需跨事件传递）
const lastDragId = ref<string | null>(null)

/** Effective position: drag override → computed layout */
function effPos(id: string): { x: number; y: number } {
  return dragPositions.value[id] ?? posMap.value[id] ?? { x: svgW.value / 2, y: PAD_TOP }
}

// ── Document-level drag (works even when pointer leaves SVG) ──────────────
function onNodeMouseDown(e: MouseEvent, nodeId: string) {
  e.preventDefault()
  lastDragId.value = null  // 每次按下都重置
  const nodePos = effPos(nodeId)
  const mouseInSvg = clientToSvg(e.clientX, e.clientY)
  dragState.value = {
    id: nodeId,
    startClientX: e.clientX, startClientY: e.clientY,
    // 节点中心与鼠标点击位置在 SVG 坐标系中的偏移，保持完全跟手
    offsetX: nodePos.x - mouseInSvg.x,
    offsetY: nodePos.y - mouseInSvg.y,
    moved: false,
  }
  document.addEventListener('mousemove', onDocMouseMove)
  document.addEventListener('mouseup', onDocMouseUp)
}

/** Convert client coords → SVG coordinate space.
 *  使用 getScreenCTM().inverse() 精确处理任意缩放/平移/DPR，
 *  比手动 rect+scale 更准确，不受 CSS width:100% 影响。 */
function clientToSvg(clientX: number, clientY: number): { x: number; y: number } {
  const el = svgRef.value
  if (!el) return { x: clientX, y: clientY }
  const ctm = el.getScreenCTM()
  if (!ctm) {
    // fallback: 手动换算
    const rect = el.getBoundingClientRect()
    const sx = rect.width  > 0 ? svgW.value / rect.width  : 1
    const sy = rect.height > 0 ? svgH.value / rect.height : 1
    return { x: (clientX - rect.left) * sx, y: (clientY - rect.top) * sy }
  }
  const pt = el.createSVGPoint()
  pt.x = clientX
  pt.y = clientY
  const r = pt.matrixTransform(ctm.inverse())
  return { x: r.x, y: r.y }
}

function onDocMouseMove(e: MouseEvent) {
  const svgPos = clientToSvg(e.clientX, e.clientY)
  mousePos.value = svgPos

  if (!dragState.value) return

  const dx = e.clientX - dragState.value.startClientX
  const dy = e.clientY - dragState.value.startClientY
  if (Math.abs(dx) > 4 || Math.abs(dy) > 4) dragState.value.moved = true
  if (!dragState.value.moved) return

  // 直接用 SVG 坐标 + 初始偏移，完全跟手，无需缩放计算
  const newX = svgPos.x + dragState.value.offsetX
  const newY = svgPos.y + dragState.value.offsetY
  // 只限左/上边界，右/下不设硬墙（SVG overflow:visible 自然溢出）
  const minX = NODE_R + 4, maxX = Infinity
  const minY = NODE_R + 4, maxY = Infinity
  dragPositions.value = {
    ...dragPositions.value,
    [dragState.value.id]: {
      x: Math.round(Math.max(minX, Math.min(maxX, newX))),
      y: Math.round(Math.max(minY, Math.min(maxY, newY))),
    },
  }
}

function onDocMouseUp() {
  if (dragState.value?.moved) {
    lastDragId.value = dragState.value.id  // 标记刚刚拖拽结束的节点
  }
  dragState.value = null
  document.removeEventListener('mousemove', onDocMouseMove)
  document.removeEventListener('mouseup', onDocMouseUp)
}

function onSvgMouseMove(e: MouseEvent) {
  if (!dragState.value) mousePos.value = clientToSvg(e.clientX, e.clientY)
}

function onSvgBgClick() { selectedNode.value = null }

// ── Connection creation + node edit ──────────────────────────────────────
const selectedNode = ref<string | null>(null)
const editingColor = ref('#409EFF')
const savingColor = ref(false)

watch(selectedNode, (id) => {
  if (!id) return
  const node = graph.value.nodes.find(n => n.id === id)
  editingColor.value = node?.avatarColor ?? nodeColor(id)
})

async function saveNodeColor() {
  if (!selectedNode.value || savingColor.value) return
  savingColor.value = true
  try {
    await agentsApi.update(selectedNode.value, { avatarColor: editingColor.value })
    ElMessage.success('头像颜色已更新')
    await loadGraph()  // 刷新图谱使颜色生效
  } catch { ElMessage.error('保存失败') }
  finally { savingColor.value = false }
}

function onNodeClick(nodeId: string) {
  // dragState 在 mouseup 时已清空，用 lastDragId 判断是否为拖拽结束
  if (lastDragId.value === nodeId) {
    lastDragId.value = null
    return  // 拖拽结束，忽略此次 click
  }
  lastDragId.value = null
  if (!selectedNode.value) {
    selectedNode.value = nodeId
    return
  }
  if (selectedNode.value === nodeId) {
    selectedNode.value = null
    return
  }
  const from = selectedNode.value
  selectedNode.value = null
  openCreateRel(from, nodeId)
}

// ── Edge helpers ──────────────────────────────────────────────────────────
function edgePt(fromId: string, toId: string, end: 'start' | 'end') {
  const a = effPos(fromId)
  const b = effPos(toId)
  const dx = b.x - a.x
  const dy = b.y - a.y
  const len = Math.sqrt(dx * dx + dy * dy) || 1
  const r = NODE_R + 3
  if (end === 'start') return { x: a.x + (dx / len) * r, y: a.y + (dy / len) * r }
  return { x: b.x - (dx / len) * r, y: b.y - (dy / len) * r }
}
function edgeWidth(strength: string) { return strengthWidths[strength] ?? 1.5 }

// ── Node helpers ──────────────────────────────────────────────────────────
const palette = ['#409EFF', '#67C23A', '#E6A23C', '#F56C6C', '#7C3AED', '#0891B2', '#B45309', '#64748B']
function nodeColor(id: string) {
  // 优先使用成员配置的头像颜色
  const node = graph.value.nodes.find(n => n.id === id)
  if (node?.avatarColor) return node.avatarColor
  // fallback: hash-based
  let h = 0
  for (let i = 0; i < id.length; i++) h = id.charCodeAt(i) + ((h << 5) - h)
  return palette[Math.abs(h) % palette.length] ?? '#409EFF'
}
function nodeInitial(id: string) { return (id || '?').charAt(0).toUpperCase() }
function nodeName(id: string) { return graph.value.nodes.find(n => n.id === id)?.name ?? id }

// ── Auto arrange ──────────────────────────────────────────────────────────
function autoArrange() {
  dragPositions.value = {}
  ElMessage.success('已重置为自动布局')
}

// ── Create relation dialog ─────────────────────────────────────────────────
const createRelDialog = ref(false)
const relForm = reactive({ from: '', to: '', type: '平级协作', strength: '常用', desc: '' })
const savingRel = ref(false)

function openCreateRel(from: string, to: string) {
  // Check if relation already exists
  const exists = graph.value.edges.some(
    e => (e.from === from && e.to === to) || (e.from === to && e.to === from)
  )
  if (exists) {
    const edge = graph.value.edges.find(
      e => (e.from === from && e.to === to) || (e.from === to && e.to === from)
    )!
    openEditEdge(edge)
    return
  }
  relForm.from = from; relForm.to = to
  relForm.type = '平级协作'; relForm.strength = '常用'; relForm.desc = ''
  createRelDialog.value = true
}

async function saveCreateRel() {
  if (savingRel.value) return
  savingRel.value = true
  try {
    await relationsApi.putEdge(relForm.from, relForm.to, relForm.type, relForm.strength, relForm.desc)
    ElMessage.success('关系已建立')
    createRelDialog.value = false
    await loadGraph()
  } catch { ElMessage.error('保存失败') }
  finally { savingRel.value = false }
}

// ── Edit relation dialog ───────────────────────────────────────────────────
const editRelDialog = ref(false)
const editForm = reactive({ from: '', to: '', type: '平级协作', strength: '常用', desc: '' })
// 记录打开编辑弹窗时的原始方向，用于翻转后清除旧边
let originalEdgeFrom = ''
let originalEdgeTo = ''

function openEditEdge(edge: TeamGraphEdge) {
  editForm.from = edge.from; editForm.to = edge.to
  editForm.type = edge.type; editForm.strength = edge.strength; editForm.desc = edge.label
  originalEdgeFrom = edge.from
  originalEdgeTo = edge.to
  editRelDialog.value = true
}

async function saveEditRel() {
  if (savingRel.value) return
  savingRel.value = true
  try {
    const directionChanged = editForm.from !== originalEdgeFrom || editForm.to !== originalEdgeTo
    if (directionChanged) {
      // 方向翻转：先删掉原来的边，再建新边（避免两条边并存）
      await relationsApi.deleteEdge(originalEdgeFrom, originalEdgeTo)
    }
    await relationsApi.putEdge(editForm.from, editForm.to, editForm.type, editForm.strength, editForm.desc)
    ElMessage.success('关系已更新')
    editRelDialog.value = false
    await loadGraph()
  } catch { ElMessage.error('保存失败') }
  finally { savingRel.value = false }
}

async function confirmDeleteEdge() {
  try {
    await ElMessageBox.confirm(`删除 ${nodeName(editForm.from)} ↔ ${nodeName(editForm.to)} 的关系？`, '删除关系', {
      confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning',
    })
  } catch { return }
  savingRel.value = true
  try {
    await relationsApi.deleteEdge(editForm.from, editForm.to)
    ElMessage.success('关系已删除')
    editRelDialog.value = false
    await loadGraph()
  } catch { ElMessage.error('删除失败') }
  finally { savingRel.value = false }
}

// ── Load ───────────────────────────────────────────────────────────────────
async function loadGraph() {
  loading.value = true
  try {
    const res = await relationsApi.graph()
    graph.value = res.data
  } catch { ElMessage.error('加载图谱失败') }
  finally { loading.value = false }
}

async function clearAllRelations() {
  try {
    await ElMessageBox.confirm('将清空所有成员的关系，不可恢复。确认吗？', '清空所有关系', {
      confirmButtonText: '确认清空', cancelButtonText: '取消', type: 'warning',
    })
  } catch { return }
  try {
    await relationsApi.clearAll()
    ElMessage.success('已清空所有成员关系')
    await loadGraph()
  } catch { ElMessage.error('清空失败') }
}

// ══ Contacts tab ═══════════════════════════════════════════════════════════
type ContactWithOwner = ContactSummary & { agentId: string }

const tab = ref<'graph' | 'contacts'>('graph')
const contacts = ref<ContactWithOwner[]>([])
const contactsLoading = ref(false)
const contactSearch = ref('')
const contactSource = ref('')
const contactAgentFilter = ref('')

const agentNameById = computed<Record<string, string>>(() => {
  const m: Record<string, string> = {}
  for (const n of graph.value.nodes) m[n.id] = n.name
  return m
})

const totalContactCount = computed(() => contacts.value.length)

const filteredContacts = computed(() => {
  const q = contactSearch.value.trim().toLowerCase()
  return contacts.value.filter(c => {
    if (contactSource.value && c.source !== contactSource.value) return false
    if (contactAgentFilter.value && c.agentId !== contactAgentFilter.value) return false
    if (!q) return true
    const hay = (
      (c.displayName || '') + ' ' +
      c.id + ' ' +
      (c.tags || []).join(' ') + ' ' +
      c.source
    ).toLowerCase()
    return hay.includes(q)
  })
})

async function loadContacts() {
  contactsLoading.value = true
  try {
    const nodes = graph.value.nodes
    const results: ContactWithOwner[] = []
    await Promise.all(nodes.map(async n => {
      try {
        const res = await networkApi.list(n.id)
        for (const c of (res.data?.contacts || [])) {
          results.push({ ...c, agentId: n.id })
        }
      } catch { /* ignore per-agent failure */ }
    }))
    contacts.value = results
  } finally {
    contactsLoading.value = false
  }
}

async function refreshAll() {
  await loadGraph()
  if (tab.value === 'contacts') await loadContacts()
}

watch(tab, (t) => {
  if (t === 'contacts' && contacts.value.length === 0) loadContacts()
})

// ── Contact drawer ────────────────────────────────────────────────────────
const contactDrawerOpen = ref(false)
const drawerContact = ref<Contact & { agentId: string } | null>(null)
const drawerSaving = ref(false)
const presetTags = ['家人', '同事', '客户', '合作伙伴', '朋友', 'AI 成员']
const addingTag = ref(false)
const newTagText = ref('')
const tagInputRef = ref<any>(null)

const drawerTitle = computed(() => {
  if (!drawerContact.value) return '联系人详情'
  return `✏️ ${drawerContact.value.displayName || drawerContact.value.id}`
})

async function openContactDrawer(c: ContactWithOwner) {
  try {
    const res = await networkApi.get(c.agentId, c.id)
    drawerContact.value = { ...res.data, agentId: c.agentId, tags: res.data.tags || [] } as any
    contactDrawerOpen.value = true
  } catch {
    ElMessage.error('读取联系人失败')
  }
}

function beginAddTag() {
  addingTag.value = true
  newTagText.value = ''
  nextTick(() => tagInputRef.value?.focus?.())
}
function commitTag() {
  const t = newTagText.value.trim()
  if (t && drawerContact.value) {
    if (!drawerContact.value.tags) drawerContact.value.tags = []
    if (!drawerContact.value.tags.includes(t)) drawerContact.value.tags.push(t)
  }
  addingTag.value = false
  newTagText.value = ''
}
function addPresetTag(t: string) {
  if (!drawerContact.value) return
  if (!drawerContact.value.tags) drawerContact.value.tags = []
  if (!drawerContact.value.tags.includes(t)) drawerContact.value.tags.push(t)
}

async function saveContact() {
  if (!drawerContact.value) return
  const c = drawerContact.value
  drawerSaving.value = true
  try {
    await networkApi.update(c.agentId, c.id, {
      displayName: c.displayName,
      tags: c.tags || [],
      body: c.body,
      isOwner: !!c.isOwner,
    })
    ElMessage.success('已保存')
    contactDrawerOpen.value = false
    await loadContacts()
  } catch {
    ElMessage.error('保存失败')
  } finally {
    drawerSaving.value = false
  }
}

async function deleteContact() {
  if (!drawerContact.value) return
  const c = drawerContact.value
  try {
    await ElMessageBox.confirm(`删除 ${c.displayName || c.id}？此操作只移除该 agent 的档案。`, '确认删除', {
      confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning',
    })
  } catch { return }
  drawerSaving.value = true
  try {
    await networkApi.delete(c.agentId, c.id)
    ElMessage.success('已删除')
    contactDrawerOpen.value = false
    await loadContacts()
  } catch {
    ElMessage.error('删除失败')
  } finally {
    drawerSaving.value = false
  }
}

// Helpers
function sourceLabel(s: string): string {
  switch (s) {
    case 'feishu': return '飞书'
    case 'telegram': return 'Telegram'
    case 'web': return 'Web'
    case 'panel': return '面板'
    default: return s || '其它'
  }
}
function sourceTagType(s: string): 'success' | 'primary' | 'warning' | 'info' | 'danger' {
  switch (s) {
    case 'feishu': return 'primary'
    case 'telegram': return 'success'
    case 'web': return 'warning'
    default: return 'info'
  }
}
function avatarColor(seed: string): string {
  // Deterministic pastel color from string hash.
  let h = 0
  for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) >>> 0
  const hue = h % 360
  return `hsl(${hue}deg 55% 62%)`
}
function formatLastSeen(iso: string): string {
  try {
    const d = new Date(iso)
    const delta = Date.now() - d.getTime()
    if (delta < 60_000) return '刚刚'
    if (delta < 3600_000) return Math.floor(delta / 60_000) + '分钟前'
    if (delta < 86400_000) return Math.floor(delta / 3600_000) + '小时前'
    return d.toLocaleDateString()
  } catch { return iso }
}

let ro: ResizeObserver | null = null
onMounted(() => {
  loadGraph()
  if (graphContainerRef.value) {
    ro = new ResizeObserver(entries => {
      const w = entries[0]?.contentRect.width
      if (w && w > 100) svgW.value = Math.floor(w)
      // ⚠️ Do NOT reset dragPositions here — that would cancel user drags
    })
    ro.observe(graphContainerRef.value)
  }
})

onUnmounted(() => {
  ro?.disconnect()
  document.removeEventListener('mousemove', onDocMouseMove)
  document.removeEventListener('mouseup', onDocMouseUp)
})
</script>

<style scoped>
.team-view { padding: 0; }
.page-header {
  display: flex; align-items: center; justify-content: space-between; margin-bottom: 20px;
}
.page-header h2 { margin: 0; font-size: 20px; font-weight: 700; color: #303133; }
.graph-card { margin-bottom: 16px; }
.empty-state { padding: 60px 0; }
.graph-container { position: relative; display: flex; flex-direction: column; overflow: visible; width: 100%; }

/* Connect-mode banner */
.connect-banner {
  display: flex; align-items: center; padding: 8px 16px;
  background: #ecf5ff; color: #409eff; font-size: 13px;
  border-radius: 6px;
}
/* Node edit panel */
.node-edit-panel {
  display: flex; align-items: center;
  background: #f5f7fa; border: 1px solid #ececec;
  border-radius: 6px; padding: 6px 14px;
  font-size: 13px; flex-shrink: 0;
}
.color-picker-input {
  width: 32px; height: 26px; border: 1px solid #dcdfe6;
  border-radius: 4px; padding: 0 2px; cursor: pointer;
  background: none;
}

.graph-svg { display: block; max-width: 100%; }

.graph-edge { transition: stroke-opacity 0.15s; }
.graph-node { transition: opacity 0.12s; }
.graph-node:hover { opacity: 0.88; }
.node-target { cursor: crosshair !important; }

/* Selection ring spin animation */
.selection-ring {
  animation: spin-ring 4s linear infinite;
  transform-origin: center;
}
@keyframes spin-ring {
  from { stroke-dashoffset: 0; }
  to   { stroke-dashoffset: -40; }
}

.no-edge-hint { text-align: center; color: #c0c4cc; font-size: 13px; padding: 8px 0 16px; }

/* Relation dialogs */
.rel-pair {
  display: flex; align-items: center; gap: 10px; justify-content: center;
  background: #f5f7fa; border-radius: 8px; padding: 12px;
}
.rel-node { font-weight: 600; font-size: 14px; color: #303133; }

/* Legend */
.suggest-card {
  margin-bottom: 12px;
  border: 1px solid #ececec;
}
.suggest-head {
  display: flex; align-items: center; justify-content: space-between;
  cursor: pointer; user-select: none;
}
.suggest-title { font-weight: 600; font-size: 14px; color: #303133; }
.suggest-count {
  margin-left: 8px; font-weight: 400; font-size: 12px; color: #64748b;
}
.suggest-toggle { color: #94a3b8; font-size: 14px; }
.suggest-body { margin-top: 10px; display: flex; flex-direction: column; gap: 8px; }
.suggest-row {
  display: flex; align-items: center; justify-content: space-between;
  padding: 10px 12px;
  background: #fafafa;
  border-radius: 6px;
  border: 1px solid transparent;
  transition: border-color 0.15s;
}
.suggest-row:hover { border-color: #e2e8f0; }
.suggest-pair { display: flex; align-items: center; gap: 8px; font-size: 13px; }
.suggest-name { font-weight: 500; color: #1e293b; }
.suggest-arrow { color: #94a3b8; font-size: 14px; }
.suggest-actions { display: flex; gap: 6px; }
.suggest-more {
  margin-top: 4px; padding: 6px 4px; font-size: 12px; color: #94a3b8; text-align: center;
}

.legend-card { padding: 0; }

/* ── Tabs ──────────────────────────────────────────────────────────────── */
.tab-bar {
  display: flex; gap: 2px;
  margin-bottom: 16px;
  border-bottom: 1px solid #ececec;
}
.tab-btn {
  padding: 10px 18px;
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  font-size: 14px;
  color: #64748b;
  cursor: pointer;
  transition: all 0.15s;
  display: flex; align-items: center; gap: 6px;
  margin-bottom: -1px;
}
.tab-btn:hover { color: #18181b; }
.tab-btn.active {
  color: #18181b;
  border-bottom-color: #18181b;
  font-weight: 500;
}
.tab-count {
  font-size: 11px;
  color: #94a3b8;
  background: #f1f5f9;
  padding: 1px 7px;
  border-radius: 9px;
}
.tab-btn.active .tab-count { background: #e0e7ff; color: #4338ca; }

/* ── Contacts pane ─────────────────────────────────────────────────────── */
.contacts-pane { margin-top: 4px; }
.contact-filter-bar {
  display: flex; gap: 10px; align-items: center; flex-wrap: wrap;
  margin-bottom: 12px;
}
.contacts-empty { border: 1px solid #ececec; }
.contact-list {
  display: flex; flex-direction: column; gap: 1px;
  background: #ececec;
  border-radius: 6px;
  overflow: hidden;
  border: 1px solid #ececec;
}
.contact-row {
  display: flex; gap: 12px;
  padding: 12px 16px;
  background: #fff;
  cursor: pointer;
  transition: background 0.1s;
}
.contact-row:hover { background: #fafafa; }
.contact-avatar {
  width: 40px; height: 40px;
  border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  color: #fff;
  font-weight: 600;
  font-size: 16px;
  flex-shrink: 0;
}
.contact-main { flex: 1; min-width: 0; }
.contact-name-row { display: flex; align-items: center; gap: 4px; margin-bottom: 3px; }
.contact-name { font-weight: 500; font-size: 14px; color: #18181b; }
.contact-meta {
  display: flex; gap: 10px; font-size: 12px; color: #94a3b8;
  flex-wrap: wrap; align-items: center;
}
.contact-id { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.contact-tags { display: inline-flex; gap: 4px; }
.contact-tag { color: #6366f1; font-weight: 500; }
.contact-msgcount { color: #64748b; }
.contact-lastseen { color: #94a3b8; }
.contact-agent-chip { margin-top: 3px; font-size: 11px; }

/* ── Contact drawer ────────────────────────────────────────────────────── */
.contact-drawer { padding: 0 8px; }
.cd-head {
  display: flex; gap: 12px; align-items: center;
  padding-bottom: 16px;
  border-bottom: 1px solid #ececec;
  margin-bottom: 16px;
}
.cd-avatar {
  width: 52px; height: 52px;
  border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  color: #fff;
  font-weight: 600;
  font-size: 20px;
  flex-shrink: 0;
}
.cd-title { flex: 1; min-width: 0; }
.cd-sub { display: flex; gap: 8px; align-items: center; margin-top: 6px; }
.cd-id {
  font-size: 12px; color: #94a3b8;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}
.cd-field { margin-bottom: 16px; }
.cd-field > label {
  display: block;
  font-size: 13px; font-weight: 500; color: #334155;
  margin-bottom: 6px;
}
.cd-tags { display: flex; flex-wrap: wrap; align-items: center; gap: 2px; }
.cd-actions {
  display: flex; gap: 8px; justify-content: flex-end;
  padding-top: 16px;
  border-top: 1px solid #ececec;
}
.legend { display: flex; align-items: center; flex-wrap: wrap; gap: 14px; font-size: 13px; color: #606266; }
.legend-title { font-weight: 600; color: #303133; }
.legend-item { display: flex; align-items: center; gap: 5px; }
.legend-divider { color: #dcdfe6; }

@media (max-width: 768px) {
  /* Toolbar: wrap buttons */
  .team-page > div:first-child { flex-wrap: wrap; gap: 8px; }

  /* Connect banner: wrap text */
  .connect-banner { flex-wrap: wrap; gap: 4px; font-size: 12px; padding: 6px 10px; }

  /* Node edit panel: wrap */
  .node-edit-panel { flex-wrap: wrap; gap: 6px; font-size: 12px; padding: 6px 8px; }

  /* Graph SVG: allow horizontal scroll */
  .graph-container { overflow-x: auto; -webkit-overflow-scrolling: touch; }

  /* Legend: smaller */
  .legend { gap: 8px; font-size: 12px; }

  /* Rel dialog: full width */
  .rel-pair { flex-direction: column; }
}
</style>
