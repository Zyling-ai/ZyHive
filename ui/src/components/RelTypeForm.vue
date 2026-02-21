<template>
  <!-- Member pair header -->
  <div class="rel-pair">
    <span class="rel-node">{{ fromName }}</span>
    <div style="display:flex;flex-direction:column;align-items:center;gap:2px;flex-shrink:0">
      <el-icon style="color:#c0c4cc;font-size:16px"><ArrowRight /></el-icon>
      <!-- 上下级时显示交换方向按钮 -->
      <el-button
        v-if="type === '上下级'"
        size="small" text type="primary"
        style="padding:0 4px;font-size:11px;height:18px;min-height:0"
        @click="$emit('swap')"
        title="交换上下级方向">
        ⇄ 翻转
      </el-button>
    </div>
    <span class="rel-node">{{ toName }}</span>
  </div>

  <!-- Relation type cards (2×2) -->
  <div class="type-grid">
    <div
      v-for="opt in typeOptions"
      :key="opt.value"
      class="type-card"
      :class="{ active: type === opt.value }"
      @click="$emit('update:type', opt.value)">
      <div class="type-card-top">
        <span class="type-tag" :style="{ background: opt.color + '18', color: opt.color }">{{ opt.value }}</span>
      </div>
      <div class="type-desc">{{ opt.desc }}</div>
    </div>
  </div>

  <!-- Strength + Desc -->
  <el-form label-width="72px" style="margin-top:16px">
    <el-form-item label="关系强度">
      <el-radio-group :model-value="strength" @update:model-value="(v: string) => $emit('update:strength', v)">
        <el-radio-button value="核心">核心</el-radio-button>
        <el-radio-button value="常用">常用</el-radio-button>
        <el-radio-button value="偶尔">偶尔</el-radio-button>
      </el-radio-group>
    </el-form-item>
    <el-form-item label="说明">
      <el-input
        :model-value="desc"
        @update:model-value="(v: string) => $emit('update:desc', v)"
        placeholder="可选备注"
      />
    </el-form-item>
  </el-form>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ArrowRight } from '@element-plus/icons-vue'

const props = defineProps<{
  fromName: string
  toName: string
  type: string
  strength: string
  desc: string
}>()

defineEmits<{
  'update:type': [v: string]
  'update:strength': [v: string]
  'update:desc': [v: string]
  'swap': []
}>()

const typeOptions = computed(() => [
  {
    value: '上下级',
    color: '#7c3aed',
    desc: `${props.fromName} 是 ${props.toName} 的上级，箭头指向下级`,
  },
  {
    value: '平级协作',
    color: '#409eff',
    desc: `${props.fromName} 与 ${props.toName} 并列合作，地位平等`,
  },
  {
    value: '支持',
    color: '#67c23a',
    desc: `${props.fromName} 为 ${props.toName} 提供支持和辅助`,
  },
  {
    value: '其他',
    color: '#909399',
    desc: `${props.fromName} 与 ${props.toName} 之间的其他关系`,
  },
])
</script>

<style scoped>
.rel-pair {
  display: flex;
  align-items: center;
  gap: 12px;
  justify-content: center;
  background: #f5f7fa;
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 16px;
}
.rel-node {
  font-weight: 600;
  font-size: 14px;
  color: #303133;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Type card grid */
.type-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}

.type-card {
  border: 1.5px solid #e4e7ed;
  border-radius: 8px;
  padding: 12px 14px;
  cursor: pointer;
  transition: all 0.15s;
  background: #fff;
}
.type-card:hover {
  border-color: #409eff;
  background: #f0f9ff;
}
.type-card.active {
  border-color: #409eff;
  background: #ecf5ff;
  box-shadow: 0 0 0 2px rgba(64,158,255,0.2);
}

.type-card-top {
  margin-bottom: 6px;
}
.type-tag {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 600;
}
.type-desc {
  font-size: 12px;
  color: #606266;
  line-height: 1.5;
}
.type-card.active .type-desc {
  color: #303133;
}
</style>
