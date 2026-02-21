<template>
  <el-container class="agent-detail">
    <el-header class="detail-header">
      <div class="header-left">
        <el-button :icon="ArrowLeft" @click="$router.push('/agents')" circle />
        <h2>{{ agent?.name || '...' }}</h2>
        <el-tag :type="statusType(agent?.status)">{{ statusLabel(agent?.status) }}</el-tag>
      </div>
      <el-text type="info">{{ agent?.model }}</el-text>
    </el-header>

    <el-main>
      <el-tabs v-model="activeTab" type="border-card">
        <!-- Tab 1: Chat with session sidebar -->
        <el-tab-pane label="对话" name="chat">
          <div class="chat-layout">
            <!-- Session History Sidebar -->
            <div class="session-sidebar">
              <div class="session-sidebar-header">
                <span class="sidebar-title">历史对话</span>
                <el-button size="small" type="primary" plain @click="newSession" :icon="Plus">新建</el-button>
              </div>

              <div class="session-list" v-loading="sessionsLoading">
                <div
                  v-for="s in agentSessions"
                  :key="s.id"
                  :class="['session-item', { active: activeSessionId === s.id }]"
                  @click="resumeSession(s)"
                >
                  <div class="session-item-title">{{ s.title || '新对话' }}</div>
                  <div class="session-item-meta">
                    <span>{{ formatRelative(s.lastAt) }}</span>
                    <el-tag size="small" type="info" effect="plain" style="font-size: 10px; padding: 0 4px">
                      {{ s.messageCount }} 条
                    </el-tag>
                    <el-tag
                      v-if="s.tokenEstimate > 60000"
                      size="small"
                      type="warning"
                      effect="plain"
                      style="font-size: 10px; padding: 0 4px"
                    >~{{ Math.round(s.tokenEstimate / 1000) }}k</el-tag>
                  </div>
                </div>

                <div v-if="!sessionsLoading && !agentSessions.length" class="session-empty">
                  还没有对话记录
                </div>
              </div>

              <!-- @ 其他成员面板 -->
              <div class="at-panel">
                <el-button
                  size="small"
                  plain
                  class="at-toggle-btn"
                  @click="toggleAtPanel"
                >
                  <span class="at-icon">@</span> 其他成员
                </el-button>

                <!-- 内联转发表单 -->
                <div v-if="showAtPanel" class="at-form">
                  <el-select
                    v-model="atTargetId"
                    placeholder="选择成员"
                    size="small"
                    style="width: 100%; margin-bottom: 6px"
                    @change="onAtAgentSelect"
                  >
                    <el-option
                      v-for="a in otherAgents"
                      :key="a.id"
                      :label="a.name"
                      :value="a.id"
                    />
                  </el-select>

                  <el-input
                    v-model="atMessage"
                    type="textarea"
                    :rows="3"
                    placeholder="输入要转发的消息…"
                    size="small"
                    style="margin-bottom: 6px"
                  />

                  <el-button
                    type="primary"
                    size="small"
                    style="width: 100%"
                    :loading="atSending"
                    :disabled="!atTargetId || !atMessage.trim()"
                    @click="sendAtMessage"
                  >
                    转发
                  </el-button>
                </div>
              </div>
            </div>
          

            <!-- Chat Area -->
            <div class="chat-area">
              <AiChat
                ref="aiChatRef"
                :agent-id="agentId"
                :scenario="'agent-detail'"
                :welcome-message="`你好！我是 **${agent?.name || 'AI'}**，有什么可以帮你的？`"
                height="calc(100vh - 145px)"
                :show-thinking="true"
                @session-change="onSessionChange"
              />
            </div>
          </div>
        </el-tab-pane>

        <!-- Tab 2: Identity & Soul -->
        <el-tab-pane label="身份 & 灵魂" name="identity">
          <!-- 基本设置卡片 -->
          <el-card style="margin-bottom: 16px;">
            <template #header>
              <span style="font-weight: 600;">基本设置</span>
            </template>
            <el-form label-width="80px" size="default">
              <el-form-item label="使用模型">
                <el-select
                  v-model="agentModelId"
                  placeholder="选择模型"
                  style="width: 280px; margin-right: 10px"
                >
                  <el-option
                    v-for="m in modelList"
                    :key="m.id"
                    :label="m.name || m.model"
                    :value="m.id"
                  >
                    <div style="display:flex; justify-content:space-between; width:100%">
                      <span>{{ m.name || m.model }}</span>
                      <span style="color:#999; font-size:12px">{{ m.provider }}</span>
                    </div>
                  </el-option>
                </el-select>
                <el-button
                  type="primary"
                  :loading="agentModelSaving"
                  @click="saveAgentModel"
                  :disabled="!agentModelId"
                >保存</el-button>
                <el-text v-if="agent?.model" type="info" style="margin-left:12px; font-size:12px">
                  当前：{{ agent.model }}
                </el-text>
              </el-form-item>
            </el-form>
          </el-card>

          <el-row :gutter="20">
            <el-col :span="12">
              <el-card header="IDENTITY.md">
                <el-input
                  v-model="identityContent"
                  type="textarea"
                  :rows="15"
                  @blur="saveFile('IDENTITY.md', identityContent)"
                />
              </el-card>
            </el-col>
            <el-col :span="12">
              <el-card header="SOUL.md">
                <el-input
                  v-model="soulContent"
                  type="textarea"
                  :rows="15"
                  @blur="saveFile('SOUL.md', soulContent)"
                />
              </el-card>
            </el-col>
          </el-row>
        </el-tab-pane>

        <!-- Tab 3: Relations -->
        <el-tab-pane label="关系" name="relations">
          <el-row :gutter="20">
            <!-- Left: form + table -->
            <el-col :span="14">
              <!-- Add relation form -->
              <el-card style="margin-bottom: 16px;">
                <template #header>
                  <span style="font-weight: 600;">添加关系</span>
                </template>
                <el-form :model="newRelation" label-position="top" size="default">
                  <el-row :gutter="12">
                    <el-col :span="10">
                      <el-form-item label="关联成员">
                        <el-select
                          v-model="newRelation.agentId"
                          placeholder="选择系统成员"
                          filterable
                          style="width: 100%;"
                          @change="onRelationAgentChange"
                        >
                          <el-option
                            v-for="a in otherAgents"
                            :key="a.id"
                            :label="a.name"
                            :value="a.id"
                          >
                            <div style="display: flex; align-items: center; gap: 8px;">
                              <div style="width: 24px; height: 24px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: 11px; color: #fff; flex-shrink: 0;"
                                :style="{ background: avatarColor(a.id) }">
                                {{ a.name.charAt(0) }}
                              </div>
                              <span>{{ a.name }}</span>
                            </div>
                          </el-option>
                        </el-select>
                      </el-form-item>
                    </el-col>
                    <el-col :span="7">
                      <el-form-item label="关系类型">
                        <el-select v-model="newRelation.relationType" style="width: 100%;">
                          <el-option label="上级" value="上级" />
                          <el-option label="下级" value="下级" />
                          <el-option label="平级协作" value="平级协作" />
                          <el-option label="支持" value="支持" />
                        </el-select>
                      </el-form-item>
                    </el-col>
                    <el-col :span="7">
                      <el-form-item label="协作程度">
                        <el-select v-model="newRelation.strength" style="width: 100%;">
                          <el-option label="核心" value="核心" />
                          <el-option label="常用" value="常用" />
                          <el-option label="偶尔" value="偶尔" />
                        </el-select>
                      </el-form-item>
                    </el-col>
                  </el-row>
                  <el-row :gutter="12">
                    <el-col :span="18">
                      <el-form-item label="说明（选填）">
                        <el-input v-model="newRelation.desc" placeholder="简要描述这段关系..." />
                      </el-form-item>
                    </el-col>
                    <el-col :span="6">
                      <el-form-item label=" ">
                        <el-button
                          type="primary"
                          style="width: 100%;"
                          :disabled="!newRelation.agentId || !newRelation.relationType || !newRelation.strength"
                          :loading="relationsSaving"
                          @click="addRelation"
                        >
                          添加
                        </el-button>
                      </el-form-item>
                    </el-col>
                  </el-row>
                </el-form>
              </el-card>

              <!-- Relations table -->
              <el-card>
                <template #header>
                  <span style="font-weight: 600;">已有关系 <el-badge :value="parsedRelations.length" type="info" style="margin-left: 4px;" /></span>
                </template>
                <div v-if="parsedRelations.length === 0" style="text-align: center; color: #c0c4cc; padding: 32px 0; font-size: 14px;">
                  暂无关系，请在上方添加
                </div>
                <el-table v-else :data="parsedRelations" size="small" style="width: 100%;">
                  <el-table-column label="成员" min-width="120">
                    <template #default="{ row }">
                      <div style="display: flex; align-items: center; gap: 8px;">
                        <div style="width: 28px; height: 28px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: 12px; color: #fff; flex-shrink: 0;"
                          :style="{ background: avatarColor(row.agentId) }">
                          {{ row.agentName.charAt(0) }}
                        </div>
                        <span style="font-size: 13px;">{{ row.agentName }}</span>
                      </div>
                    </template>
                  </el-table-column>
                  <el-table-column label="类型" width="100">
                    <template #default="{ row }">
                      <el-tag :type="relationTypeColor(row.relationType)" size="small">{{ row.relationType }}</el-tag>
                    </template>
                  </el-table-column>
                  <el-table-column label="程度" width="80">
                    <template #default="{ row }">
                      <el-tag :type="strengthColor(row.strength)" size="small" effect="plain">{{ row.strength }}</el-tag>
                    </template>
                  </el-table-column>
                  <el-table-column label="说明" min-width="120" show-overflow-tooltip>
                    <template #default="{ row }">
                      <span style="font-size: 13px; color: #606266;">{{ row.desc || '—' }}</span>
                    </template>
                  </el-table-column>
                  <el-table-column label="操作" width="70" fixed="right">
                    <template #default="{ $index }">
                      <el-button
                        type="danger"
                        link
                        size="small"
                        @click="deleteRelation($index)"
                      >删除</el-button>
                    </template>
                  </el-table-column>
                </el-table>
              </el-card>
            </el-col>

            <!-- Right: preview cards -->
            <el-col :span="10">
              <el-card header="关系预览">
                <div v-if="parsedRelations.length === 0" style="text-align: center; color: #c0c4cc; padding: 40px 0;">
                  暂无关系数据
                </div>
                <div v-else class="relations-list">
                  <div v-for="row in parsedRelations" :key="row.agentId" class="relation-card">
                    <div class="relation-avatar" :style="{ background: avatarColor(row.agentId) }">
                      {{ row.agentName.charAt(0).toUpperCase() }}
                    </div>
                    <div class="relation-info">
                      <div class="relation-name">{{ row.agentName }}</div>
                      <div class="relation-tags">
                        <el-tag :type="relationTypeColor(row.relationType)" size="small">{{ row.relationType }}</el-tag>
                        <el-tag :type="strengthColor(row.strength)" size="small" effect="plain">{{ row.strength }}</el-tag>
                      </div>
                      <div class="relation-desc">{{ row.desc }}</div>
                    </div>
                  </div>
                </div>
              </el-card>
            </el-col>
          </el-row>
        </el-tab-pane>

        <!-- Tab 4: Memory Tree -->
        <el-tab-pane label="记忆" name="memory">
          <!-- Memory Config Card -->
          <el-card style="margin-bottom: 16px;" shadow="never">
            <template #header>
              <div style="display: flex; align-items: center; justify-content: space-between;">
                <span style="font-weight: 600;">自动记忆</span>
                <el-switch
                  v-model="memCfg.enabled"
                  active-text="已开启"
                  inactive-text="已关闭"
                  @change="saveMemConfig"
                />
              </div>
            </template>
            <el-form :model="memCfg" label-position="top" size="small" :disabled="!memCfg.enabled">
              <el-row :gutter="16">
                <el-col :span="6">
                  <el-form-item label="整理频率">
                    <el-select v-model="memCfg.schedule" style="width: 100%;">
                      <el-option label="每小时" value="hourly" />
                      <el-option label="每6小时" value="every6h" />
                      <el-option label="每天" value="daily" />
                      <el-option label="每周" value="weekly" />
                    </el-select>
                  </el-form-item>
                </el-col>
                <el-col :span="5">
                  <el-form-item label="每个会话保留轮数">
                    <el-input-number
                      v-model="memCfg.keepTurns"
                      :min="1"
                      :max="20"
                      style="width: 100%;"
                    />
                  </el-form-item>
                </el-col>
                <el-col :span="13">
                  <el-form-item label="记录重点（留空则自动）">
                    <el-input
                      v-model="memCfg.focusHint"
                      placeholder="例如：记录数学解题步骤和用户常见错误"
                    />
                  </el-form-item>
                </el-col>
              </el-row>
              <div style="display: flex; gap: 8px; margin-top: 4px;">
                <el-button type="primary" size="small" :loading="memCfgSaving" @click="saveMemConfig">
                  保存设置
                </el-button>
                <el-button size="small" :loading="memConsolidating" @click="consolidateNow">
                  立即整理
                </el-button>
                <el-text type="info" size="small" style="align-self: center; margin-left: 4px;">
                  整理后：LLM提炼摘要写入 MEMORY.md，每个会话只保留最近 {{ memCfg.keepTurns }} 轮对话
                </el-text>
              </div>
            </el-form>
          </el-card>

          <!-- Consolidation Log Card -->
          <el-card style="margin-bottom: 16px;" shadow="never">
            <template #header>
              <div style="display: flex; align-items: center; justify-content: space-between;">
                <span style="font-weight: 600;">整理日志</span>
                <el-button size="small" text @click="loadMemLogs" :loading="memLogsLoading">刷新</el-button>
              </div>
            </template>
            <div v-if="memLogs.length === 0 && !memLogsLoading" style="text-align: center; color: #c0c4cc; padding: 16px 0; font-size: 13px;">
              暂无运行记录，点击「立即整理」触发第一次
            </div>
            <el-table v-else :data="memLogs.slice(0, 20)" size="small">
              <el-table-column label="时间" width="160">
                <template #default="{ row }">
                  <span style="font-size: 12px;">{{ formatTimestamp(row.timestamp) }}</span>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="72">
                <template #default="{ row }">
                  <el-tag :type="row.status === 'ok' ? 'success' : 'danger'" size="small">{{ row.status }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="结果" min-width="200" show-overflow-tooltip>
                <template #default="{ row }">
                  <span style="font-size: 12px; color: #606266;">{{ row.message || '—' }}</span>
                </template>
              </el-table-column>
            </el-table>
          </el-card>

          <div class="memory-toolbar" style="margin-bottom: 12px; display: flex; gap: 8px;">
            <el-button type="primary" size="small" @click="showNewMemoryFile = true">
              <el-icon><Plus /></el-icon> 新建文件
            </el-button>
            <el-button size="small" @click="showDailyEntry = true">
              <el-icon><EditPen /></el-icon> 添加日志
            </el-button>
            <el-button size="small" @click="loadMemoryTree">
              <el-icon><Refresh /></el-icon> 刷新
            </el-button>
          </div>
          <el-row :gutter="16">
            <!-- Left: tree navigator (30%) -->
            <el-col :span="7">
              <el-card header="记忆目录" shadow="hover">
                <el-tree
                  :data="memoryTreeData"
                  :props="{ label: 'name', children: 'children', isLeaf: (d: any) => !d.isDir }"
                  @node-click="handleMemoryNodeClick"
                  highlight-current
                  default-expand-all
                  :expand-on-click-node="false"
                >
                  <template #default="{ data }">
                    <span style="display: flex; align-items: center; gap: 4px; font-size: 13px;">
                      <el-icon v-if="data.isDir" style="color: #E6A23C"><FolderOpened /></el-icon>
                      <el-icon v-else style="color: #409EFF"><Document /></el-icon>
                      <span>{{ data.name }}</span>
                      <el-text v-if="!data.isDir && data.size" type="info" size="small" style="margin-left: auto">
                        {{ formatSize(data.size) }}
                      </el-text>
                    </span>
                  </template>
                </el-tree>
                <el-empty v-if="memoryTreeData.length === 0" description="记忆树为空" :image-size="40" />
              </el-card>
            </el-col>
            <!-- Right: file editor (70%) -->
            <el-col :span="17">
              <el-card shadow="hover">
                <template #header>
                  <div style="display: flex; align-items: center; justify-content: space-between;">
                    <el-breadcrumb separator="/">
                      <el-breadcrumb-item>memory</el-breadcrumb-item>
                      <el-breadcrumb-item v-for="(seg, i) in memoryFileBreadcrumb" :key="i">{{ seg }}</el-breadcrumb-item>
                    </el-breadcrumb>
                    <el-button v-if="memoryEditPath" type="primary" size="small" @click="saveMemoryFile" :loading="memorySaving">保存</el-button>
                  </div>
                </template>
                <template v-if="memoryEditPath">
                  <el-input
                    v-model="memoryEditContent"
                    type="textarea"
                    :rows="22"
                    style="font-family: monospace;"
                  />
                </template>
                <template v-else>
                  <el-empty description="点击左侧文件查看和编辑" :image-size="60" />
                </template>
              </el-card>
            </el-col>
          </el-row>

          <!-- New memory file dialog -->
          <el-dialog v-model="showNewMemoryFile" title="新建记忆文件" width="480px">
            <el-form label-width="80px">
              <el-form-item label="路径">
                <el-input v-model="newMemoryPath" placeholder="例如: projects/my-project.md 或 topics/cooking.md" />
                <el-text type="info" size="small" style="margin-top: 4px">相对于 memory/ 目录</el-text>
              </el-form-item>
            </el-form>
            <template #footer>
              <el-button @click="showNewMemoryFile = false">取消</el-button>
              <el-button type="primary" @click="createMemoryFile">创建</el-button>
            </template>
          </el-dialog>

          <!-- Daily log entry dialog -->
          <el-dialog v-model="showDailyEntry" title="添加今日日志" width="600px">
            <el-input
              v-model="dailyEntryContent"
              type="textarea"
              :rows="10"
              placeholder="记录今天的重要事项、学习心得、待办..."
            />
            <template #footer>
              <el-button @click="showDailyEntry = false">取消</el-button>
              <el-button type="primary" @click="submitDailyEntry">提交</el-button>
            </template>
          </el-dialog>
        </el-tab-pane>

        <!-- Tab: 技能 -->
        <el-tab-pane label="技能" name="skills">
          <SkillStudio :agent-id="agentId" />
        </el-tab-pane>

        <!-- Tab: 历史对话 -->
        <el-tab-pane label="历史对话" name="convlogs">
          <div style="margin-bottom: 16px; display: flex; align-items: center; gap: 8px;">
            <span style="font-weight: 600; font-size: 15px;">渠道对话记录</span>
            <el-button size="small" :icon="Refresh" circle @click="loadConvChannels" :loading="convLoading" />
          </div>

          <el-table :data="convChannels" stripe v-loading="convLoading" empty-text="暂无对话记录">
            <el-table-column label="渠道" min-width="200">
              <template #default="{ row }">
                <span>{{ row.channelType === 'telegram' ? 'Telegram' : 'Web' }} {{ row.channelId }}</span>
              </template>
            </el-table-column>
            <el-table-column label="消息数" width="100">
              <template #default="{ row }">{{ row.messageCount }} 条</template>
            </el-table-column>
            <el-table-column label="最后活跃" width="180">
              <template #default="{ row }">{{ row.lastAt ? new Date(row.lastAt).toLocaleString('zh-CN') : '-' }}</template>
            </el-table-column>
            <el-table-column label="操作" width="100">
              <template #default="{ row }">
                <el-button size="small" type="primary" plain @click="openConvDrawer(row)">查看</el-button>
              </template>
            </el-table-column>
          </el-table>

          <!-- Conversation Drawer -->
          <el-drawer
            v-model="convDrawerVisible"
            :title="convDrawerChannelId + ' 对话记录'"
            direction="rtl"
            size="520px"
            :destroy-on-close="false"
          >
            <div class="conv-drawer-body">
              <!-- Load more button at top -->
              <div v-if="convHasMore" style="text-align: center; margin-bottom: 12px;">
                <el-button size="small" plain :loading="convMsgLoading" @click="loadMoreConvMsgs">加载更多</el-button>
              </div>

              <div v-loading="convMsgLoading && convMessages.length === 0" class="conv-msg-list">
                <div
                  v-for="(msg, idx) in convMessages"
                  :key="idx"
                  :class="['conv-msg-item', msg.role === 'user' ? 'conv-msg-user' : 'conv-msg-assistant']"
                >
                  <div class="conv-msg-meta">
                    <span class="conv-msg-role">{{ msg.role === 'user' ? '用户' : '助手' }}</span>
                    <span v-if="msg.sender" class="conv-msg-sender">{{ msg.sender }}</span>
                    <span class="conv-msg-time">{{ msg.ts ? new Date(msg.ts).toLocaleString('zh-CN') : '' }}</span>
                  </div>
                  <div class="conv-msg-content">{{ msg.content }}</div>
                </div>
                <div v-if="!convMsgLoading && convMessages.length === 0" class="conv-msg-empty">
                  暂无消息记录
                </div>
              </div>
            </div>
          </el-drawer>
        </el-tab-pane>

        <!-- Tab 4: Workspace -->
        <el-tab-pane label="工作区" name="workspace">
          <el-row :gutter="16">
            <!-- 左栏：文件树 -->
            <el-col :span="7">
              <el-card shadow="never" style="height: calc(100vh - 200px); display: flex; flex-direction: column;">
                <template #header>
                  <div style="display: flex; align-items: center; justify-content: space-between;">
                    <span>文件列表</span>
                    <div style="display:flex;gap:4px">
                      <el-button text size="small" @click="showNewFileDialog = true" title="新建文件">
                        <el-icon><Plus /></el-icon>
                      </el-button>
                      <el-button text size="small" @click="loadWorkspace" title="刷新">
                        <el-icon><Refresh /></el-icon>
                      </el-button>
                    </div>
                  </div>
                </template>
                <div style="flex: 1; overflow-y: auto;">
                  <el-tree
                    :data="fileTreeData"
                    :props="{ label: 'name', children: 'children' }"
                    @node-click="handleFileClick"
                    highlight-current
                    default-expand-all
                    style="font-size: 13px;"
                  >
                    <template #default="{ data }">
                      <span style="display: flex; align-items: center; gap: 5px; line-height: 1.8; width: 100%;">
                        <el-icon v-if="data.isDir" style="color: #e6a23c; font-size: 14px;"><FolderOpened /></el-icon>
                        <el-icon v-else :style="{ color: fileIconColor(data.name), fontSize: '14px' }"><Document /></el-icon>
                        <span style="flex:1; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">{{ data.name }}</span>
                        <el-text v-if="!data.isDir" type="info" size="small" style="flex-shrink:0; font-size:11px;">
                          {{ formatSize(data.size) }}
                        </el-text>
                      </span>
                    </template>
                  </el-tree>
                </div>
              </el-card>
            </el-col>

            <!-- 右栏：编辑器 -->
            <el-col :span="17">
              <el-card shadow="never" style="height: calc(100vh - 200px); display: flex; flex-direction: column;">
                <template #header>
                  <div style="display:flex; align-items:center; justify-content:space-between;">
                    <span style="font-family:monospace; font-size:13px; color:#606266;">
                      {{ currentFile || '选择文件查看' }}
                      <el-tag v-if="currentFile" size="small" style="margin-left:6px; font-size:11px;">
                        {{ fileExt(currentFile) }}
                      </el-tag>
                    </span>
                    <el-button
                      v-if="currentFile"
                      text size="small" type="danger"
                      title="删除文件"
                      @click="deleteCurrentFile"
                    >
                      <el-icon><Delete /></el-icon>
                    </el-button>
                  </div>
                </template>
                <template v-if="currentFile">
                  <el-input
                    v-model="currentFileContent"
                    type="textarea"
                    :autosize="false"
                    :placeholder="currentFileBinary ? '二进制文件，无法编辑' : '（空文件）'"
                    :readonly="currentFileBinary"
                    style="flex: 1; font-family: monospace; font-size: 13px;"
                    :rows="22"
                  />
                  <div style="margin-top: 8px; display: flex; gap: 8px; align-items: center;">
                    <el-button type="primary" :disabled="currentFileBinary" @click="saveCurrentFile">保存</el-button>
                    <el-text type="info" size="small" v-if="currentFileInfo">
                      {{ formatSize(currentFileInfo.size) }} · {{ formatTime(currentFileInfo.modTime) }}
                    </el-text>
                  </div>
                </template>
                <el-empty v-else description="从左侧选择文件" :image-size="60" />
              </el-card>
            </el-col>
          </el-row>

          <!-- 新建文件 Dialog -->
          <el-dialog v-model="showNewFileDialog" title="新建文件" width="400px">
            <el-form label-width="80px">
              <el-form-item label="文件路径">
                <el-input
                  v-model="newFilePath"
                  placeholder="例如: notes.md 或 idear/my-idea.md"
                  @keyup.enter="createNewFile"
                />
              </el-form-item>
            </el-form>
            <template #footer>
              <el-button @click="showNewFileDialog = false">取消</el-button>
              <el-button type="primary" @click="createNewFile">创建</el-button>
            </template>
          </el-dialog>
        </el-tab-pane>

        <!-- Tab 5: Cron -->
        <el-tab-pane label="定时任务" name="cron">
          <el-button type="primary" @click="showCronCreate = true" style="margin-bottom: 16px">
            <el-icon><Plus /></el-icon> 新建任务
          </el-button>
          <el-table :data="cronJobs" stripe>
            <el-table-column prop="name" label="名称" />
            <el-table-column label="调度">
              <template #default="{ row }">{{ row.schedule?.expr }} ({{ row.schedule?.tz }})</template>
            </el-table-column>
            <el-table-column label="最近运行" width="180">
              <template #default="{ row }">
                <template v-if="row.state?.lastRunAtMs">
                  <el-tag :type="row.state?.lastStatus === 'ok' ? 'success' : 'danger'" size="small">
                    {{ row.state?.lastStatus }}
                  </el-tag>
                  <el-text type="info" size="small" style="margin-left: 4px">
                    {{ formatTimestamp(row.state?.lastRunAtMs) }}
                  </el-text>
                </template>
                <el-text v-else type="info" size="small">未运行</el-text>
              </template>
            </el-table-column>
            <el-table-column label="启用" width="80">
              <template #default="{ row }">
                <el-switch v-model="row.enabled" @change="toggleCron(row)" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="220">
              <template #default="{ row }">
                <template v-if="row.payload?.message === '__MEMORY_CONSOLIDATE__'">
                  <el-tag type="info" size="small" style="margin-right: 8px;">记忆管理</el-tag>
                  <el-button size="small" @click="runCronNow(row)">立即运行</el-button>
                </template>
                <template v-else>
                  <el-button size="small" @click="runCronNow(row)">立即运行</el-button>
                  <el-button size="small" type="danger" @click="deleteCron(row)">删除</el-button>
                </template>
              </template>
            </el-table-column>
          </el-table>

          <!-- Create Cron Dialog -->
          <el-dialog v-model="showCronCreate" title="新建定时任务" width="520px">
            <el-form :model="cronForm" label-width="100px">
              <el-form-item label="名称">
                <el-input v-model="cronForm.name" />
              </el-form-item>
              <el-form-item label="Cron 表达式">
                <el-input v-model="cronForm.expr" placeholder="30 3 * * *" />
              </el-form-item>
              <el-form-item label="时区">
                <el-select v-model="cronForm.tz">
                  <el-option label="Asia/Shanghai" value="Asia/Shanghai" />
                  <el-option label="UTC" value="UTC" />
                  <el-option label="America/New_York" value="America/New_York" />
                </el-select>
              </el-form-item>
              <el-form-item label="消息">
                <el-input v-model="cronForm.message" type="textarea" :rows="3" />
              </el-form-item>
              <el-form-item label="启用">
                <el-switch v-model="cronForm.enabled" />
              </el-form-item>
            </el-form>
            <template #footer>
              <el-button @click="showCronCreate = false">取消</el-button>
              <el-button type="primary" @click="createCron">创建</el-button>
            </template>
          </el-dialog>
        </el-tab-pane>

        <!-- Tab 7: 渠道 (per-agent channel config) -->
        <el-tab-pane label="渠道" name="channels">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px">
            <el-text type="info" size="small">每个 AI 成员独立配置自己的消息通道（如 Telegram Bot Token）</el-text>
            <el-button type="primary" size="small" @click="openAddChannel">
              <el-icon><Plus /></el-icon> 添加消息渠道
            </el-button>
          </div>

          <!-- Channel cards -->
          <div v-for="ch in agentChannelList" :key="ch.id" class="channel-card">
            <div class="channel-card-header">
              <div class="channel-card-left">
                <el-tag size="small" style="margin-right: 8px">{{ ch.type }}</el-tag>
                <span class="channel-card-name">{{ ch.name }}</span>
                <span v-if="ch.config?.botName" class="channel-bot-username">@{{ ch.config.botName }}</span>
                <el-tag
                  :type="ch.status === 'ok' ? 'success' : ch.status === 'error' ? 'danger' : 'info'"
                  size="small" effect="plain" style="margin-left: 8px"
                >{{ ch.status === 'ok' ? '✓ 正常' : ch.status === 'error' ? '✗ 错误' : '未测试' }}</el-tag>
              </div>
              <div class="channel-card-actions">
                <el-switch v-model="ch.enabled" size="small" @change="saveChannels" style="margin-right: 8px" />
                <el-button size="small" @click="testAgentChannel(ch)" :loading="testingChannelId === ch.id">测试连接</el-button>
                <el-button size="small" @click="openEditChannel(ch)">编辑</el-button>
                <el-button size="small" type="danger" plain @click="deleteAgentChannel(ch)">删除</el-button>
              </div>
            </div>

            <!-- Web channel: show public URL -->
            <div v-if="ch.type === 'web'" class="channel-card-body">
              <div class="channel-info-row">
                <span class="channel-info-label">公开地址</span>
                <span class="channel-info-value">
                  <el-link :href="webChatUrl(agentId, ch.id)" target="_blank" type="primary" style="font-size:13px">
                    {{ webChatUrl(agentId, ch.id) }}
                  </el-link>
                  <el-button size="small" link style="margin-left:8px" @click="copyUrl(webChatUrl(agentId, ch.id))">复制</el-button>
                </span>
              </div>
              <div class="channel-info-row">
                <span class="channel-info-label">访问密码</span>
                <span class="channel-info-value">
                  <el-tag size="small" :type="ch.config?.password ? 'warning' : 'info'" effect="plain">
                    {{ ch.config?.password ? '已设置' : '无密码' }}
                  </el-tag>
                </span>
              </div>
            </div>

            <!-- Telegram whitelist info -->
            <div v-if="ch.type === 'telegram'" class="channel-card-body">
              <div class="channel-info-row">
                <span class="channel-info-label">白名单用户</span>
                <span class="channel-info-value">
                  <template v-if="ch.allowedFromUsers?.length">
                    <el-tag
                      v-for="u in ch.allowedFromUsers"
                      :key="u.id"
                      size="small"
                      closable
                      :disable-transitions="true"
                      style="margin-right: 4px; margin-bottom: 4px"
                      @close="removeAllowed(ch.id, u.id)"
                    >
                      {{ u.username ? '@' + u.username : u.firstName || String(u.id) }}
                      <span style="opacity:0.6;font-size:11px;margin-left:3px">({{ u.id }})</span>
                    </el-tag>
                  </template>
                  <template v-else-if="ch.config?.allowedFrom">
                    <el-tag
                      v-for="uid in ch.config.allowedFrom.split(',')"
                      :key="uid"
                      size="small"
                      closable
                      :disable-transitions="true"
                      style="margin-right: 4px; margin-bottom: 4px"
                      @close="removeAllowed(ch.id, Number(uid.trim()))"
                    >{{ uid.trim() }}</el-tag>
                  </template>
                  <el-text v-else type="warning" size="small">未设置（配对模式，向用户返回其 ID）</el-text>
                </span>
              </div>

              <!-- Pending users section -->
              <div class="pending-section">
                <div class="pending-section-header" @click="togglePending(ch.id)">
                  <span>待审核用户</span>
                  <el-badge
                    :value="(pendingUsers[ch.id] || []).length"
                    :hidden="!(pendingUsers[ch.id] || []).length"
                    type="warning"
                    style="margin-left: 6px"
                  />
                  <el-button size="small" link @click.stop="loadPendingUsers(ch.id)" style="margin-left: 8px">刷新</el-button>
                  <el-icon style="margin-left: 4px; transition: transform 0.2s" :style="{ transform: expandedPending === ch.id ? 'rotate(180deg)' : '' }">
                    <ArrowDown />
                  </el-icon>
                </div>

                <div v-if="expandedPending === ch.id" class="pending-list">
                  <div v-if="pendingLoading[ch.id]" style="text-align: center; padding: 12px">
                    <el-text type="info" size="small">加载中...</el-text>
                  </div>
                  <template v-else-if="(pendingUsers[ch.id] || []).length">
                    <div v-for="user in pendingUsers[ch.id]" :key="user.id" class="pending-user-row">
                      <div class="pending-user-info">
                        <span class="pending-user-name">{{ user.firstName || '未知' }}</span>
                        <span v-if="user.username" class="pending-user-username">@{{ user.username }}</span>
                        <span class="pending-user-id">ID: {{ user.id }}</span>
                        <el-text type="info" size="small" style="margin-left: 8px">{{ formatRelative(user.lastSeen) }}</el-text>
                      </div>
                      <div class="pending-user-actions">
                        <el-button
                          size="small" type="success" plain
                          @click="allowUser(ch.id, user.id)"
                          :loading="allowingUserId === `${ch.id}-${user.id}`"
                        >加入白名单</el-button>
                        <el-button
                          size="small" type="danger" plain
                          @click="dismissUser(ch.id, user.id)"
                        >忽略</el-button>
                      </div>
                    </div>
                  </template>
                  <div v-else class="pending-empty">
                    暂无待审核用户。让用户向 Bot 发送 /start 即可出现在此处。
                  </div>
                </div>
              </div>
            </div>
          </div>

          <el-empty v-if="!channelsLoading && !agentChannelList.length" description="暂无消息渠道，点击「添加消息渠道」开始配置" :image-size="80" style="margin-top: 40px" />

          <!-- Add/Edit Dialog -->
          <el-dialog v-model="channelDialogVisible" :title="channelEditingId ? '编辑消息渠道' : '添加消息渠道'" width="540px">
            <el-form :model="channelForm" label-width="120px">
              <el-form-item label="类型" required>
                <el-select v-model="channelForm.type" style="width: 100%">
                  <el-option label="Telegram" value="telegram" />
                  <el-option label="Web 聊天页" value="web" />
                  <el-option label="iMessage" value="imessage" />
                  <el-option label="WhatsApp" value="whatsapp" />
                </el-select>
              </el-form-item>
              <el-form-item label="名称" required>
                <el-input v-model="channelForm.name" placeholder="如：客服 Bot" />
              </el-form-item>

              <!-- Telegram-specific -->
              <template v-if="channelForm.type === 'telegram'">
                <el-form-item label="Bot Token" required>
                  <div style="width:100%">
                    <div style="display:flex;gap:6px;align-items:center">
                      <el-input
                        v-model="channelForm.botToken"
                        type="password"
                        show-password
                        placeholder="从 @BotFather 获取"
                        style="flex:1"
                        :status="tokenCheckState.status === 'error' ? 'error' : tokenCheckState.status === 'ok' ? 'success' : ''"
                      />
                      <el-button
                        size="default"
                        :loading="tokenCheckState.loading"
                        :type="tokenCheckState.status === 'ok' ? 'success' : tokenCheckState.status === 'error' ? 'danger' : 'default'"
                        @click="doCheckToken"
                        :disabled="!channelForm.botToken || ismaskedToken(channelForm.botToken)"
                      >验证</el-button>
                    </div>
                    <!-- Inline feedback -->
                    <div v-if="tokenCheckState.loading" style="margin-top:6px;display:flex;align-items:center;gap:6px;color:#909399;font-size:13px">
                      <el-icon class="is-loading"><Refresh /></el-icon> 正在验证 Token…
                    </div>
                    <div v-else-if="tokenCheckState.status === 'ok'" style="margin-top:6px;color:#67c23a;font-size:13px">
                      <el-icon style="vertical-align:-2px;margin-right:4px"><CircleCheck /></el-icon>Token 有效，Bot 名称：<b>@{{ tokenCheckState.botName }}</b>
                    </div>
                    <div v-else-if="tokenCheckState.status === 'duplicate'" style="margin-top:6px;color:#e6a23c;font-size:13px">
                      <el-icon style="vertical-align:-2px;margin-right:4px"><Warning /></el-icon>此 Token 已被成员「<b>{{ tokenCheckState.usedBy }}</b>」的渠道「{{ tokenCheckState.usedByCh }}」使用
                    </div>
                    <div v-else-if="tokenCheckState.status === 'error'" style="margin-top:6px;color:#f56c6c;font-size:13px">
                      <el-icon style="vertical-align:-2px;margin-right:4px"><CircleClose /></el-icon>{{ tokenCheckState.error }}
                    </div>
                    <div v-else style="margin-top:4px">
                      <el-text type="info" size="small"><el-icon style="vertical-align:-2px;margin-right:4px"><InfoFilled /></el-icon>输入完成后自动验证，也可点右侧「验证」按钮手动触发</el-text>
                    </div>
                  </div>
                </el-form-item>
                <el-form-item label="白名单用户">
                  <el-input v-model="channelForm.allowedFrom" placeholder="填入 Telegram 用户 ID，多个用逗号分隔" />
                  <el-text type="info" size="small" style="display:block;margin-top:4px">
                    <el-icon style="vertical-align:-2px;margin-right:4px"><InfoFilled /></el-icon>留空时 Bot 进入配对模式——向用户返回其 ID，引导联系管理员添加白名单
                  </el-text>
                </el-form-item>
              </template>

              <!-- Web channel -->
              <template v-if="channelForm.type === 'web'">
                <el-form-item v-if="channelEditingId" label="访问链接">
                  <div class="channel-url-preview">
                    <el-icon style="flex-shrink:0;color:#909399"><Link /></el-icon>
                    <span class="channel-url-text">{{ webChatUrl(agentId, channelEditingId) }}</span>
                    <el-button size="small" link @click="copyUrl(webChatUrl(agentId, channelEditingId))">复制</el-button>
                  </div>
                </el-form-item>
                <el-form-item v-if="!channelEditingId" label="访问链接">
                  <div class="channel-url-preview">
                    <el-icon style="flex-shrink:0;color:#909399"><Link /></el-icon>
                    <span class="channel-url-text">{{ webChatUrl(agentId, pendingChannelId) }}</span>
                    <el-tag size="small" type="info" effect="plain">保存后生效</el-tag>
                  </div>
                  <el-text type="info" size="small" style="display:block;margin-top:4px">
                    每个 Web 渠道有独立链接，可同时开放多个入口
                  </el-text>
                </el-form-item>
                <el-form-item label="访问密码">
                  <el-input v-model="channelForm.webPassword" type="password" show-password placeholder="留空则无需密码" />
                </el-form-item>
                <el-form-item label="欢迎语">
                  <el-input v-model="channelForm.webWelcome" placeholder="你好！有什么可以帮你的？" />
                </el-form-item>
                <el-form-item label="页面标题">
                  <el-input v-model="channelForm.webTitle" placeholder="AI 助手" />
                </el-form-item>
              </template>

              <el-form-item label="启用">
                <el-switch v-model="channelForm.enabled" />
              </el-form-item>
            </el-form>
            <template #footer>
              <el-button @click="channelDialogVisible = false">取消</el-button>
              <el-button type="primary" @click="saveChannelDialog" :loading="channelSaving">保存</el-button>
            </template>
          </el-dialog>
        </el-tab-pane>

      </el-tabs>
    </el-main>
  </el-container>
</template>

<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft, Plus, EditPen, Refresh, FolderOpened, Document, ArrowDown } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import SkillStudio from '../components/SkillStudio.vue'
import { agents as agentsApi, files as filesApi, memoryApi, cron as cronApi, sessions as sessionsApi, relationsApi, memoryConfigApi, agentChannels as agentChannelsApi, agentConversations, models as modelsApi, type AgentInfo, type FileEntry, type FileNode, type CronJob, type SessionSummary, type RelationRow, type MemConfig, type MemRunLog, type ChannelEntry, type PendingUser, type ConvEntry, type ChannelSummary, type ModelEntry } from '../api'
import AiChat, { type ChatMsg } from '../components/AiChat.vue'

const route = useRoute()
const agentId = route.params.id as string
const agent = ref<AgentInfo | null>(null)
const activeTab = ref('chat')

// ── Session sidebar ────────────────────────────────────────────────────────
const aiChatRef = ref<InstanceType<typeof AiChat>>()
const agentSessions = ref<SessionSummary[]>([])
const sessionsLoading = ref(false)
const activeSessionId = ref<string | undefined>()

async function loadAgentSessions() {
  sessionsLoading.value = true
  try {
    const res = await sessionsApi.list({ agentId, limit: 50 })
    agentSessions.value = res.data.sessions
  } catch {}
  finally { sessionsLoading.value = false }
}

function resumeSession(s: SessionSummary) {
  activeSessionId.value = s.id
  aiChatRef.value?.resumeSession(s.id)
}

function newSession() {
  activeSessionId.value = undefined
  aiChatRef.value?.startNewSession()
}

function onSessionChange(sessionId: string) {
  activeSessionId.value = sessionId
  // Refresh session list to show new entry
  setTimeout(loadAgentSessions, 500)
}

function formatRelative(ms: number): string {
  if (!ms) return ''
  const diff = Date.now() - ms
  if (diff < 60_000) return '刚刚'
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}分前`
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}小时前`
  return `${Math.floor(diff / 86_400_000)}天前`
}

// ── @ 其他成员 ─────────────────────────────────────────────────────────────
const showAtPanel   = ref(false)
const atTargetId    = ref('')
const atMessage     = ref('')
const atSending     = ref(false)
const otherAgents   = ref<AgentInfo[]>([])

function toggleAtPanel() {
  showAtPanel.value = !showAtPanel.value
  if (showAtPanel.value && !otherAgents.value.length) loadOtherAgents()
}

async function loadOtherAgents() {
  try {
    const res = await agentsApi.list()
    otherAgents.value = res.data.filter(a => a.id !== agentId)
  } catch {
    otherAgents.value = []
  }
}

function onAtAgentSelect(id: string) {
  // 同步在 AiChat 输入框填入 @AgentName: 前缀（方便用户知道当前 @ 模式）
  const target = otherAgents.value.find(a => a.id === id)
  if (target) {
    aiChatRef.value?.fillInput(`@${target.name}: `)
  }
}

async function sendAtMessage() {
  const targetId = atTargetId.value
  const msg = atMessage.value.trim()
  if (!targetId || !msg) return

  const targetAgent = otherAgents.value.find(a => a.id === targetId)
  const targetName  = targetAgent?.name ?? targetId

  atSending.value = true

  // 在对话区显示「转发」提示气泡
  const forwardBubble: ChatMsg = {
    role: 'user',
    text: `→ 转发给 ${targetName}：\n${msg}`,
  }
  aiChatRef.value?.appendMessage(forwardBubble)

  try {
    const res = await agentsApi.message(targetId, msg, agentId)
    const reply = res.data.response

    // 显示「回复」气泡
    const replyBubble: ChatMsg = {
      role: 'assistant',
      text: `← **${targetName}** 回复：\n\n${reply}`,
    }
    aiChatRef.value?.appendMessage(replyBubble)

    // 清空输入
    atMessage.value = ''
    atTargetId.value = ''
    showAtPanel.value = false
    ElMessage.success(`${targetName} 已回复`)
  } catch (e: any) {
    const errMsg: ChatMsg = {
      role: 'system',
      text: `[失败] 转发失败：${e.response?.data?.error ?? e.message ?? '网络错误'}`,
    }
    aiChatRef.value?.appendMessage(errMsg)
    ElMessage.error('转发失败')
  } finally {
    atSending.value = false
  }
}

// Identity/Soul
const identityContent = ref('')
const soulContent = ref('')

// Model selector
const modelList = ref<ModelEntry[]>([])
const agentModelId = ref('')
const agentModelSaving = ref(false)

// Memory config (automatic consolidation)
const memCfg = ref<MemConfig>({
  enabled: false,
  schedule: 'daily',
  keepTurns: 3,
  focusHint: '',
  cronJobId: '',
})
const memCfgSaving = ref(false)
const memConsolidating = ref(false)

async function loadMemConfig() {
  try {
    const res = await memoryConfigApi.getConfig(agentId)
    memCfg.value = res.data
    loadMemLogs()
  } catch {
    // use defaults
  }
}

async function saveMemConfig() {
  memCfgSaving.value = true
  try {
    const res = await memoryConfigApi.setConfig(agentId, memCfg.value)
    memCfg.value = res.data
    ElMessage.success(memCfg.value.enabled ? '自动记忆已开启' : '自动记忆已关闭')
  } catch {
    ElMessage.error('保存失败')
  } finally {
    memCfgSaving.value = false
  }
}

async function consolidateNow() {
  memConsolidating.value = true
  try {
    await memoryConfigApi.consolidate(agentId)
    ElMessage.success('记忆整理已在后台启动（约需10~30秒），稍后自动刷新日志')
    setTimeout(loadMemLogs, 10000) // 10秒后刷新日志
  } catch {
    ElMessage.error('整理失败')
  } finally {
    memConsolidating.value = false
  }
}

// Consolidation run log
const memLogs = ref<MemRunLog[]>([])
const memLogsLoading = ref(false)

async function loadMemLogs() {
  memLogsLoading.value = true
  try {
    const res = await memoryConfigApi.runLog(agentId)
    memLogs.value = res.data || []
  } catch {
    memLogs.value = []
  } finally {
    memLogsLoading.value = false
  }
}

// Memory tree
const memoryTreeData = ref<any[]>([])
const memoryEditPath = ref('')
const memoryEditContent = ref('')
const memorySaving = ref(false)
const memoryFileBreadcrumb = ref<string[]>([])
const showNewMemoryFile = ref(false)
const newMemoryPath = ref('')
const showDailyEntry = ref(false)
const dailyEntryContent = ref('')

// Workspace
const fileTreeData = ref<FileNode[]>([])
const currentFile = ref('')
const currentFileContent = ref('')
const currentFileInfo = ref<FileEntry | null>(null)
const currentFileBinary = ref(false)
const showNewFileDialog = ref(false)
const newFilePath = ref('')

// 根据扩展名给文件图标着色
function fileIconColor(name: string): string {
  const ext = name.split('.').pop()?.toLowerCase() || ''
  if (['md', 'txt', 'rst'].includes(ext)) return '#409eff'
  if (['json', 'yaml', 'yml', 'toml'].includes(ext)) return '#67c23a'
  if (['go', 'py', 'js', 'ts', 'sh', 'bash'].includes(ext)) return '#e6a23c'
  if (['jpg', 'jpeg', 'png', 'gif', 'svg', 'webp'].includes(ext)) return '#f56c6c'
  if (name.startsWith('.')) return '#c0c4cc'
  return '#909399'
}

// 获取文件扩展名
function fileExt(path: string): string {
  const name = path.split('/').pop() || path
  const ext = name.split('.').pop()?.toLowerCase() || ''
  if (!ext || ext === name.toLowerCase()) return 'file'
  return '.' + ext
}

// Relations
const parsedRelations = ref<RelationRow[]>([])
const relationsSaving = ref(false)
const newRelation = ref({ agentId: '', agentName: '', relationType: '平级协作', strength: '常用', desc: '' })

async function loadRelations() {
  try {
    const res = await relationsApi.get(agentId)
    parsedRelations.value = res.data.parsed || []
  } catch {
    parsedRelations.value = []
  }
}

function onRelationAgentChange(id: string) {
  const a = otherAgents.value.find(x => x.id === id)
  newRelation.value.agentName = a ? a.name : id
}

async function addRelation() {
  if (!newRelation.value.agentId) return
  // Avoid duplicate
  const exists = parsedRelations.value.find(r => r.agentId === newRelation.value.agentId)
  if (exists) {
    ElMessage.warning('该成员关系已存在，请先删除再重新添加')
    return
  }
  parsedRelations.value.push({ ...newRelation.value })
  newRelation.value = { agentId: '', agentName: '', relationType: '平级协作', strength: '常用', desc: '' }
  await saveRelations()
}

async function deleteRelation(index: number) {
  parsedRelations.value.splice(index, 1)
  await saveRelations()
}

function serializeRelations(): string {
  if (parsedRelations.value.length === 0) return ''
  const header = '| 成员ID | 成员名称 | 关系类型 | 关系程度 | 说明 |\n|--------|--------|--------|--------|------|'
  const rows = parsedRelations.value
    .map(r => `| ${r.agentId} | ${r.agentName} | ${r.relationType} | ${r.strength} | ${r.desc || ''} |`)
    .join('\n')
  return header + '\n' + rows
}

async function saveRelations() {
  relationsSaving.value = true
  try {
    await relationsApi.put(agentId, serializeRelations())
    ElMessage.success('关系已保存')
  } catch {
    ElMessage.error('保存失败')
  } finally {
    relationsSaving.value = false
  }
}

function avatarColor(id: string): string {
  const colors = ['#409EFF', '#67C23A', '#E6A23C', '#F56C6C', '#909399', '#B45309', '#7C3AED', '#0891B2']
  let hash = 0
  for (let i = 0; i < id.length; i++) hash = id.charCodeAt(i) + ((hash << 5) - hash)
  return colors[Math.abs(hash) % colors.length] ?? '#409EFF'
}

function relationTypeColor(type: string): '' | 'success' | 'warning' | 'info' | 'danger' {
  if (type === '上级') return 'danger'
  if (type === '下级') return ''     // blue = default primary
  if (type === '平级协作') return 'success'
  return 'info'  // 支持
}

function strengthColor(s: string): '' | 'success' | 'warning' | 'info' | 'danger' {
  if (s === '核心') return 'danger'
  if (s === '常用') return 'warning'
  return 'info'
}

// Cron
const cronJobs = ref<CronJob[]>([])
const showCronCreate = ref(false)
const cronForm = ref({ name: '', expr: '0 9 * * *', tz: 'Asia/Shanghai', message: '', enabled: true })

function statusType(s?: string) {
  return s === 'running' ? 'success' : s === 'stopped' ? 'danger' : 'info'
}
function statusLabel(s?: string) {
  return s === 'running' ? '运行中' : s === 'stopped' ? '已停止' : '空闲'
}
function formatSize(bytes: number) {
  if (!bytes) return '0 B'
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / 1048576).toFixed(1) + ' MB'
}
function formatTime(t: string) {
  return new Date(t).toLocaleString()
}
function formatTimestamp(ms: number) {
  if (!ms) return ''
  return new Date(ms).toLocaleString()
}

// Load agent
// ── Per-agent Channel management ──────────────────────────────────────────
const agentChannelList = ref<ChannelEntry[]>([])
const channelsLoading = ref(false)
const channelDialogVisible = ref(false)
const channelEditingId = ref('')
const pendingChannelId = ref('')  // pre-generated id for new web channel
const channelSaving = ref(false)
const testingChannelId = ref('')
const channelForm = ref({
  type: 'telegram',
  name: '',
  enabled: true,
  botToken: '',
  allowedFrom: '',
  webPassword: '',
  webWelcome: '',
  webTitle: '',
})

// ── Token inline validation ────────────────────────────────────────────────
const tokenCheckState = ref<{
  loading: boolean
  status: '' | 'ok' | 'error' | 'duplicate'
  botName?: string
  usedBy?: string
  usedByCh?: string
  error?: string
}>({ loading: false, status: '' })

let tokenDebounceTimer: ReturnType<typeof setTimeout> | null = null

function ismaskedToken(v: string) {
  return /^\*+$/.test(v)
}

async function doCheckToken() {
  const token = channelForm.value.botToken
  if (!token || ismaskedToken(token)) return
  tokenCheckState.value = { loading: true, status: '' }
  try {
    const res = await agentChannelsApi.checkToken(agentId, token)
    const d = res.data
    if (d.duplicate) {
      tokenCheckState.value = { loading: false, status: 'duplicate', usedBy: d.usedBy, usedByCh: d.usedByCh }
    } else if (d.valid) {
      tokenCheckState.value = { loading: false, status: 'ok', botName: d.botName }
      // Auto-fill name if empty
      if (!channelForm.value.name && d.botName) channelForm.value.name = d.botName
    } else {
      tokenCheckState.value = { loading: false, status: 'error', error: d.error || 'Token 无效' }
    }
  } catch {
    tokenCheckState.value = { loading: false, status: 'error', error: '网络错误，请重试' }
  }
}

// Auto-check when token input stabilises (800ms debounce, min length ~20)
watch(() => channelForm.value.type, (val) => {
  if (!channelEditingId.value) {
    pendingChannelId.value = genChannelId(val)
  }
})

watch(() => channelForm.value.botToken, (val) => {
  // Reset state on change
  tokenCheckState.value = { loading: false, status: '' }
  if (tokenDebounceTimer) clearTimeout(tokenDebounceTimer)
  // Telegram tokens are "botId:hash" — typically 40+ chars; skip short/masked values
  if (!val || ismaskedToken(val) || val.length < 20 || !val.includes(':')) return
  tokenDebounceTimer = setTimeout(doCheckToken, 800)
})

function webChatUrl(aid: string, chId?: string): string {
  return chId
    ? `${window.location.origin}/chat/${aid}/${chId}`
    : `${window.location.origin}/chat/${aid}`
}

function copyUrl(url: string) {
  navigator.clipboard.writeText(url).then(() => ElMessage.success('链接已复制'))
}

async function loadAgentChannels() {
  channelsLoading.value = true
  try {
    const res = await agentChannelsApi.list(agentId)
    agentChannelList.value = res.data || []
  } catch {
    agentChannelList.value = []
  } finally {
    channelsLoading.value = false
  }
}

function genChannelId(type: string) {
  return type + '-' + Date.now().toString(36)
}

function openAddChannel() {
  channelEditingId.value = ''
  const defaultName = agent.value?.name || ''
  pendingChannelId.value = genChannelId('telegram') // default, updated on type change
  channelForm.value = { type: 'telegram', name: defaultName, enabled: true, botToken: '', allowedFrom: '', webPassword: '', webWelcome: '', webTitle: '' }
  tokenCheckState.value = { loading: false, status: '' }
  channelDialogVisible.value = true
}

function openEditChannel(row: ChannelEntry) {
  channelEditingId.value = row.id
  channelForm.value = {
    type: row.type,
    name: row.name,
    enabled: row.enabled,
    botToken: row.config?.botToken || '',
    allowedFrom: row.config?.allowedFrom || '',
    webPassword: '',  // password always cleared on edit for security
    webWelcome: row.config?.welcomeMsg || '',
    webTitle: row.config?.title || '',
  }
  tokenCheckState.value = { loading: false, status: '' }
  channelDialogVisible.value = true
}

async function saveChannelDialog() {
  if (!channelForm.value.name || !channelForm.value.type) {
    ElMessage.warning('请填写名称和类型')
    return
  }
  if (tokenCheckState.value.status === 'duplicate') {
    ElMessage.error(`Bot Token 已被成员「${tokenCheckState.value.usedBy}」使用，请更换`)
    return
  }
  channelSaving.value = true
  try {
    const newConfig: Record<string, string> = {}
    if (channelForm.value.type === 'telegram') {
      if (channelForm.value.botToken) newConfig.botToken = channelForm.value.botToken
      if (channelForm.value.allowedFrom) newConfig.allowedFrom = channelForm.value.allowedFrom
    } else if (channelForm.value.type === 'web') {
      if (channelForm.value.webPassword) newConfig.password = channelForm.value.webPassword
      if (channelForm.value.webWelcome) newConfig.welcomeMsg = channelForm.value.webWelcome
      if (channelForm.value.webTitle) newConfig.title = channelForm.value.webTitle
    }

    if (channelEditingId.value) {
      // Update existing
      const list = agentChannelList.value.map(ch => {
        if (ch.id !== channelEditingId.value) return ch
        return { ...ch, name: channelForm.value.name, type: channelForm.value.type, enabled: channelForm.value.enabled, config: { ...ch.config, ...newConfig } }
      })
      await agentChannelsApi.set(agentId, list)
    } else {
      // Add new
      const newEntry: ChannelEntry = {
        id: pendingChannelId.value || genChannelId(channelForm.value.type),
        name: channelForm.value.name,
        type: channelForm.value.type,
        enabled: channelForm.value.enabled,
        config: newConfig,
        status: 'untested',
      }
      await agentChannelsApi.set(agentId, [...agentChannelList.value, newEntry])
    }
    ElMessage.success(channelForm.value.type === 'web' ? '保存成功，Web 聊天页立即生效' : '保存成功，重启后新渠道生效')
    channelDialogVisible.value = false
    await loadAgentChannels()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    channelSaving.value = false
  }
}

async function saveChannels() {
  try {
    await agentChannelsApi.set(agentId, agentChannelList.value)
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
    await loadAgentChannels() // revert UI state on error
  }
}

async function deleteAgentChannel(row: ChannelEntry) {
  const updated = agentChannelList.value.filter(ch => ch.id !== row.id)
  try {
    await agentChannelsApi.set(agentId, updated)
    agentChannelList.value = updated
    ElMessage.success('已删除')
  } catch {
    ElMessage.error('删除失败')
  }
}

async function testAgentChannel(row: ChannelEntry) {
  testingChannelId.value = row.id
  try {
    const res = await agentChannelsApi.test(agentId, row.id)
    if (res.data.valid) {
      ElMessage.success(res.data.botName ? `测试成功 (@${res.data.botName})` : '测试成功')
    } else {
      ElMessage.error(res.data.error || '测试失败')
    }
    await loadAgentChannels()
  } catch {
    ElMessage.error('测试请求失败')
  } finally {
    testingChannelId.value = ''
  }
}

// ── Pending users (待审核用户) ────────────────────────────────────────────
const pendingUsers = ref<Record<string, PendingUser[]>>({})
const pendingLoading = ref<Record<string, boolean>>({})
const expandedPending = ref<string>('')
const allowingUserId = ref('')

async function loadPendingUsers(chId: string) {
  pendingLoading.value[chId] = true
  try {
    const res = await agentChannelsApi.listPending(agentId, chId)
    pendingUsers.value[chId] = res.data || []
  } catch {
    pendingUsers.value[chId] = []
  } finally {
    pendingLoading.value[chId] = false
  }
}

function togglePending(chId: string) {
  if (expandedPending.value === chId) {
    expandedPending.value = ''
  } else {
    expandedPending.value = chId
    loadPendingUsers(chId)
  }
}

async function allowUser(chId: string, userId: number) {
  allowingUserId.value = `${chId}-${userId}`
  try {
    await agentChannelsApi.allowUser(agentId, chId, userId)
    ElMessage.success(`用户 ${userId} 已加入白名单`)
    await loadPendingUsers(chId)
    await loadAgentChannels() // refresh allowedFrom display
  } catch {
    ElMessage.error('操作失败')
  } finally {
    allowingUserId.value = ''
  }
}

async function dismissUser(chId: string, userId: number) {
  try {
    await agentChannelsApi.dismissUser(agentId, chId, userId)
    ElMessage.success('已忽略')
    await loadPendingUsers(chId)
  } catch {
    ElMessage.error('操作失败')
  }
}

async function removeAllowed(chId: string, userId: number) {
  try {
    await ElMessageBox.confirm(
      `确定将用户 ${userId} 从白名单中移除？移除后该用户将无法使用此 Bot。`,
      '移除白名单',
      { confirmButtonText: '确认移除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return // user cancelled
  }
  try {
    await agentChannelsApi.removeAllowed(agentId, chId, userId)
    ElMessage.success(`用户 ${userId} 已从白名单移除`)
    await loadAgentChannels()
  } catch {
    ElMessage.error('操作失败')
  }
}

onMounted(async () => {
  try {
    const res = await agentsApi.get(agentId)
    agent.value = res.data
  } catch {
    ElMessage.error('加载 Agent 失败')
  }
  loadIdentityFiles()
  loadModels()
  loadRelations()
  loadOtherAgents()
  loadMemConfig()
  loadWorkspace()
  loadCron()
  loadAgentChannels()
  await loadAgentSessions()

  // Handle ?tab=<name> query param (e.g. from CronView "查看" button)
  const tabParam = route.query.tab as string | undefined
  if (tabParam) activeTab.value = tabParam

  // Handle ?resumeSession=<id> query param (from ChatsView 继续对话 button)
  const resumeId = route.query.resumeSession as string | undefined
  if (resumeId) {
    activeSessionId.value = resumeId
    // Give AiChat a tick to mount before calling resumeSession
    await new Promise(r => setTimeout(r, 100))
    aiChatRef.value?.resumeSession(resumeId)
    // Scroll the sidebar item into view by highlighting
    const target = agentSessions.value.find(s => s.id === resumeId)
    if (!target) {
      // Session not in list yet — still set active id so it highlights when list loads
      activeSessionId.value = resumeId
    }
  }
})

// Identity files
async function loadIdentityFiles() {
  try {
    const [id, soul] = await Promise.all([
      filesApi.read(agentId, 'IDENTITY.md'),
      filesApi.read(agentId, 'SOUL.md'),
    ])
    identityContent.value = id.data?.content || ''
    soulContent.value = soul.data?.content || ''
  } catch {}
  loadMemoryTree()
}

async function saveFile(name: string, content: string) {
  try {
    await filesApi.write(agentId, name, content)
    ElMessage.success(`${name} 已保存`)
  } catch {
    ElMessage.error(`保存 ${name} 失败`)
  }
}

async function loadModels() {
  try {
    const res = await modelsApi.list()
    modelList.value = res.data || []
    // Init selector from current agent
    if (agent.value?.modelId) {
      agentModelId.value = agent.value.modelId
    } else {
      // Try to match by model string
      const matched = modelList.value.find(m => m.provider + '/' + m.model === agent.value?.model || m.id === agent.value?.model)
      agentModelId.value = matched?.id || ''
    }
  } catch {}
}

async function saveAgentModel() {
  if (!agentModelId.value) return
  agentModelSaving.value = true
  try {
    const res = await agentsApi.update(agentId, { modelId: agentModelId.value })
    agent.value = res.data
    ElMessage.success('模型已更新')
  } catch {
    ElMessage.error('更新失败')
  } finally {
    agentModelSaving.value = false
  }
}

// Memory tree functions
async function loadMemoryTree() {
  try {
    const res = await memoryApi.tree(agentId)
    memoryTreeData.value = res.data || []
  } catch {
    memoryTreeData.value = []
  }
}

async function handleMemoryNodeClick(data: any) {
  if (data.isDir) return
  memoryEditPath.value = data.path
  memoryFileBreadcrumb.value = data.path.split('/')
  try {
    const res = await memoryApi.readFile(agentId, data.path)
    memoryEditContent.value = res.data?.content || ''
  } catch {
    memoryEditContent.value = '(无法读取)'
  }
}

async function saveMemoryFile() {
  if (!memoryEditPath.value) return
  memorySaving.value = true
  try {
    await memoryApi.writeFile(agentId, memoryEditPath.value, memoryEditContent.value)
    ElMessage.success('记忆文件已保存')
    loadMemoryTree()
  } catch {
    ElMessage.error('保存失败')
  } finally {
    memorySaving.value = false
  }
}

async function createMemoryFile() {
  const p = newMemoryPath.value.trim()
  if (!p) { ElMessage.warning('请输入路径'); return }
  try {
    await memoryApi.writeFile(agentId, p, `# ${p.split('/').pop()?.replace('.md', '') || 'New File'}\n\n`)
    ElMessage.success('文件已创建')
    showNewMemoryFile.value = false
    newMemoryPath.value = ''
    loadMemoryTree()
    // Open the new file
    memoryEditPath.value = p
    memoryFileBreadcrumb.value = p.split('/')
    memoryEditContent.value = `# ${p.split('/').pop()?.replace('.md', '') || 'New File'}\n\n`
  } catch {
    ElMessage.error('创建失败')
  }
}

async function submitDailyEntry() {
  const content = dailyEntryContent.value.trim()
  if (!content) { ElMessage.warning('请输入内容'); return }
  try {
    await memoryApi.dailyLog(agentId, content)
    ElMessage.success('日志已添加')
    showDailyEntry.value = false
    dailyEntryContent.value = ''
    loadMemoryTree()
  } catch {
    ElMessage.error('添加失败')
  }
}

// Workspace
async function loadWorkspace() {
  try {
    const res = await filesApi.readTree(agentId)
    if (Array.isArray(res.data)) {
      fileTreeData.value = res.data as FileNode[]
    }
  } catch {}
}

async function handleFileClick(data: any) {
  if (data.isDir) return
  currentFile.value = data.path || data.name
  currentFileInfo.value = data
  currentFileBinary.value = false
  try {
    const res = await filesApi.read(agentId, currentFile.value)
    if (res.data?.encoding === 'base64') {
      currentFileBinary.value = true
      currentFileContent.value = `[二进制文件 ${formatSize(res.data.size)}]`
    } else {
      currentFileContent.value = res.data?.content ?? ''
    }
  } catch {
    currentFileContent.value = ''
  }
}

async function deleteCurrentFile() {
  if (!currentFile.value) return
  try {
    await ElMessageBox.confirm(`删除「${currentFile.value}」？此操作不可恢复`, '删除文件', {
      confirmButtonText: '确认删除', cancelButtonText: '取消',
      type: 'warning', confirmButtonClass: 'el-button--danger',
    })
    await filesApi.delete(agentId, currentFile.value)
    ElMessage.success('已删除')
    currentFile.value = ''
    currentFileContent.value = ''
    currentFileInfo.value = null
    loadWorkspace()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error('删除失败')
  }
}

async function createNewFile() {
  const path = newFilePath.value.trim()
  if (!path) return
  try {
    await filesApi.write(agentId, path, '')
    ElMessage.success(`已创建 ${path}`)
    showNewFileDialog.value = false
    newFilePath.value = ''
    await loadWorkspace()
    // Auto-open the new file
    currentFile.value = path
    currentFileContent.value = ''
    currentFileInfo.value = null
    currentFileBinary.value = false
  } catch (e: any) {
    ElMessage.error('创建失败')
  }
}

async function saveCurrentFile() {
  if (!currentFile.value) return
  await saveFile(currentFile.value, currentFileContent.value)
}

// Cron
async function loadCron() {
  try {
    // Only load this agent's own cron jobs
    const res = await cronApi.list(agentId)
    cronJobs.value = res.data || []
  } catch {}
}

async function createCron() {
  try {
    await cronApi.create({
      name: cronForm.value.name,
      enabled: cronForm.value.enabled,
      agentId: agentId,  // bind to this agent
      schedule: { kind: 'cron', expr: cronForm.value.expr, tz: cronForm.value.tz },
      payload: { kind: 'agentTurn', message: cronForm.value.message },
      delivery: { mode: 'announce' },
    } as any)
    ElMessage.success('任务创建成功')
    showCronCreate.value = false
    loadCron()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '创建失败')
  }
}

async function toggleCron(job: any) {
  try {
    await cronApi.update(job.id, job)
  } catch {
    ElMessage.error('更新失败')
  }
}

async function runCronNow(job: any) {
  try {
    await cronApi.run(job.id)
    ElMessage.success('已触发运行')
    setTimeout(loadCron, 2000)
  } catch {
    ElMessage.error('运行失败')
  }
}

async function deleteCron(job: any) {
  try {
    await cronApi.delete(job.id)
    ElMessage.success('已删除')
    loadCron()
  } catch {
    ElMessage.error('删除失败')
  }
}

// ── Conv Log Management ──────────────────────────────────────────────────────
const convLoading = ref(false)
const convChannels = ref<ChannelSummary[]>([])
const convDrawerVisible = ref(false)
const convDrawerChannelId = ref('')
const convMessages = ref<ConvEntry[]>([])
const convTotal = ref(0)
const convOffset = ref(0)
const convHasMore = computed(() => convMessages.value.length < convTotal.value)
const convMsgLoading = ref(false)
const convPageSize = 50







async function loadConvChannels() {
  convLoading.value = true
  try {
    const res = await agentConversations.list(agentId)
    convChannels.value = res.data
  } catch {
    ElMessage.error('加载对话渠道失败')
  } finally {
    convLoading.value = false
  }
}

async function openConvDrawer(ch: ChannelSummary) {
  convDrawerChannelId.value = ch.channelId
  convMessages.value = []
  convTotal.value = 0
  convOffset.value = 0
  convDrawerVisible.value = true
  await fetchConvMessages()
}

async function fetchConvMessages() {
  convMsgLoading.value = true
  try {
    const res = await agentConversations.messages(agentId, convDrawerChannelId.value, {
      limit: convPageSize,
      offset: convOffset.value,
    })
    const data = res.data
    convTotal.value = data.total
    // Append to existing list (newer messages already shown, these are older)
    if (convOffset.value === 0) {
      convMessages.value = data.messages
    } else {
      convMessages.value = [...data.messages, ...convMessages.value]
    }
    convOffset.value += data.messages.length
  } catch {
    ElMessage.error('加载消息失败')
  } finally {
    convMsgLoading.value = false
  }
}

async function loadMoreConvMsgs() {
  // Load older messages: offset is current count, go backwards
  // Since JSONL is oldest-first and we display oldest-first, we need to load from the beginning
  // with a different offset strategy. We load page by page from offset 0 incrementally.
  await fetchConvMessages()
}

// Load conv channels when tab is activated
watch(activeTab, (tab) => {
  if (tab === 'convlogs' && convChannels.value.length === 0) {
    loadConvChannels()
  }
})
</script>

<style scoped>
.agent-detail {
  min-height: 100vh;
  background: #f5f7fa;
}
.detail-header {
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
}
.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}
.header-left h2 { margin: 0; }

/* Chat */
.chat-container {
  display: flex;
  flex-direction: column;
  height: 600px;
}
.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  background: #fafafa;
  border-radius: 8px;
}
.chat-msg {
  display: flex;
  margin-bottom: 12px;
}
.chat-msg.user {
  justify-content: flex-end;
}
.chat-msg.assistant, .chat-msg.tool {
  justify-content: flex-start;
}
.msg-bubble {
  max-width: 70%;
  padding: 10px 14px;
  border-radius: 12px;
  font-size: 14px;
  line-height: 1.6;
}
.chat-msg.user .msg-bubble {
  background: #409EFF;
  color: #fff;
  border-bottom-right-radius: 4px;
}
.chat-msg.assistant .msg-bubble {
  background: #fff;
  border: 1px solid #e4e7ed;
  border-bottom-left-radius: 4px;
}
.chat-msg.tool .msg-bubble {
  background: #f0f9eb;
  border: 1px solid #e1f3d8;
  width: 100%;
  max-width: 100%;
}
.tool-block { font-size: 13px; }
.tool-result { white-space: pre-wrap; font-size: 12px; max-height: 200px; overflow-y: auto; }
.cursor { animation: blink 1s infinite; color: #409EFF; }
@keyframes blink { 0%,100% { opacity: 1 } 50% { opacity: 0 } }

/* Typing indicator */
.typing-indicator {
  display: flex;
  gap: 4px;
  padding: 4px 0;
}
.typing-indicator span {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #909399;
  animation: typing 1.4s infinite;
}
.typing-indicator span:nth-child(2) { animation-delay: 0.2s; }
.typing-indicator span:nth-child(3) { animation-delay: 0.4s; }
@keyframes typing {
  0%, 100% { opacity: 0.3; transform: scale(0.8); }
  50% { opacity: 1; transform: scale(1); }
}

.chat-input {
  padding: 12px 0 0;
}
.msg-text :deep(code) {
  background: rgba(0,0,0,0.06);
  padding: 2px 4px;
  border-radius: 3px;
  font-size: 13px;
}

/* Memory timeline */
.memory-card {
  cursor: pointer;
}
.memory-card:hover {
  border-color: #409EFF;
}

/* Chat + Session sidebar layout */
.chat-layout {
  display: flex;
  gap: 0;
  height: calc(100vh - 145px);
  overflow: hidden;
}

.session-sidebar {
  width: 220px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  border-right: 1px solid #e4e7ed;
  background: #fafafa;
  overflow: hidden;
}

.session-sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  border-bottom: 1px solid #e4e7ed;
  flex-shrink: 0;
}

.sidebar-title {
  font-size: 13px;
  font-weight: 600;
  color: #303133;
}

.session-list {
  flex: 1;
  overflow-y: auto;
  padding: 6px 4px;
}

.session-item {
  padding: 8px 10px;
  border-radius: 6px;
  cursor: pointer;
  margin-bottom: 2px;
  transition: background 0.15s;
}

.session-item:hover {
  background: #f0f2f5;
}

.session-item.active {
  background: #ecf5ff;
  border-left: 3px solid #409eff;
}

.session-item-title {
  font-size: 13px;
  color: #303133;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 1.4;
  margin-bottom: 4px;
}

.session-item-meta {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: #909399;
}

.session-empty {
  text-align: center;
  color: #c0c4cc;
  font-size: 12px;
  padding: 20px 0;
}

.chat-area {
  flex: 1;
  overflow: hidden;
}

/* @ 其他成员面板 */
.at-panel {
  flex-shrink: 0;
  border-top: 1px solid #e4e7ed;
  padding: 8px 8px 10px;
  background: #f5f7fa;
}

.at-toggle-btn {
  width: 100%;
  justify-content: flex-start;
  color: #909399;
  font-size: 12px;
  border-color: #dcdfe6;
}

.at-toggle-btn:hover {
  color: #409eff;
  border-color: #b3d8ff;
  background: #ecf5ff;
}

.at-icon {
  font-weight: 700;
  color: #409eff;
  margin-right: 2px;
  font-size: 13px;
}

.at-form {
  margin-top: 8px;
}

/* Relations tab */
.relations-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.relation-card {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  background: #fafafa;
}
.relation-avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 700;
  font-size: 14px;
  flex-shrink: 0;
}
.relation-info {
  flex: 1;
  min-width: 0;
}
.relation-name {
  font-weight: 600;
  font-size: 14px;
  color: #303133;
  margin-bottom: 4px;
}
.relation-tags {
  display: flex;
  gap: 6px;
  margin-bottom: 4px;
}
.relation-desc {
  font-size: 12px;
  color: #606266;
  line-height: 1.5;
}

/* Channel cards */
.channel-card {
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  margin-bottom: 16px;
  overflow: hidden;
}
.channel-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  background: #fafafa;
  border-bottom: 1px solid #f0f0f0;
}
.channel-card-left {
  display: flex;
  align-items: center;
}
.channel-card-name {
  font-weight: 600;
  font-size: 14px;
  color: #303133;
}
.channel-bot-username {
  font-size: 12px;
  color: #409eff;
  margin-left: 6px;
}
.channel-card-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}
.channel-card-body {
  padding: 12px 16px;
}
.channel-info-row {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 12px;
}
.channel-info-label {
  font-size: 12px;
  color: #909399;
  width: 72px;
  flex-shrink: 0;
  padding-top: 2px;
}
.channel-info-value {
  flex: 1;
}
.channel-url-preview {
  display: flex;
  align-items: center;
  gap: 8px;
  background: #f5f7fa;
  border: 1px solid #e4e7ed;
  border-radius: 6px;
  padding: 7px 10px;
  width: 100%;
}
.channel-url-text {
  flex: 1;
  font-size: 12px;
  color: #409eff;
  word-break: break-all;
  font-family: monospace;
}
.pending-section {
  border: 1px solid #e4e7ed;
  border-radius: 6px;
  overflow: hidden;
}
.pending-section-header {
  display: flex;
  align-items: center;
  padding: 8px 12px;
  background: #f5f7fa;
  cursor: pointer;
  font-size: 13px;
  color: #606266;
  user-select: none;
}
.pending-section-header:hover {
  background: #ebedf0;
}
.pending-list {
  padding: 8px 0;
}
.pending-user-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-bottom: 1px solid #f5f7fa;
}
.pending-user-row:last-child {
  border-bottom: none;
}
.pending-user-info {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
}
.pending-user-name {
  font-weight: 600;
  font-size: 13px;
  color: #303133;
}
.pending-user-username {
  font-size: 12px;
  color: #409eff;
}
.pending-user-id {
  font-size: 11px;
  color: #909399;
  background: #f5f7fa;
  padding: 1px 6px;
  border-radius: 4px;
}
.pending-user-actions {
  display: flex;
  gap: 4px;
}
.pending-empty {
  padding: 16px 12px;
  text-align: center;
  font-size: 12px;
  color: #c0c4cc;
}

/* ── Conversation Log Drawer ─────────────────────────────────────────── */
.conv-drawer-body {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 0 4px;
}
.conv-msg-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.conv-msg-item {
  display: flex;
  flex-direction: column;
  max-width: 90%;
}
.conv-msg-user {
  align-self: flex-end;
  align-items: flex-end;
}
.conv-msg-assistant {
  align-self: flex-start;
  align-items: flex-start;
}
.conv-msg-meta {
  display: flex;
  gap: 6px;
  align-items: center;
  margin-bottom: 4px;
  font-size: 11px;
  color: #909399;
}
.conv-msg-role {
  font-weight: 600;
}
.conv-msg-sender {
  color: #409eff;
}
.conv-msg-time {
  color: #c0c4cc;
}
.conv-msg-content {
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 13px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}
.conv-msg-user .conv-msg-content {
  background: #409eff;
  color: #fff;
}
.conv-msg-assistant .conv-msg-content {
  background: #f4f4f5;
  color: #303133;
}
.conv-msg-empty {
  text-align: center;
  color: #c0c4cc;
  padding: 32px 0;
  font-size: 13px;
}
</style>
