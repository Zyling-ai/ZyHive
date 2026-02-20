<template>
  <div class="skills-page">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px">
      <h2 style="margin: 0"><el-icon style="vertical-align:-2px;margin-right:6px"><Aim /></el-icon>Skills</h2>
      <el-button type="primary" @click="showInstall = true">
        <el-icon><Plus /></el-icon> 安装 Skill
      </el-button>
    </div>

    <el-row :gutter="16">
      <el-col :span="8" v-for="s in list" :key="s.id">
        <el-card shadow="hover" style="margin-bottom: 16px">
          <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 8px">
            <div>
              <div style="font-weight: 600; font-size: 16px">{{ s.name }}</div>
              <el-text type="info" size="small">v{{ s.version }}</el-text>
            </div>
            <el-switch v-model="s.enabled" size="small" />
          </div>
          <el-text type="info" size="small" style="display: block; margin-bottom: 12px">
            {{ s.description || '无描述' }}
          </el-text>
          <el-text type="info" size="small" style="display: block; margin-bottom: 12px">
            路径: {{ s.path || '-' }}
          </el-text>
          <el-button size="small" type="danger" @click="deleteSkill(s)">卸载</el-button>
        </el-card>
      </el-col>
    </el-row>

    <el-empty v-if="list.length === 0" description="暂无已安装 Skills" />

    <!-- Install Dialog -->
    <el-dialog v-model="showInstall" title="安装 Skill" width="520px">
      <el-form :model="installForm" label-width="80px">
        <el-form-item label="ID" required>
          <el-input v-model="installForm.id" placeholder="skill-id" />
        </el-form-item>
        <el-form-item label="名称" required>
          <el-input v-model="installForm.name" placeholder="Skill 名称" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="installForm.description" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item label="路径/URL">
          <el-input v-model="installForm.path" placeholder="本地路径或远程 URL" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showInstall = false">取消</el-button>
        <el-button type="primary" @click="installSkill" :loading="installing">安装</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { skills as skillsApi, type SkillEntry } from '../api'

const list = ref<SkillEntry[]>([])
const showInstall = ref(false)
const installing = ref(false)
const installForm = reactive({ id: '', name: '', description: '', path: '' })

onMounted(loadList)

async function loadList() {
  try {
    const res = await skillsApi.list()
    list.value = res.data
  } catch {}
}

async function installSkill() {
  if (!installForm.id || !installForm.name) {
    ElMessage.warning('请填写 ID 和名称')
    return
  }
  installing.value = true
  try {
    await skillsApi.install({ ...installForm })
    ElMessage.success('安装成功')
    showInstall.value = false
    Object.assign(installForm, { id: '', name: '', description: '', path: '' })
    loadList()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '安装失败')
  } finally {
    installing.value = false
  }
}

async function deleteSkill(s: SkillEntry) {
  try {
    await ElMessageBox.confirm(`确定卸载 "${s.name}"？`, '确认卸载', { type: 'warning' })
    await skillsApi.delete(s.id)
    ElMessage.success('已卸载')
    loadList()
  } catch {}
}
</script>
