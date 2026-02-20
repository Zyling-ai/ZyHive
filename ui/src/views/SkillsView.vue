<template>
  <div class="skills-page">
    <!-- ── 顶部 ── -->
    <div class="page-header">
      <div class="header-left">
        <h2 class="page-title">
          <el-icon><Aim /></el-icon> 技能库
        </h2>
        <div class="header-stats">
          <span class="stat-item">共 <b>{{ allRows.length }}</b> 个技能</span>
          <span class="stat-sep">·</span>
          <span class="stat-item">已启用 <b>{{ allRows.filter(r => r.skill.enabled).length }}</b></span>
          <span class="stat-sep">·</span>
          <span class="stat-item">涉及 <b>{{ activeAgentIds.size }}</b> 个成员</span>
        </div>
      </div>
      <div class="header-acts">
        <el-select
          v-model="filterAgentId"
          placeholder="全部成员"
          clearable
          size="small"
          style="width:150px"
        >
          <el-option v-for="ag in agents" :key="ag.id" :label="ag.name" :value="ag.id" />
        </el-select>
        <el-input
          v-model="filterKeyword"
          placeholder="搜索技能…"
          clearable
          size="small"
          style="width:180px"
          :prefix-icon="Search"
        />
        <el-button :loading="loading" size="small" circle @click="loadAll">
          <el-icon><Refresh /></el-icon>
        </el-button>
      </div>
    </div>

    <!-- ── 内容区 ── -->
    <div v-if="loading" style="padding:40px"><el-skeleton :rows="3" animated /></div>
    <el-empty v-else-if="filteredRows.length === 0" description="暂无技能" style="padding:60px 0" />

    <div v-else class="skills-grid">
      <div v-for="row in filteredRows" :key="`${row.agentId}-${row.skill.id}`" class="skill-card">
        <!-- 成员标签 -->
        <div class="card-agent">
          <el-avatar :size="18" style="font-size:10px;flex-shrink:0;background:#409eff">
            {{ row.agentName.charAt(0) }}
          </el-avatar>
          <span class="agent-name">{{ row.agentName }}</span>
          <el-switch
            :model-value="row.skill.enabled"
            size="small"
            style="margin-left:auto"
            @change="(v: boolean) => toggleSkill(row, v)"
          />
        </div>

        <!-- 技能主体 -->
        <div class="card-body">
          <div class="skill-icon">
            <span v-if="row.skill.icon">{{ row.skill.icon }}</span>
            <el-icon v-else size="20" color="#c0c4cc"><Tools /></el-icon>
          </div>
          <div class="skill-info">
            <div class="skill-name">{{ row.skill.name }}</div>
            <div class="skill-id">{{ row.skill.id }}</div>
            <el-tag v-if="row.skill.category" size="small" type="primary" effect="plain" style="margin-top:4px;font-size:11px">
              {{ row.skill.category }}
            </el-tag>
          </div>
        </div>

        <div v-if="row.skill.description" class="card-desc">{{ row.skill.description }}</div>

        <!-- 操作 -->
        <div class="card-acts">
          <el-button size="small" link type="primary" @click="goEdit(row)">
            <el-icon><Edit /></el-icon> 编辑
          </el-button>
          <el-button size="small" link type="primary" @click="openCopy(row)">
            <el-icon><CopyDocument /></el-icon> 复制到…
          </el-button>
          <el-popconfirm
            :title="`从「${row.agentName}」删除「${row.skill.name}」？`"
            @confirm="removeSkill(row)"
          >
            <template #reference>
              <el-button size="small" link type="danger"><el-icon><Delete /></el-icon></el-button>
            </template>
          </el-popconfirm>
        </div>
      </div>
    </div>

    <!-- ── 复制对话框 ── -->
    <el-dialog v-model="copyDialog.visible" title="复制技能到其他成员" width="420px">
      <div v-if="copyDialog.source" class="copy-source">
        <b>{{ copyDialog.source.agentName }}</b>
        <el-icon style="margin:0 4px;color:#c0c4cc"><ArrowRight /></el-icon>
        {{ copyDialog.source.skill.icon }} {{ copyDialog.source.skill.name }}
        <el-tag size="small" type="info" effect="plain" style="margin-left:6px;font-family:monospace;font-size:11px">
          {{ copyDialog.source.skill.id }}
        </el-tag>
      </div>
      <el-form label-width="80px" size="small" style="margin-top:16px">
        <el-form-item label="目标成员" required>
          <el-select v-model="copyDialog.targetAgentId" placeholder="选择目标成员" style="width:100%">
            <el-option
              v-for="ag in agents.filter(a => a.id !== copyDialog.source?.agentId)"
              :key="ag.id"
              :label="ag.name"
              :value="ag.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="新技能 ID">
          <el-input v-model="copyDialog.newSkillId" :placeholder="copyDialog.source?.skill.id || '留空使用原 ID'" />
          <div style="font-size:11px;color:#c0c4cc;margin-top:3px">目标成员已有同 ID 技能时需修改</div>
        </el-form-item>
        <el-form-item label="新名称">
          <el-input v-model="copyDialog.newName" :placeholder="copyDialog.source?.skill.name || '留空使用原名称'" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="copyDialog.visible = false">取消</el-button>
        <el-button type="primary" :loading="copyDialog.copying" @click="doCopy">
          <el-icon><CopyDocument /></el-icon> 复制
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import {
  agents as agentsApi,
  agentSkills as skillsApi,
  files as filesApi,
  type AgentInfo,
  type AgentSkillMeta,
} from '../api'

const router = useRouter()

// ── State ──────────────────────────────────────────────────────────────────
const agents = ref<AgentInfo[]>([])
const loading = ref(false)
const filterAgentId = ref('')
const filterKeyword = ref('')

interface SkillRow {
  agentId: string
  agentName: string
  skill: AgentSkillMeta
}
const allRows = ref<SkillRow[]>([])

// ── Computed ───────────────────────────────────────────────────────────────
const activeAgentIds = computed(() => new Set(allRows.value.map(r => r.agentId)))

const filteredRows = computed(() => {
  let rows = allRows.value
  if (filterAgentId.value) rows = rows.filter(r => r.agentId === filterAgentId.value)
  if (filterKeyword.value.trim()) {
    const kw = filterKeyword.value.trim().toLowerCase()
    rows = rows.filter(r =>
      r.skill.name.toLowerCase().includes(kw) ||
      r.skill.id.toLowerCase().includes(kw) ||
      (r.skill.category || '').toLowerCase().includes(kw)
    )
  }
  return rows
})

// ── Load ───────────────────────────────────────────────────────────────────
async function loadAll() {
  loading.value = true
  try {
    const agRes = await agentsApi.list()
    agents.value = agRes.data || []

    const results = await Promise.allSettled(
      agents.value.map(ag =>
        skillsApi.list(ag.id).then(res =>
          (res.data || []).map((sk): SkillRow => ({
            agentId: ag.id,
            agentName: ag.name,
            skill: sk,
          }))
        )
      )
    )

    const rows: SkillRow[] = []
    for (const r of results) {
      if (r.status === 'fulfilled') rows.push(...r.value)
    }
    rows.sort((a, b) =>
      a.agentName.localeCompare(b.agentName) || a.skill.name.localeCompare(b.skill.name)
    )
    allRows.value = rows
  } catch (e: any) {
    ElMessage.error('加载失败: ' + (e.message || ''))
  } finally {
    loading.value = false
  }
}

// ── Toggle ─────────────────────────────────────────────────────────────────
async function toggleSkill(row: SkillRow, enabled: boolean) {
  try {
    await skillsApi.update(row.agentId, row.skill.id, { enabled })
    row.skill.enabled = enabled
  } catch { ElMessage.error('操作失败') }
}

// ── Remove ─────────────────────────────────────────────────────────────────
async function removeSkill(row: SkillRow) {
  try {
    await skillsApi.remove(row.agentId, row.skill.id)
    allRows.value = allRows.value.filter(
      r => !(r.agentId === row.agentId && r.skill.id === row.skill.id)
    )
    ElMessage.success('已删除')
  } catch { ElMessage.error('删除失败') }
}

// ── Edit ───────────────────────────────────────────────────────────────────
function goEdit(row: SkillRow) {
  router.push(`/agents/${row.agentId}?tab=skills&skill=${row.skill.id}`)
}

// ── Copy ───────────────────────────────────────────────────────────────────
const copyDialog = ref({
  visible: false,
  source: null as SkillRow | null,
  targetAgentId: '',
  newSkillId: '',
  newName: '',
  copying: false,
})

function openCopy(row: SkillRow) {
  copyDialog.value = {
    visible: true,
    source: row,
    targetAgentId: '',
    newSkillId: '',
    newName: '',
    copying: false,
  }
}

async function doCopy() {
  const { source, targetAgentId, newSkillId, newName } = copyDialog.value
  if (!source || !targetAgentId) { ElMessage.warning('请选择目标成员'); return }
  copyDialog.value.copying = true
  try {
    // 读取 SKILL.md 内容
    let promptContent = ''
    try {
      const mdRes = await filesApi.read(source.agentId, `skills/${source.skill.id}/SKILL.md`)
      promptContent = mdRes.data?.content || ''
    } catch { /* 无 SKILL.md 正常 */ }

    const targetId = newSkillId.trim() || source.skill.id
    const targetName = newName.trim() || source.skill.name

    await skillsApi.create(targetAgentId, {
      meta: {
        id: targetId,
        name: targetName,
        icon: source.skill.icon || '',
        category: source.skill.category || '',
        description: source.skill.description || '',
        version: source.skill.version || '1.0.0',
        enabled: source.skill.enabled,
        source: 'local',
        installedAt: '',
      },
      promptContent,
    })

    const targetAgent = agents.value.find(a => a.id === targetAgentId)
    ElMessage.success(`已复制「${targetName}」到「${targetAgent?.name}」`)
    copyDialog.value.visible = false
    await loadAll()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '复制失败')
  } finally {
    copyDialog.value.copying = false
  }
}

onMounted(loadAll)
</script>

<style scoped>
.skills-page { padding: 24px; }

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 24px;
  flex-wrap: wrap;
  gap: 12px;
}
.header-left { display: flex; align-items: center; gap: 16px; flex-wrap: wrap; }
.page-title {
  margin: 0; font-size: 18px; font-weight: 600; color: #303133;
  display: flex; align-items: center; gap: 6px;
}
.header-stats { display: flex; align-items: center; gap: 6px; font-size: 13px; color: #909399; }
.stat-sep { color: #dcdfe6; }
.header-acts { display: flex; align-items: center; gap: 10px; }

/* Grid */
.skills-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(270px, 1fr));
  gap: 14px;
}
.skill-card {
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  transition: box-shadow 0.15s, border-color 0.15s;
}
.skill-card:hover { box-shadow: 0 4px 16px rgba(0,0,0,0.08); border-color: #c0c4cc; }

.card-agent {
  display: flex; align-items: center; gap: 6px;
  font-size: 12px; padding-bottom: 8px;
  border-bottom: 1px solid #f5f5f5;
}
.agent-name { font-weight: 500; color: #606266; }

.card-body { display: flex; align-items: flex-start; gap: 10px; }
.skill-icon { font-size: 22px; flex-shrink: 0; width: 28px; text-align: center; padding-top: 2px; }
.skill-info { flex: 1; min-width: 0; }
.skill-name { font-size: 14px; font-weight: 600; color: #303133; }
.skill-id { font-size: 11px; color: #c0c4cc; font-family: monospace; }

.card-desc {
  font-size: 12px; color: #909399; line-height: 1.5;
  display: -webkit-box; -webkit-line-clamp: 2;
  -webkit-box-orient: vertical; overflow: hidden;
}

.card-acts {
  display: flex; align-items: center; gap: 4px;
  border-top: 1px solid #f0f0f0; padding-top: 8px;
}

/* Copy dialog */
.copy-source {
  display: flex; align-items: center; flex-wrap: wrap; gap: 4px;
  background: #f5f7fa; border-radius: 6px; padding: 10px 12px;
  font-size: 13px;
}
</style>
