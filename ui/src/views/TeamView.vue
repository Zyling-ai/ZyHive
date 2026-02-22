<template>
  <div class="team-view">
    <!-- Header -->
    <div class="page-header">
      <h2>团队关系图谱</h2>
      <div style="display:flex;gap:8px;">
        <el-button size="small" @click="autoArrange">
          <el-icon><Grid /></el-icon> 整理
        </el-button>
        <el-button size="small" @click="loadGraph">
          <el-icon><Refresh /></el-icon> 刷新
        </el-button>
        <el-button size="small" type="danger" plain @click="clearAllRelations">
          <el-icon><Delete /></el-icon> 清空关系
        </el-button>
      </div>
    </div>

    <!-- Graph card -->
    <el-card v-loading="loading" class="graph-card">
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

    <!-- Legend -->
    <el-card v-if="graph.nodes.length" class="legend-card">
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
import { ref, computed, onMounted, onUnmounted, reactive, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { relationsApi, agents as agentsApi, type TeamGraph, type TeamGraphEdge, type TeamGraphNode } from '../api'
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
  background: #f5f7fa; border: 1px solid #e4e7ed;
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
.legend-card { padding: 0; }
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
