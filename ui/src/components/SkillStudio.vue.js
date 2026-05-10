/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed, onMounted, watch, nextTick } from 'vue';
import { ElMessage } from 'element-plus';
import { agentSkills as skillsApi, files as filesApi } from '../api';
import AiChat from './AiChat.vue';
const props = defineProps();
const agentId = props.agentId;
// ── Panel resize ──────────────────────────────────────────────────────────
const sideW = ref(200); // sidebar width
const treeW = ref(140); // file tree width
const chatW = ref(340); // chat panel width
const dragging = ref('');
function startResize(e, target) {
    const startX = e.clientX;
    const startW = target === 'side' ? sideW.value : target === 'tree' ? treeW.value : chatW.value;
    dragging.value = target;
    const onMove = (ev) => {
        const d = ev.clientX - startX;
        if (target === 'side')
            sideW.value = Math.max(140, Math.min(340, startW + d));
        else if (target === 'tree')
            treeW.value = Math.max(100, Math.min(280, startW + d));
        else
            chatW.value = Math.max(240, Math.min(560, startW - d));
    };
    const onUp = () => {
        dragging.value = '';
        window.removeEventListener('mousemove', onMove);
        window.removeEventListener('mouseup', onUp);
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
    };
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
    document.body.style.cssText += 'cursor:col-resize;user-select:none;';
}
// ── State ──────────────────────────────────────────────────────────────────
const skills = ref([]);
const listLoading = ref(false);
const selected = ref(null);
const activeFile = ref('meta');
const editorFullscreen = ref(false); // 全屏编辑模式（隐藏右侧 AI 聊天）
const pendingEdit = ref(null);
const pendingEditStats = computed(() => {
    if (!pendingEdit.value)
        return '';
    const oldLines = (pendingEdit.value.file === 'SKILL.md' ? promptContent.value : genericContent.value).split('\n');
    const newLines = pendingEdit.value.content.split('\n');
    const added = newLines.filter(l => !oldLines.includes(l)).length;
    const removed = oldLines.filter(l => !newLines.includes(l)).length;
    return `+${added} / -${removed} 行`;
});
function applyPendingEdit() {
    if (!pendingEdit.value)
        return;
    if (pendingEdit.value.file === 'SKILL.md') {
        promptContent.value = pendingEdit.value.content;
        promptDirty.value = true;
        activeFile.value = 'prompt';
    }
    else {
        genericContent.value = pendingEdit.value.content;
        genericDirty.value = true;
        activeFile.value = pendingEdit.value.file;
    }
    ElMessage.success('已应用 AI 修改');
    pendingEdit.value = null;
}
// Metadata form (mirrors selected skill)
const metaForm = ref({ name: '', icon: '', category: '', description: '', version: '1.0.0', enabled: true });
// SKILL.md
const promptContent = ref('');
const promptLoading = ref(false);
const promptDirty = ref(false);
const promptLineCount = computed(() => Math.max(1, promptContent.value.split('\n').length));
const saving = ref(false);
// Create
const creating = ref(false);
const isNewSkill = ref(false); // true when just created — AI should guide user
const dirFiles = ref([]);
const dirLoading = ref(false);
// ── 目录展开/收起 ──────────────────────────────────────────────────────────
const collapsedDirs = ref(new Set());
function toggleDir(path) {
    if (collapsedDirs.value.has(path))
        collapsedDirs.value.delete(path);
    else
        collapsedDirs.value.add(path);
    // Trigger reactivity
    collapsedDirs.value = new Set(collapsedDirs.value);
}
const visibleFiles = computed(() => dirFiles.value.filter(f => {
    const parts = f.path.split('/');
    for (let i = 1; i < parts.length; i++) {
        if (collapsedDirs.value.has(parts.slice(0, i).join('/')))
            return false;
    }
    return true;
}));
// ── 新建文件/目录 ──────────────────────────────────────────────────────────
const newEntryDialog = ref({ visible: false, isDir: false, inDir: '', name: '', creating: false });
function openNewFileDialog(inDir) {
    newEntryDialog.value = { visible: true, isDir: false, inDir, name: '', creating: false };
}
function openNewDirDialog(inDir) {
    newEntryDialog.value = { visible: true, isDir: true, inDir, name: '', creating: false };
}
async function createEntry() {
    const { isDir, inDir, name } = newEntryDialog.value;
    if (!name.trim() || !selected.value)
        return;
    newEntryDialog.value.creating = true;
    const relPath = inDir ? `${inDir}/${name.trim()}` : name.trim();
    const skillBase = `skills/${selected.value.id}`;
    try {
        if (isDir) {
            // Create a .gitkeep placeholder so the directory exists
            await filesApi.write(agentId, `${skillBase}/${relPath}/.gitkeep`, '');
        }
        else {
            await filesApi.write(agentId, `${skillBase}/${relPath}`, '');
        }
        newEntryDialog.value.visible = false;
        await loadDirFiles();
        if (!isDir) {
            // Auto-open the new file
            await openFile(relPath, false);
        }
        else {
            // Auto-expand the new dir
            collapsedDirs.value.delete(relPath);
        }
        ElMessage.success(`${isDir ? '目录' : '文件'} ${relPath} 已创建`);
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '创建失败');
    }
    finally {
        newEntryDialog.value.creating = false;
    }
}
// ── 重命名文件 ──────────────────────────────────────────────────────────────
const renameDialog = ref({ visible: false, oldPath: '', newName: '', saving: false });
function openRenameDialog(path) {
    const parts = path.split('/');
    renameDialog.value = { visible: true, oldPath: path, newName: parts[parts.length - 1] ?? '', saving: false };
}
async function doRename() {
    const { oldPath, newName } = renameDialog.value;
    if (!newName.trim() || !selected.value)
        return;
    renameDialog.value.saving = true;
    const parts = oldPath.split('/');
    const dir = parts.slice(0, -1).join('/');
    const newPath = dir ? `${dir}/${newName.trim()}` : newName.trim();
    const skillBase = `skills/${selected.value.id}`;
    try {
        // Read old, write new, delete old
        const res = await filesApi.read(agentId, `${skillBase}/${oldPath}`);
        await filesApi.write(agentId, `${skillBase}/${newPath}`, res.data?.content || '');
        await filesApi.delete(agentId, `${skillBase}/${oldPath}`);
        renameDialog.value.visible = false;
        if (activeFile.value === oldPath)
            activeFile.value = newPath === 'SKILL.md' ? 'prompt' : newPath;
        await loadDirFiles();
        ElMessage.success('重命名成功');
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '重命名失败');
    }
    finally {
        renameDialog.value.saving = false;
    }
}
// Generic file editor (for non-skill.json / non-SKILL.md files)
const genericContent = ref('');
const genericDirty = ref(false);
const genericLoading = ref(false);
// 每个 skill 独立的 AiChat 实例（支持并发后台生成）
const chatRefsMap = {};
function setChatRef(skillId, el) {
    if (el)
        chatRefsMap[skillId] = el;
    else
        delete chatRefsMap[skillId];
}
function getChatRef(skillId) {
    return skillId ? chatRefsMap[skillId] : null;
}
// 正在流式生成的 skill 集合（用于 UI 指示器）
const streamingSkills = ref(new Set());
function onStreamingChange(skillId, streaming) {
    const next = new Set(streamingSkills.value);
    if (streaming)
        next.add(skillId);
    else
        next.delete(skillId);
    streamingSkills.value = next;
}
// 已初始化过 session 的 skill 集合
const initializedSessions = ref(new Set());
// 当选中技能变化时，首次初始化其 chat session
watch(selected, async (sk) => {
    if (!sk)
        return;
    if (initializedSessions.value.has(sk.id))
        return;
    initializedSessions.value.add(sk.id);
    await nextTick(); // 等 DOM 渲染出对应的 AiChat 实例
    await getChatRef(sk.id)?.resumeSession?.(`skill-studio-${sk.id}`);
}, { flush: 'post' });
// ── AI Chat context ────────────────────────────────────────────────────────
const chatContext = computed(() => {
    if (!selected.value)
        return '你是一个技能架构师，帮助用户设计和生成完整的 AI 技能包。';
    const sid = selected.value.id;
    const base = `skills/${sid}`;
    const currentFiles = dirFiles.value.map(f => f.path).join(', ') || '（空）';
    const skillState = selected.value.name ? `名称: ${selected.value.name}，分类: ${selected.value.category || '未设置'}` : '（新建，尚未配置）';
    return `你是一个专业的技能架构师，负责为 AI 成员生成完整、规范的技能包。
当前技能目录: ${base}/（技能 ID: ${sid}）
当前技能状态: ${skillState}
目录中已有文件: ${currentFiles}

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
## 一、技能包标准结构

一个完整的技能包包含以下文件：

${'```'}
skills/{id}/
├── SKILL.md          # 核心：注入 AI System Prompt 的指令（必须）
├── skill.json        # 元数据：名称/图标/分类/描述（自动管理，无需手写）
└── tools/            # 可选：工具脚本（仅需外部计算/数据处理时创建）
    ├── main.py       # 工具入口
    └── README.md     # 工具说明
${'```'}

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
## 二、SKILL.md 渐进式披露规范

**核心原则：按层次组织，让 LLM 先理解角色，再理解规则，最后才处理细节。**

SKILL.md 标准分层结构（越往后的章节越少被触发）：

${'```'}markdown
# [技能名称]

## 🎯 角色
[一句话定义：你是谁，核心使命是什么]

## ⚡ 核心能力
- 能力1：[简洁描述]
- 能力2：[简洁描述]
- 能力3：[简洁描述]（不超过5条）

## 📋 工作流程
[遇到任务时的标准思路/步骤，3-6步]

## 📐 输出规范
[格式要求：结构、语言风格、长度]

## ⚠️ 边界规则
[什么情况下拒绝 / 降级 / 澄清]

## 🔧 工具使用（可选，仅需工具时加此章节）
[工具调用规范和参数说明]
${'```'}

**写作要点：**
- 角色 + 核心能力：简短有力，LLM 每次都读
- 工作流程：结构化步骤，触发频率高
- 输出规范 / 边界规则：只在需要时展开，避免冗余
- 避免重复：同一信息只在一个章节出现

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
## 三、⚠️ 生成技能的完整流程（每次创建/重新生成必须全部执行，缺一不可）

当用户描述一个技能需求，**严格按以下顺序**：

**步骤1：【必须】先输出 fill_skill JSON 填写元数据**
直接在回复开头输出，不要用代码块包裹，格式如下：
{"action":"fill_skill","data":{"name":"利润表审核分析","icon":"📊","category":"财税审核","description":"分析企业利润表，识别异常数据，给出专业税务审核意见","enabled":true}}

**步骤2：【必须】write 工具写入 SKILL.md**
路径：\`${base}/SKILL.md\`
按渐进式披露规范写完整内容（不要在聊天中输出内容，直接写文件）。

**步骤3：按需创建工具文件**（仅需外部计算/数据处理时）
如需工具，用 write 工具创建 \`${base}/tools/\` 下的脚本文件。

**重要：步骤1和步骤2必须都完成，不能只做一个。**

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
## 四、修改技能

- **修改 SKILL.md**：先用 read 工具读取当前内容，理解后再用 write 工具写回
- **新增工具文件**：直接 write 到对应路径
- **优化提示词**：遵循渐进式披露，减少冗余
- **所有操作直接用工具完成，不要把内容输出给用户复制**

${promptContent.value && promptContent.value.length <= 1200
        ? `\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n## 当前 SKILL.md 内容\n\`\`\`markdown\n${promptContent.value}\n\`\`\``
        : promptContent.value ? `\n当前 SKILL.md 已有内容（${promptContent.value.length} 字符），如需修改请先 read 工具读取。` : ''}`;
});
const chatWelcome = computed(() => {
    if (!selected.value)
        return '选择一个技能后，我可以帮你一键生成完整技能包（元数据 + SKILL.md + 工具文件）。';
    if (isNewSkill.value)
        return `新技能已创建（ID: ${selected.value.id}）。\n\n告诉我这个技能要做什么，我会**一次性生成完整技能包**：自动填写名称/图标/描述，写入规范的 SKILL.md（渐进式披露结构），如需工具也一并创建。`;
    return `当前技能：「${selected.value.name || selected.value.id}」\n\n告诉我需要如何调整——我会直接用工具修改对应文件，不会让你手动复制内容。`;
});
const chatExamples = computed(() => {
    if (!selected.value)
        return ['帮我生成一个财务报表审核技能', '帮我设计一个代码审查技能'];
    if (isNewSkill.value)
        return [
            '生成完整技能包',
            '这个技能需要能分析利润表，识别异常数据，给出审核意见',
        ];
    return [
        `重新生成完整的 ${selected.value.name} 技能包`,
        '优化 SKILL.md 的渐进式披露结构',
        '为这个技能添加 Python 工具脚本',
    ];
});
// ── Load ───────────────────────────────────────────────────────────────────
async function loadList() {
    listLoading.value = true;
    try {
        const res = await skillsApi.list(agentId);
        skills.value = res.data || [];
        // Keep selected in sync
        if (selected.value) {
            const updated = skills.value.find(s => s.id === selected.value.id);
            if (updated) {
                selected.value = updated;
                syncMetaForm(updated);
            }
        }
    }
    catch { /* silent */ }
    finally {
        listLoading.value = false;
    }
}
function syncMetaForm(sk) {
    metaForm.value = {
        name: sk.name, icon: sk.icon || '', category: sk.category || '',
        description: sk.description || '', version: sk.version || '1.0.0', enabled: sk.enabled,
    };
}
async function selectSkill(sk) {
    // 已选中同一个技能：跳过
    if (selected.value?.id === sk.id)
        return;
    // 切换编辑器视图（立即生效，不影响任何 AiChat 的流）
    selected.value = sk;
    syncMetaForm(sk);
    activeFile.value = 'meta';
    promptDirty.value = false;
    promptContent.value = '';
    isNewSkill.value = false;
    loadDirFiles();
    reloadPrompt();
    // session 初始化由 watch(selected) 处理（首次选中时）
}
async function switchToPrompt() {
    if (!selected.value)
        return;
    if (activeFile.value === 'prompt')
        return;
    activeFile.value = 'prompt';
    if (!promptContent.value)
        await reloadPrompt();
}
// 递归读取目录，返回扁平列表（含深度和相对 path）
async function readDirRecursive(apiPath, relPrefix, depth) {
    const res = await filesApi.read(agentId, apiPath);
    const entries = Array.isArray(res.data) ? res.data : [];
    const result = [];
    for (const f of entries) {
        if (depth === 0 && f.name === 'skill.json')
            continue; // skill.json 固定显示，跳过
        const relPath = relPrefix ? `${relPrefix}/${f.name}` : f.name;
        result.push({ name: f.name, path: relPath, isDir: f.isDir, depth });
        if (f.isDir) {
            const children = await readDirRecursive(`skills/${selected.value.id}/${relPath}`, relPath, depth + 1);
            result.push(...children);
        }
    }
    return result;
}
async function loadDirFiles() {
    if (!selected.value)
        return;
    dirLoading.value = true;
    try {
        dirFiles.value = await readDirRecursive(`skills/${selected.value.id}/`, '', 0);
    }
    catch {
        dirFiles.value = [{ name: 'SKILL.md', path: 'SKILL.md', isDir: false, depth: 0 }];
    }
    finally {
        dirLoading.value = false;
    }
}
// path = 相对于 skills/{skillId}/ 的路径，如 "SKILL.md" 或 "tools/eda.py"
async function openFile(path, isDir) {
    if (isDir)
        return; // 目录不可打开
    if (path === 'SKILL.md') {
        await switchToPrompt();
        return;
    }
    activeFile.value = path;
    genericDirty.value = false;
    await reloadGenericFile();
}
async function reloadGenericFile() {
    if (!selected.value || !activeFile.value || activeFile.value === 'meta' || activeFile.value === 'prompt')
        return;
    genericLoading.value = true;
    try {
        const res = await filesApi.read(agentId, `skills/${selected.value.id}/${activeFile.value}`);
        genericContent.value = res.data?.content || '';
        genericDirty.value = false;
    }
    catch {
        genericContent.value = '';
    }
    finally {
        genericLoading.value = false;
    }
}
async function deleteFile(path) {
    if (!selected.value)
        return;
    try {
        await filesApi.delete(agentId, `skills/${selected.value.id}/${path}`);
        if (activeFile.value === path)
            activeFile.value = 'prompt';
        await loadDirFiles();
        ElMessage.success('已删除');
    }
    catch {
        ElMessage.error('删除失败');
    }
}
// ── Save ───────────────────────────────────────────────────────────────────
async function saveSkill() {
    if (!selected.value)
        return;
    saving.value = true;
    try {
        if (activeFile.value === 'meta' || activeFile.value === 'prompt') {
            // Save metadata
            await skillsApi.update(props.agentId, selected.value.id, {
                name: metaForm.value.name,
                icon: metaForm.value.icon,
                category: metaForm.value.category,
                description: metaForm.value.description,
                enabled: metaForm.value.enabled,
            });
            // Save SKILL.md if in prompt mode or if content was loaded
            if (activeFile.value === 'prompt' || promptContent.value) {
                await filesApi.write(agentId, `skills/${selected.value.id}/SKILL.md`, promptContent.value);
                promptDirty.value = false;
            }
            await loadList();
        }
        else {
            // 通用文件保存
            await filesApi.write(agentId, `skills/${selected.value.id}/${activeFile.value}`, genericContent.value);
            genericDirty.value = false;
        }
        ElMessage.success('保存成功');
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '保存失败');
    }
    finally {
        saving.value = false;
    }
}
// ── Toggle ─────────────────────────────────────────────────────────────────
async function toggleSkill(sk, enabled) {
    try {
        await skillsApi.update(props.agentId, sk.id, { enabled });
        await loadList();
    }
    catch {
        ElMessage.error('操作失败');
    }
}
// ── Delete ─────────────────────────────────────────────────────────────────
async function deleteSkill() {
    if (!selected.value)
        return;
    try {
        await skillsApi.remove(props.agentId, selected.value.id);
        ElMessage.success('已删除');
        selected.value = null;
        await loadList();
    }
    catch {
        ElMessage.error('删除失败');
    }
}
// ── Create ─────────────────────────────────────────────────────────────────
// 直接在左侧新增空白技能，无弹窗
async function openNew() {
    if (creating.value)
        return;
    creating.value = true;
    // 生成唯一 ID：skill_ + base36 timestamp
    const id = 'skill_' + Date.now().toString(36);
    try {
        await skillsApi.create(props.agentId, {
            meta: {
                id, name: '新技能', icon: '', category: '', description: '',
                version: '1.0.0', enabled: false, source: 'local', installedAt: '',
            },
            promptContent: '',
        });
        await loadList();
        const sk = skills.value.find(s => s.id === id);
        if (sk) {
            await selectSkill(sk);
            // 直接跳到 SKILL.md 编辑器，引导用户用 AI 生成内容
            activeFile.value = 'prompt';
            promptContent.value = '';
            isNewSkill.value = true;
            // 等 watch(selected) 初始化 session 完成（resumeSession 404→空）
            await nextTick();
            // 确保 initializedSessions 已处理
            if (!initializedSessions.value.has(id)) {
                initializedSessions.value.add(id);
                await getChatRef(id)?.resumeSession?.(`skill-studio-${id}`);
            }
            // 欢迎词已通过 chatWelcome computed + :welcome-message 展示，无需 AI 自动发消息
        }
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '创建失败');
    }
    finally {
        creating.value = false;
    }
}
// ── AI response hook ──────────────────────────────────────────────────────
async function onAiResponse(skillId, text) {
    if (skillId === selected.value?.id)
        isNewSkill.value = false;
    // 尝试解析 fill_skill / edit_file JSON（兼容旧模式 + 备用协议）
    if (skillId === selected.value?.id)
        tryFillSkill(text);
    // 始终刷新编辑器：AI 可能通过 write 工具直接写了文件
    await loadList();
    if (skillId === selected.value?.id) {
        const prevContent = promptContent.value;
        await Promise.all([loadDirFiles(), reloadPrompt()]);
        // 如果 SKILL.md 内容有变化，自动切到编辑器并提示
        if (promptContent.value && promptContent.value !== prevContent) {
            activeFile.value = 'prompt';
            ElMessage.success({ message: 'SKILL.md 已更新', duration: 2000 });
        }
        if (activeFile.value !== 'meta' && activeFile.value !== 'prompt') {
            await reloadGenericFile();
        }
    }
}
// 解析并应用 fill_skill / edit_file JSON
function tryFillSkill(text) {
    const tryApply = (jsonStr) => {
        try {
            const obj = JSON.parse(jsonStr);
            // ── edit_file：AI 直接修改文件内容，显示 diff 预览 ──────────────────
            if (obj.action === 'edit_file' && obj.file && typeof obj.content === 'string') {
                pendingEdit.value = {
                    file: obj.file,
                    content: obj.content,
                    summary: obj.summary || obj.file + ' 内容已更新',
                };
                activeFile.value = obj.file === 'SKILL.md' ? 'prompt' : obj.file;
                return true;
            }
            if (obj.action === 'fill_skill' && obj.data) {
                const d = obj.data;
                if (d.name)
                    metaForm.value.name = d.name;
                if (d.icon)
                    metaForm.value.icon = d.icon;
                if (d.category)
                    metaForm.value.category = d.category;
                if (d.description)
                    metaForm.value.description = d.description;
                if (typeof d.enabled === 'boolean')
                    metaForm.value.enabled = d.enabled;
                if (d.prompt) {
                    promptContent.value = d.prompt;
                    promptDirty.value = true;
                    activeFile.value = 'prompt'; // 自动切到 SKILL.md 编辑器
                }
                ElMessage.success('AI 已填写技能信息，确认后点击保存');
                return true;
            }
        }
        catch { }
        return false;
    };
    // 代码块内
    const codeBlock = text.match(/```(?:json)?\s*(\{[\s\S]*?\})\s*```/);
    if (codeBlock?.[1] && tryApply(codeBlock[1]))
        return true;
    // 裸 JSON (fill_skill / edit_file)
    const bare = text.match(/(\{"action"\s*:\s*"(?:fill_skill|edit_file)"[\s\S]*?\})\s*(?:```|$)/);
    if (bare?.[1] && tryApply(bare[1]))
        return true;
    return false;
}
async function reloadPrompt() {
    if (!selected.value)
        return;
    promptLoading.value = true;
    try {
        const res = await filesApi.read(agentId, `skills/${selected.value.id}/SKILL.md`);
        promptContent.value = res.data?.content || '';
        promptDirty.value = false;
    }
    catch {
        promptContent.value = '';
    }
    finally {
        promptLoading.value = false;
    }
}
// ── Test ───────────────────────────────────────────────────────────────────
async function sendTestToChat() {
    if (!selected.value)
        return;
    // Load SKILL.md if not yet loaded
    if (!promptContent.value)
        await switchToPrompt();
    const testMsg = `请用「${selected.value.name}」技能效果回复：你好，请介绍一下你的功能。`;
    getChatRef(selected.value?.id)?.fillInput?.(testMsg);
    ElMessage.info('测试消息已填入右侧聊天框，点击发送即可测试');
}
onMounted(loadList);
const __VLS_ctx = {
    ...{},
    ...{},
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['skill-item']} */ ;
/** @type {__VLS_StyleScopedClasses['skill-item']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-item']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-item']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-item']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-item']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-dir']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-item']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-item']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-dir-root']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-item']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-item-acts']} */ ;
/** @type {__VLS_StyleScopedClasses['tree-item-acts']} */ ;
/** @type {__VLS_StyleScopedClasses['dir-arrow']} */ ;
/** @type {__VLS_StyleScopedClasses['code-textarea']} */ ;
/** @type {__VLS_StyleScopedClasses['code-textarea']} */ ;
/** @type {__VLS_StyleScopedClasses['ss-handle']} */ ;
/** @type {__VLS_StyleScopedClasses['ss-handle']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "skill-studio" },
});
/** @type {__VLS_StyleScopedClasses['skill-studio']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "studio-sidebar" },
    ...{ style: ({ width: __VLS_ctx.sideW + 'px' }) },
});
/** @type {__VLS_StyleScopedClasses['studio-sidebar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "sidebar-top" },
});
/** @type {__VLS_StyleScopedClasses['sidebar-top']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "sidebar-title" },
});
/** @type {__VLS_StyleScopedClasses['sidebar-title']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "sidebar-acts" },
});
/** @type {__VLS_StyleScopedClasses['sidebar-acts']} */ ;
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
    ...{ 'onClick': {} },
    size: "small",
    loading: (__VLS_ctx.listLoading),
    circle: true,
}));
const __VLS_2 = __VLS_1({
    ...{ 'onClick': {} },
    size: "small",
    loading: (__VLS_ctx.listLoading),
    circle: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_1));
let __VLS_5;
const __VLS_6 = ({ click: {} },
    { onClick: (__VLS_ctx.loadList) });
const { default: __VLS_7 } = __VLS_3.slots;
let __VLS_8;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_9 = __VLS_asFunctionalComponent1(__VLS_8, new __VLS_8({}));
const __VLS_10 = __VLS_9({}, ...__VLS_functionalComponentArgsRest(__VLS_9));
const { default: __VLS_13 } = __VLS_11.slots;
let __VLS_14;
/** @ts-ignore @type { | typeof __VLS_components.Refresh} */
Refresh;
// @ts-ignore
const __VLS_15 = __VLS_asFunctionalComponent1(__VLS_14, new __VLS_14({}));
const __VLS_16 = __VLS_15({}, ...__VLS_functionalComponentArgsRest(__VLS_15));
// @ts-ignore
[sideW, listLoading, loadList,];
var __VLS_11;
// @ts-ignore
[];
var __VLS_3;
var __VLS_4;
let __VLS_19;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_20 = __VLS_asFunctionalComponent1(__VLS_19, new __VLS_19({
    ...{ 'onClick': {} },
    size: "small",
    type: "primary",
    circle: true,
    loading: (__VLS_ctx.creating),
}));
const __VLS_21 = __VLS_20({
    ...{ 'onClick': {} },
    size: "small",
    type: "primary",
    circle: true,
    loading: (__VLS_ctx.creating),
}, ...__VLS_functionalComponentArgsRest(__VLS_20));
let __VLS_24;
const __VLS_25 = ({ click: {} },
    { onClick: (__VLS_ctx.openNew) });
const { default: __VLS_26 } = __VLS_22.slots;
let __VLS_27;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_28 = __VLS_asFunctionalComponent1(__VLS_27, new __VLS_27({}));
const __VLS_29 = __VLS_28({}, ...__VLS_functionalComponentArgsRest(__VLS_28));
const { default: __VLS_32 } = __VLS_30.slots;
let __VLS_33;
/** @ts-ignore @type { | typeof __VLS_components.Plus} */
Plus;
// @ts-ignore
const __VLS_34 = __VLS_asFunctionalComponent1(__VLS_33, new __VLS_33({}));
const __VLS_35 = __VLS_34({}, ...__VLS_functionalComponentArgsRest(__VLS_34));
// @ts-ignore
[creating, openNew,];
var __VLS_30;
// @ts-ignore
[];
var __VLS_22;
var __VLS_23;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "skill-list" },
});
/** @type {__VLS_StyleScopedClasses['skill-list']} */ ;
if (!__VLS_ctx.listLoading && __VLS_ctx.skills.length === 0) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "list-empty" },
    });
    /** @type {__VLS_StyleScopedClasses['list-empty']} */ ;
}
for (const [sk] of __VLS_vFor((__VLS_ctx.skills))) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                __VLS_ctx.selectSkill(sk);
                // @ts-ignore
                [listLoading, skills, skills, selectSkill,];
            } },
        key: (sk.id),
        ...{ class: (['skill-item', { active: __VLS_ctx.selected?.id === sk.id }]) },
    });
    /** @type {__VLS_StyleScopedClasses['active']} */ ;
    /** @type {__VLS_StyleScopedClasses['skill-item']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "sk-icon" },
    });
    /** @type {__VLS_StyleScopedClasses['sk-icon']} */ ;
    if (sk.icon) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
        (sk.icon);
    }
    else {
        let __VLS_38;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_39 = __VLS_asFunctionalComponent1(__VLS_38, new __VLS_38({}));
        const __VLS_40 = __VLS_39({}, ...__VLS_functionalComponentArgsRest(__VLS_39));
        const { default: __VLS_43 } = __VLS_41.slots;
        let __VLS_44;
        /** @ts-ignore @type { | typeof __VLS_components.Tools} */
        Tools;
        // @ts-ignore
        const __VLS_45 = __VLS_asFunctionalComponent1(__VLS_44, new __VLS_44({}));
        const __VLS_46 = __VLS_45({}, ...__VLS_functionalComponentArgsRest(__VLS_45));
        // @ts-ignore
        [selected,];
        var __VLS_41;
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "sk-info" },
    });
    /** @type {__VLS_StyleScopedClasses['sk-info']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "sk-name" },
    });
    /** @type {__VLS_StyleScopedClasses['sk-name']} */ ;
    (sk.name);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "sk-id" },
    });
    /** @type {__VLS_StyleScopedClasses['sk-id']} */ ;
    (sk.id);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "sk-right" },
    });
    /** @type {__VLS_StyleScopedClasses['sk-right']} */ ;
    if (__VLS_ctx.streamingSkills.has(sk.id)) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span)({
            ...{ class: "sk-streaming-dot" },
            title: "AI 生成中…",
        });
        /** @type {__VLS_StyleScopedClasses['sk-streaming-dot']} */ ;
    }
    else if (sk.category) {
        let __VLS_49;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_50 = __VLS_asFunctionalComponent1(__VLS_49, new __VLS_49({
            size: "small",
            effect: "plain",
            ...{ style: {} },
        }));
        const __VLS_51 = __VLS_50({
            size: "small",
            effect: "plain",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_50));
        const { default: __VLS_54 } = __VLS_52.slots;
        (sk.category);
        // @ts-ignore
        [streamingSkills,];
        var __VLS_52;
    }
    let __VLS_55;
    /** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
    elSwitch;
    // @ts-ignore
    const __VLS_56 = __VLS_asFunctionalComponent1(__VLS_55, new __VLS_55({
        ...{ 'onChange': {} },
        ...{ 'onClick': {} },
        modelValue: (sk.enabled),
        size: "small",
    }));
    const __VLS_57 = __VLS_56({
        ...{ 'onChange': {} },
        ...{ 'onClick': {} },
        modelValue: (sk.enabled),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_56));
    let __VLS_60;
    const __VLS_61 = ({ change: {} },
        { onChange: ((v) => __VLS_ctx.toggleSkill(sk, v)) });
    const __VLS_62 = ({ click: {} },
        { onClick: () => { } });
    var __VLS_58;
    var __VLS_59;
    // @ts-ignore
    [toggleSkill,];
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ onMousedown: (...[$event]) => {
            __VLS_ctx.startResize($event, 'side');
            // @ts-ignore
            [startResize,];
        } },
    ...{ class: "ss-handle" },
    ...{ class: ({ dragging: __VLS_ctx.dragging === 'side' }) },
});
/** @type {__VLS_StyleScopedClasses['ss-handle']} */ ;
/** @type {__VLS_StyleScopedClasses['dragging']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div)({
    ...{ class: "ss-handle-bar" },
});
/** @type {__VLS_StyleScopedClasses['ss-handle-bar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "studio-editor" },
});
/** @type {__VLS_StyleScopedClasses['studio-editor']} */ ;
if (!__VLS_ctx.selected) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-empty" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-empty']} */ ;
    let __VLS_63;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_64 = __VLS_asFunctionalComponent1(__VLS_63, new __VLS_63({
        size: "48",
        color: "#c0c4cc",
    }));
    const __VLS_65 = __VLS_64({
        size: "48",
        color: "#c0c4cc",
    }, ...__VLS_functionalComponentArgsRest(__VLS_64));
    const { default: __VLS_68 } = __VLS_66.slots;
    let __VLS_69;
    /** @ts-ignore @type { | typeof __VLS_components.Setting} */
    Setting;
    // @ts-ignore
    const __VLS_70 = __VLS_asFunctionalComponent1(__VLS_69, new __VLS_69({}));
    const __VLS_71 = __VLS_70({}, ...__VLS_functionalComponentArgsRest(__VLS_70));
    // @ts-ignore
    [selected, dragging,];
    var __VLS_66;
    __VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({});
    let __VLS_74;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_75 = __VLS_asFunctionalComponent1(__VLS_74, new __VLS_74({
        ...{ 'onClick': {} },
        type: "primary",
    }));
    const __VLS_76 = __VLS_75({
        ...{ 'onClick': {} },
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_75));
    let __VLS_79;
    const __VLS_80 = ({ click: {} },
        { onClick: (__VLS_ctx.openNew) });
    const { default: __VLS_81 } = __VLS_77.slots;
    let __VLS_82;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_83 = __VLS_asFunctionalComponent1(__VLS_82, new __VLS_82({}));
    const __VLS_84 = __VLS_83({}, ...__VLS_functionalComponentArgsRest(__VLS_83));
    const { default: __VLS_87 } = __VLS_85.slots;
    let __VLS_88;
    /** @ts-ignore @type { | typeof __VLS_components.Plus} */
    Plus;
    // @ts-ignore
    const __VLS_89 = __VLS_asFunctionalComponent1(__VLS_88, new __VLS_88({}));
    const __VLS_90 = __VLS_89({}, ...__VLS_functionalComponentArgsRest(__VLS_89));
    // @ts-ignore
    [openNew,];
    var __VLS_85;
    // @ts-ignore
    [];
    var __VLS_77;
    var __VLS_78;
}
else {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-toolbar" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-toolbar']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-breadcrumb" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-breadcrumb']} */ ;
    let __VLS_93;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_94 = __VLS_asFunctionalComponent1(__VLS_93, new __VLS_93({
        ...{ style: {} },
    }));
    const __VLS_95 = __VLS_94({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_94));
    const { default: __VLS_98 } = __VLS_96.slots;
    let __VLS_99;
    /** @ts-ignore @type { | typeof __VLS_components.FolderOpened} */
    FolderOpened;
    // @ts-ignore
    const __VLS_100 = __VLS_asFunctionalComponent1(__VLS_99, new __VLS_99({}));
    const __VLS_101 = __VLS_100({}, ...__VLS_functionalComponentArgsRest(__VLS_100));
    // @ts-ignore
    [];
    var __VLS_96;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "crumb-sep" },
    });
    /** @type {__VLS_StyleScopedClasses['crumb-sep']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "crumb-name" },
    });
    /** @type {__VLS_StyleScopedClasses['crumb-name']} */ ;
    (__VLS_ctx.selected.id);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "toolbar-acts" },
    });
    /** @type {__VLS_StyleScopedClasses['toolbar-acts']} */ ;
    let __VLS_104;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_105 = __VLS_asFunctionalComponent1(__VLS_104, new __VLS_104({
        ...{ 'onClick': {} },
        size: "small",
    }));
    const __VLS_106 = __VLS_105({
        ...{ 'onClick': {} },
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_105));
    let __VLS_109;
    const __VLS_110 = ({ click: {} },
        { onClick: (__VLS_ctx.sendTestToChat) });
    const { default: __VLS_111 } = __VLS_107.slots;
    let __VLS_112;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_113 = __VLS_asFunctionalComponent1(__VLS_112, new __VLS_112({}));
    const __VLS_114 = __VLS_113({}, ...__VLS_functionalComponentArgsRest(__VLS_113));
    const { default: __VLS_117 } = __VLS_115.slots;
    let __VLS_118;
    /** @ts-ignore @type { | typeof __VLS_components.VideoPlay} */
    VideoPlay;
    // @ts-ignore
    const __VLS_119 = __VLS_asFunctionalComponent1(__VLS_118, new __VLS_118({}));
    const __VLS_120 = __VLS_119({}, ...__VLS_functionalComponentArgsRest(__VLS_119));
    // @ts-ignore
    [selected, sendTestToChat,];
    var __VLS_115;
    // @ts-ignore
    [];
    var __VLS_107;
    var __VLS_108;
    let __VLS_123;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_124 = __VLS_asFunctionalComponent1(__VLS_123, new __VLS_123({
        ...{ 'onClick': {} },
        size: "small",
        type: "primary",
        loading: (__VLS_ctx.saving),
    }));
    const __VLS_125 = __VLS_124({
        ...{ 'onClick': {} },
        size: "small",
        type: "primary",
        loading: (__VLS_ctx.saving),
    }, ...__VLS_functionalComponentArgsRest(__VLS_124));
    let __VLS_128;
    const __VLS_129 = ({ click: {} },
        { onClick: (__VLS_ctx.saveSkill) });
    const { default: __VLS_130 } = __VLS_126.slots;
    let __VLS_131;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_132 = __VLS_asFunctionalComponent1(__VLS_131, new __VLS_131({}));
    const __VLS_133 = __VLS_132({}, ...__VLS_functionalComponentArgsRest(__VLS_132));
    const { default: __VLS_136 } = __VLS_134.slots;
    let __VLS_137;
    /** @ts-ignore @type { | typeof __VLS_components.DocumentChecked} */
    DocumentChecked;
    // @ts-ignore
    const __VLS_138 = __VLS_asFunctionalComponent1(__VLS_137, new __VLS_137({}));
    const __VLS_139 = __VLS_138({}, ...__VLS_functionalComponentArgsRest(__VLS_138));
    // @ts-ignore
    [saving, saveSkill,];
    var __VLS_134;
    // @ts-ignore
    [];
    var __VLS_126;
    var __VLS_127;
    let __VLS_142;
    /** @ts-ignore @type { | typeof __VLS_components.elPopconfirm | typeof __VLS_components.ElPopconfirm | typeof __VLS_components['el-popconfirm'] | typeof __VLS_components.elPopconfirm | typeof __VLS_components.ElPopconfirm | typeof __VLS_components['el-popconfirm']} */
    elPopconfirm;
    // @ts-ignore
    const __VLS_143 = __VLS_asFunctionalComponent1(__VLS_142, new __VLS_142({
        ...{ 'onConfirm': {} },
        title: "确认删除该技能？",
    }));
    const __VLS_144 = __VLS_143({
        ...{ 'onConfirm': {} },
        title: "确认删除该技能？",
    }, ...__VLS_functionalComponentArgsRest(__VLS_143));
    let __VLS_147;
    const __VLS_148 = ({ confirm: {} },
        { onConfirm: (__VLS_ctx.deleteSkill) });
    const { default: __VLS_149 } = __VLS_145.slots;
    {
        const { reference: __VLS_150 } = __VLS_145.slots;
        let __VLS_151;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_152 = __VLS_asFunctionalComponent1(__VLS_151, new __VLS_151({
            size: "small",
            type: "danger",
            plain: true,
        }));
        const __VLS_153 = __VLS_152({
            size: "small",
            type: "danger",
            plain: true,
        }, ...__VLS_functionalComponentArgsRest(__VLS_152));
        const { default: __VLS_156 } = __VLS_154.slots;
        let __VLS_157;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_158 = __VLS_asFunctionalComponent1(__VLS_157, new __VLS_157({}));
        const __VLS_159 = __VLS_158({}, ...__VLS_functionalComponentArgsRest(__VLS_158));
        const { default: __VLS_162 } = __VLS_160.slots;
        let __VLS_163;
        /** @ts-ignore @type { | typeof __VLS_components.Delete} */
        Delete;
        // @ts-ignore
        const __VLS_164 = __VLS_asFunctionalComponent1(__VLS_163, new __VLS_163({}));
        const __VLS_165 = __VLS_164({}, ...__VLS_functionalComponentArgsRest(__VLS_164));
        // @ts-ignore
        [deleteSkill,];
        var __VLS_160;
        // @ts-ignore
        [];
        var __VLS_154;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_145;
    var __VLS_146;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-body" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-body']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "file-tree" },
        ...{ style: ({ width: __VLS_ctx.treeW + 'px' }) },
    });
    /** @type {__VLS_StyleScopedClasses['file-tree']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "tree-title" },
    });
    /** @type {__VLS_StyleScopedClasses['tree-title']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_168;
    /** @ts-ignore @type { | typeof __VLS_components.elTooltip | typeof __VLS_components.ElTooltip | typeof __VLS_components['el-tooltip'] | typeof __VLS_components.elTooltip | typeof __VLS_components.ElTooltip | typeof __VLS_components['el-tooltip']} */
    elTooltip;
    // @ts-ignore
    const __VLS_169 = __VLS_asFunctionalComponent1(__VLS_168, new __VLS_168({
        content: "新建文件",
        placement: "top",
        showAfter: (500),
    }));
    const __VLS_170 = __VLS_169({
        content: "新建文件",
        placement: "top",
        showAfter: (500),
    }, ...__VLS_functionalComponentArgsRest(__VLS_169));
    const { default: __VLS_173 } = __VLS_171.slots;
    let __VLS_174;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_175 = __VLS_asFunctionalComponent1(__VLS_174, new __VLS_174({
        ...{ 'onClick': {} },
        link: true,
        size: "small",
    }));
    const __VLS_176 = __VLS_175({
        ...{ 'onClick': {} },
        link: true,
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_175));
    let __VLS_179;
    const __VLS_180 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!!(!__VLS_ctx.selected))
                    return;
                __VLS_ctx.openNewFileDialog('');
                // @ts-ignore
                [treeW, openNewFileDialog,];
            } });
    const { default: __VLS_181 } = __VLS_177.slots;
    let __VLS_182;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_183 = __VLS_asFunctionalComponent1(__VLS_182, new __VLS_182({}));
    const __VLS_184 = __VLS_183({}, ...__VLS_functionalComponentArgsRest(__VLS_183));
    const { default: __VLS_187 } = __VLS_185.slots;
    let __VLS_188;
    /** @ts-ignore @type { | typeof __VLS_components.DocumentAdd} */
    DocumentAdd;
    // @ts-ignore
    const __VLS_189 = __VLS_asFunctionalComponent1(__VLS_188, new __VLS_188({}));
    const __VLS_190 = __VLS_189({}, ...__VLS_functionalComponentArgsRest(__VLS_189));
    // @ts-ignore
    [];
    var __VLS_185;
    // @ts-ignore
    [];
    var __VLS_177;
    var __VLS_178;
    // @ts-ignore
    [];
    var __VLS_171;
    let __VLS_193;
    /** @ts-ignore @type { | typeof __VLS_components.elTooltip | typeof __VLS_components.ElTooltip | typeof __VLS_components['el-tooltip'] | typeof __VLS_components.elTooltip | typeof __VLS_components.ElTooltip | typeof __VLS_components['el-tooltip']} */
    elTooltip;
    // @ts-ignore
    const __VLS_194 = __VLS_asFunctionalComponent1(__VLS_193, new __VLS_193({
        content: "新建目录",
        placement: "top",
        showAfter: (500),
    }));
    const __VLS_195 = __VLS_194({
        content: "新建目录",
        placement: "top",
        showAfter: (500),
    }, ...__VLS_functionalComponentArgsRest(__VLS_194));
    const { default: __VLS_198 } = __VLS_196.slots;
    let __VLS_199;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_200 = __VLS_asFunctionalComponent1(__VLS_199, new __VLS_199({
        ...{ 'onClick': {} },
        link: true,
        size: "small",
    }));
    const __VLS_201 = __VLS_200({
        ...{ 'onClick': {} },
        link: true,
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_200));
    let __VLS_204;
    const __VLS_205 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!!(!__VLS_ctx.selected))
                    return;
                __VLS_ctx.openNewDirDialog('');
                // @ts-ignore
                [openNewDirDialog,];
            } });
    const { default: __VLS_206 } = __VLS_202.slots;
    let __VLS_207;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_208 = __VLS_asFunctionalComponent1(__VLS_207, new __VLS_207({}));
    const __VLS_209 = __VLS_208({}, ...__VLS_functionalComponentArgsRest(__VLS_208));
    const { default: __VLS_212 } = __VLS_210.slots;
    let __VLS_213;
    /** @ts-ignore @type { | typeof __VLS_components.FolderAdd} */
    FolderAdd;
    // @ts-ignore
    const __VLS_214 = __VLS_asFunctionalComponent1(__VLS_213, new __VLS_213({}));
    const __VLS_215 = __VLS_214({}, ...__VLS_functionalComponentArgsRest(__VLS_214));
    // @ts-ignore
    [];
    var __VLS_210;
    // @ts-ignore
    [];
    var __VLS_202;
    var __VLS_203;
    // @ts-ignore
    [];
    var __VLS_196;
    let __VLS_218;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_219 = __VLS_asFunctionalComponent1(__VLS_218, new __VLS_218({
        ...{ 'onClick': {} },
        link: true,
        size: "small",
        loading: (__VLS_ctx.dirLoading),
    }));
    const __VLS_220 = __VLS_219({
        ...{ 'onClick': {} },
        link: true,
        size: "small",
        loading: (__VLS_ctx.dirLoading),
    }, ...__VLS_functionalComponentArgsRest(__VLS_219));
    let __VLS_223;
    const __VLS_224 = ({ click: {} },
        { onClick: (__VLS_ctx.loadDirFiles) });
    const { default: __VLS_225 } = __VLS_221.slots;
    let __VLS_226;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_227 = __VLS_asFunctionalComponent1(__VLS_226, new __VLS_226({}));
    const __VLS_228 = __VLS_227({}, ...__VLS_functionalComponentArgsRest(__VLS_227));
    const { default: __VLS_231 } = __VLS_229.slots;
    let __VLS_232;
    /** @ts-ignore @type { | typeof __VLS_components.Refresh} */
    Refresh;
    // @ts-ignore
    const __VLS_233 = __VLS_asFunctionalComponent1(__VLS_232, new __VLS_232({}));
    const __VLS_234 = __VLS_233({}, ...__VLS_functionalComponentArgsRest(__VLS_233));
    // @ts-ignore
    [dirLoading, loadDirFiles,];
    var __VLS_229;
    // @ts-ignore
    [];
    var __VLS_221;
    var __VLS_222;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "tree-item tree-dir-root" },
    });
    /** @type {__VLS_StyleScopedClasses['tree-item']} */ ;
    /** @type {__VLS_StyleScopedClasses['tree-dir-root']} */ ;
    let __VLS_237;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_238 = __VLS_asFunctionalComponent1(__VLS_237, new __VLS_237({
        ...{ style: {} },
    }));
    const __VLS_239 = __VLS_238({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_238));
    const { default: __VLS_242 } = __VLS_240.slots;
    let __VLS_243;
    /** @ts-ignore @type { | typeof __VLS_components.Folder} */
    Folder;
    // @ts-ignore
    const __VLS_244 = __VLS_asFunctionalComponent1(__VLS_243, new __VLS_243({}));
    const __VLS_245 = __VLS_244({}, ...__VLS_functionalComponentArgsRest(__VLS_244));
    // @ts-ignore
    [];
    var __VLS_240;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "tree-name" },
    });
    /** @type {__VLS_StyleScopedClasses['tree-name']} */ ;
    (__VLS_ctx.selected.id);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                if (!!(!__VLS_ctx.selected))
                    return;
                __VLS_ctx.activeFile = 'meta';
                // @ts-ignore
                [selected, activeFile,];
            } },
        ...{ class: (['tree-item', { 'tree-active': __VLS_ctx.activeFile === 'meta' }]) },
    });
    /** @type {__VLS_StyleScopedClasses['tree-item']} */ ;
    /** @type {__VLS_StyleScopedClasses['tree-active']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span)({
        ...{ class: "tree-indent" },
    });
    /** @type {__VLS_StyleScopedClasses['tree-indent']} */ ;
    let __VLS_248;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_249 = __VLS_asFunctionalComponent1(__VLS_248, new __VLS_248({
        ...{ style: {} },
    }));
    const __VLS_250 = __VLS_249({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_249));
    const { default: __VLS_253 } = __VLS_251.slots;
    let __VLS_254;
    /** @ts-ignore @type { | typeof __VLS_components.Setting} */
    Setting;
    // @ts-ignore
    const __VLS_255 = __VLS_asFunctionalComponent1(__VLS_254, new __VLS_254({}));
    const __VLS_256 = __VLS_255({}, ...__VLS_functionalComponentArgsRest(__VLS_255));
    // @ts-ignore
    [activeFile,];
    var __VLS_251;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "tree-name" },
    });
    /** @type {__VLS_StyleScopedClasses['tree-name']} */ ;
    for (const [f] of __VLS_vFor((__VLS_ctx.visibleFiles))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ onClick: (...[$event]) => {
                    if (!!(!__VLS_ctx.selected))
                        return;
                    f.isDir ? __VLS_ctx.toggleDir(f.path) : __VLS_ctx.openFile(f.path, false);
                    // @ts-ignore
                    [visibleFiles, toggleDir, openFile,];
                } },
            ...{ class: (['tree-item', {
                        'tree-active': __VLS_ctx.activeFile === (f.path === 'SKILL.md' ? 'prompt' : f.path) && !f.isDir,
                        'tree-dir-row': f.isDir,
                    }]) },
            ...{ style: ({ paddingLeft: `${8 + (f.depth + 1) * 12}px` }) },
            key: (f.path),
        });
        /** @type {__VLS_StyleScopedClasses['tree-item']} */ ;
        /** @type {__VLS_StyleScopedClasses['tree-active']} */ ;
        /** @type {__VLS_StyleScopedClasses['tree-dir-row']} */ ;
        if (f.isDir) {
            let __VLS_259;
            /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
            elIcon;
            // @ts-ignore
            const __VLS_260 = __VLS_asFunctionalComponent1(__VLS_259, new __VLS_259({
                ...{ class: "dir-arrow" },
                ...{ class: ({ 'dir-open': !__VLS_ctx.collapsedDirs.has(f.path) }) },
            }));
            const __VLS_261 = __VLS_260({
                ...{ class: "dir-arrow" },
                ...{ class: ({ 'dir-open': !__VLS_ctx.collapsedDirs.has(f.path) }) },
            }, ...__VLS_functionalComponentArgsRest(__VLS_260));
            /** @type {__VLS_StyleScopedClasses['dir-arrow']} */ ;
            /** @type {__VLS_StyleScopedClasses['dir-open']} */ ;
            const { default: __VLS_264 } = __VLS_262.slots;
            let __VLS_265;
            /** @ts-ignore @type { | typeof __VLS_components.ArrowRight} */
            ArrowRight;
            // @ts-ignore
            const __VLS_266 = __VLS_asFunctionalComponent1(__VLS_265, new __VLS_265({}));
            const __VLS_267 = __VLS_266({}, ...__VLS_functionalComponentArgsRest(__VLS_266));
            // @ts-ignore
            [activeFile, collapsedDirs,];
            var __VLS_262;
        }
        if (f.isDir) {
            let __VLS_270;
            /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
            elIcon;
            // @ts-ignore
            const __VLS_271 = __VLS_asFunctionalComponent1(__VLS_270, new __VLS_270({
                ...{ style: {} },
            }));
            const __VLS_272 = __VLS_271({
                ...{ style: {} },
            }, ...__VLS_functionalComponentArgsRest(__VLS_271));
            const { default: __VLS_275 } = __VLS_273.slots;
            let __VLS_276;
            /** @ts-ignore @type { | typeof __VLS_components.Folder} */
            Folder;
            // @ts-ignore
            const __VLS_277 = __VLS_asFunctionalComponent1(__VLS_276, new __VLS_276({}));
            const __VLS_278 = __VLS_277({}, ...__VLS_functionalComponentArgsRest(__VLS_277));
            // @ts-ignore
            [];
            var __VLS_273;
        }
        else {
            let __VLS_281;
            /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
            elIcon;
            // @ts-ignore
            const __VLS_282 = __VLS_asFunctionalComponent1(__VLS_281, new __VLS_281({
                ...{ style: {} },
            }));
            const __VLS_283 = __VLS_282({
                ...{ style: {} },
            }, ...__VLS_functionalComponentArgsRest(__VLS_282));
            const { default: __VLS_286 } = __VLS_284.slots;
            let __VLS_287;
            /** @ts-ignore @type { | typeof __VLS_components.Document} */
            Document;
            // @ts-ignore
            const __VLS_288 = __VLS_asFunctionalComponent1(__VLS_287, new __VLS_287({}));
            const __VLS_289 = __VLS_288({}, ...__VLS_functionalComponentArgsRest(__VLS_288));
            // @ts-ignore
            [];
            var __VLS_284;
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "tree-name" },
        });
        /** @type {__VLS_StyleScopedClasses['tree-name']} */ ;
        (f.name);
        if (f.path === 'SKILL.md' && __VLS_ctx.selected.enabled) {
            let __VLS_292;
            /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
            elTag;
            // @ts-ignore
            const __VLS_293 = __VLS_asFunctionalComponent1(__VLS_292, new __VLS_292({
                size: "small",
                type: "success",
                effect: "plain",
                ...{ style: {} },
            }));
            const __VLS_294 = __VLS_293({
                size: "small",
                type: "success",
                effect: "plain",
                ...{ style: {} },
            }, ...__VLS_functionalComponentArgsRest(__VLS_293));
            const { default: __VLS_297 } = __VLS_295.slots;
            // @ts-ignore
            [selected,];
            var __VLS_295;
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ onClick: () => { } },
            ...{ class: "tree-item-acts" },
        });
        /** @type {__VLS_StyleScopedClasses['tree-item-acts']} */ ;
        if (f.isDir) {
            let __VLS_298;
            /** @ts-ignore @type { | typeof __VLS_components.elTooltip | typeof __VLS_components.ElTooltip | typeof __VLS_components['el-tooltip'] | typeof __VLS_components.elTooltip | typeof __VLS_components.ElTooltip | typeof __VLS_components['el-tooltip']} */
            elTooltip;
            // @ts-ignore
            const __VLS_299 = __VLS_asFunctionalComponent1(__VLS_298, new __VLS_298({
                content: "在此目录新建文件",
                placement: "top",
                showAfter: (300),
            }));
            const __VLS_300 = __VLS_299({
                content: "在此目录新建文件",
                placement: "top",
                showAfter: (300),
            }, ...__VLS_functionalComponentArgsRest(__VLS_299));
            const { default: __VLS_303 } = __VLS_301.slots;
            let __VLS_304;
            /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
            elButton;
            // @ts-ignore
            const __VLS_305 = __VLS_asFunctionalComponent1(__VLS_304, new __VLS_304({
                ...{ 'onClick': {} },
                link: true,
                size: "small",
            }));
            const __VLS_306 = __VLS_305({
                ...{ 'onClick': {} },
                link: true,
                size: "small",
            }, ...__VLS_functionalComponentArgsRest(__VLS_305));
            let __VLS_309;
            const __VLS_310 = ({ click: {} },
                { onClick: (...[$event]) => {
                        if (!!(!__VLS_ctx.selected))
                            return;
                        if (!(f.isDir))
                            return;
                        __VLS_ctx.openNewFileDialog(f.path);
                        // @ts-ignore
                        [openNewFileDialog,];
                    } });
            const { default: __VLS_311 } = __VLS_307.slots;
            let __VLS_312;
            /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
            elIcon;
            // @ts-ignore
            const __VLS_313 = __VLS_asFunctionalComponent1(__VLS_312, new __VLS_312({}));
            const __VLS_314 = __VLS_313({}, ...__VLS_functionalComponentArgsRest(__VLS_313));
            const { default: __VLS_317 } = __VLS_315.slots;
            let __VLS_318;
            /** @ts-ignore @type { | typeof __VLS_components.DocumentAdd} */
            DocumentAdd;
            // @ts-ignore
            const __VLS_319 = __VLS_asFunctionalComponent1(__VLS_318, new __VLS_318({}));
            const __VLS_320 = __VLS_319({}, ...__VLS_functionalComponentArgsRest(__VLS_319));
            // @ts-ignore
            [];
            var __VLS_315;
            // @ts-ignore
            [];
            var __VLS_307;
            var __VLS_308;
            // @ts-ignore
            [];
            var __VLS_301;
        }
        if (f.isDir) {
            let __VLS_323;
            /** @ts-ignore @type { | typeof __VLS_components.elTooltip | typeof __VLS_components.ElTooltip | typeof __VLS_components['el-tooltip'] | typeof __VLS_components.elTooltip | typeof __VLS_components.ElTooltip | typeof __VLS_components['el-tooltip']} */
            elTooltip;
            // @ts-ignore
            const __VLS_324 = __VLS_asFunctionalComponent1(__VLS_323, new __VLS_323({
                content: "在此目录新建子目录",
                placement: "top",
                showAfter: (300),
            }));
            const __VLS_325 = __VLS_324({
                content: "在此目录新建子目录",
                placement: "top",
                showAfter: (300),
            }, ...__VLS_functionalComponentArgsRest(__VLS_324));
            const { default: __VLS_328 } = __VLS_326.slots;
            let __VLS_329;
            /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
            elButton;
            // @ts-ignore
            const __VLS_330 = __VLS_asFunctionalComponent1(__VLS_329, new __VLS_329({
                ...{ 'onClick': {} },
                link: true,
                size: "small",
            }));
            const __VLS_331 = __VLS_330({
                ...{ 'onClick': {} },
                link: true,
                size: "small",
            }, ...__VLS_functionalComponentArgsRest(__VLS_330));
            let __VLS_334;
            const __VLS_335 = ({ click: {} },
                { onClick: (...[$event]) => {
                        if (!!(!__VLS_ctx.selected))
                            return;
                        if (!(f.isDir))
                            return;
                        __VLS_ctx.openNewDirDialog(f.path);
                        // @ts-ignore
                        [openNewDirDialog,];
                    } });
            const { default: __VLS_336 } = __VLS_332.slots;
            let __VLS_337;
            /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
            elIcon;
            // @ts-ignore
            const __VLS_338 = __VLS_asFunctionalComponent1(__VLS_337, new __VLS_337({}));
            const __VLS_339 = __VLS_338({}, ...__VLS_functionalComponentArgsRest(__VLS_338));
            const { default: __VLS_342 } = __VLS_340.slots;
            let __VLS_343;
            /** @ts-ignore @type { | typeof __VLS_components.FolderAdd} */
            FolderAdd;
            // @ts-ignore
            const __VLS_344 = __VLS_asFunctionalComponent1(__VLS_343, new __VLS_343({}));
            const __VLS_345 = __VLS_344({}, ...__VLS_functionalComponentArgsRest(__VLS_344));
            // @ts-ignore
            [];
            var __VLS_340;
            // @ts-ignore
            [];
            var __VLS_332;
            var __VLS_333;
            // @ts-ignore
            [];
            var __VLS_326;
        }
        if (!f.isDir) {
            let __VLS_348;
            /** @ts-ignore @type { | typeof __VLS_components.elTooltip | typeof __VLS_components.ElTooltip | typeof __VLS_components['el-tooltip'] | typeof __VLS_components.elTooltip | typeof __VLS_components.ElTooltip | typeof __VLS_components['el-tooltip']} */
            elTooltip;
            // @ts-ignore
            const __VLS_349 = __VLS_asFunctionalComponent1(__VLS_348, new __VLS_348({
                content: "重命名",
                placement: "top",
                showAfter: (300),
            }));
            const __VLS_350 = __VLS_349({
                content: "重命名",
                placement: "top",
                showAfter: (300),
            }, ...__VLS_functionalComponentArgsRest(__VLS_349));
            const { default: __VLS_353 } = __VLS_351.slots;
            let __VLS_354;
            /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
            elButton;
            // @ts-ignore
            const __VLS_355 = __VLS_asFunctionalComponent1(__VLS_354, new __VLS_354({
                ...{ 'onClick': {} },
                link: true,
                size: "small",
            }));
            const __VLS_356 = __VLS_355({
                ...{ 'onClick': {} },
                link: true,
                size: "small",
            }, ...__VLS_functionalComponentArgsRest(__VLS_355));
            let __VLS_359;
            const __VLS_360 = ({ click: {} },
                { onClick: (...[$event]) => {
                        if (!!(!__VLS_ctx.selected))
                            return;
                        if (!(!f.isDir))
                            return;
                        __VLS_ctx.openRenameDialog(f.path);
                        // @ts-ignore
                        [openRenameDialog,];
                    } });
            const { default: __VLS_361 } = __VLS_357.slots;
            let __VLS_362;
            /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
            elIcon;
            // @ts-ignore
            const __VLS_363 = __VLS_asFunctionalComponent1(__VLS_362, new __VLS_362({}));
            const __VLS_364 = __VLS_363({}, ...__VLS_functionalComponentArgsRest(__VLS_363));
            const { default: __VLS_367 } = __VLS_365.slots;
            let __VLS_368;
            /** @ts-ignore @type { | typeof __VLS_components.Edit} */
            Edit;
            // @ts-ignore
            const __VLS_369 = __VLS_asFunctionalComponent1(__VLS_368, new __VLS_368({}));
            const __VLS_370 = __VLS_369({}, ...__VLS_functionalComponentArgsRest(__VLS_369));
            // @ts-ignore
            [];
            var __VLS_365;
            // @ts-ignore
            [];
            var __VLS_357;
            var __VLS_358;
            // @ts-ignore
            [];
            var __VLS_351;
        }
        let __VLS_373;
        /** @ts-ignore @type { | typeof __VLS_components.elPopconfirm | typeof __VLS_components.ElPopconfirm | typeof __VLS_components['el-popconfirm'] | typeof __VLS_components.elPopconfirm | typeof __VLS_components.ElPopconfirm | typeof __VLS_components['el-popconfirm']} */
        elPopconfirm;
        // @ts-ignore
        const __VLS_374 = __VLS_asFunctionalComponent1(__VLS_373, new __VLS_373({
            ...{ 'onConfirm': {} },
            title: (`删除 ${f.name}？`),
            width: "180",
        }));
        const __VLS_375 = __VLS_374({
            ...{ 'onConfirm': {} },
            title: (`删除 ${f.name}？`),
            width: "180",
        }, ...__VLS_functionalComponentArgsRest(__VLS_374));
        let __VLS_378;
        const __VLS_379 = ({ confirm: {} },
            { onConfirm: (...[$event]) => {
                    if (!!(!__VLS_ctx.selected))
                        return;
                    __VLS_ctx.deleteFile(f.path);
                    // @ts-ignore
                    [deleteFile,];
                } });
        const { default: __VLS_380 } = __VLS_376.slots;
        {
            const { reference: __VLS_381 } = __VLS_376.slots;
            let __VLS_382;
            /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
            elButton;
            // @ts-ignore
            const __VLS_383 = __VLS_asFunctionalComponent1(__VLS_382, new __VLS_382({
                link: true,
                size: "small",
                type: "danger",
            }));
            const __VLS_384 = __VLS_383({
                link: true,
                size: "small",
                type: "danger",
            }, ...__VLS_functionalComponentArgsRest(__VLS_383));
            const { default: __VLS_387 } = __VLS_385.slots;
            let __VLS_388;
            /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
            elIcon;
            // @ts-ignore
            const __VLS_389 = __VLS_asFunctionalComponent1(__VLS_388, new __VLS_388({}));
            const __VLS_390 = __VLS_389({}, ...__VLS_functionalComponentArgsRest(__VLS_389));
            const { default: __VLS_393 } = __VLS_391.slots;
            let __VLS_394;
            /** @ts-ignore @type { | typeof __VLS_components.Delete} */
            Delete;
            // @ts-ignore
            const __VLS_395 = __VLS_asFunctionalComponent1(__VLS_394, new __VLS_394({}));
            const __VLS_396 = __VLS_395({}, ...__VLS_functionalComponentArgsRest(__VLS_395));
            // @ts-ignore
            [];
            var __VLS_391;
            // @ts-ignore
            [];
            var __VLS_385;
            // @ts-ignore
            [];
        }
        // @ts-ignore
        [];
        var __VLS_376;
        var __VLS_377;
        // @ts-ignore
        [];
    }
    if (!__VLS_ctx.dirLoading && __VLS_ctx.dirFiles.length === 0) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "tree-empty" },
        });
        /** @type {__VLS_StyleScopedClasses['tree-empty']} */ ;
    }
    let __VLS_399;
    /** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
    elDialog;
    // @ts-ignore
    const __VLS_400 = __VLS_asFunctionalComponent1(__VLS_399, new __VLS_399({
        modelValue: (__VLS_ctx.newEntryDialog.visible),
        title: (__VLS_ctx.newEntryDialog.isDir ? '新建目录' : '新建文件'),
        width: "360px",
        closeOnClickModal: (false),
    }));
    const __VLS_401 = __VLS_400({
        modelValue: (__VLS_ctx.newEntryDialog.visible),
        title: (__VLS_ctx.newEntryDialog.isDir ? '新建目录' : '新建文件'),
        width: "360px",
        closeOnClickModal: (false),
    }, ...__VLS_functionalComponentArgsRest(__VLS_400));
    const { default: __VLS_404 } = __VLS_402.slots;
    let __VLS_405;
    /** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
    elForm;
    // @ts-ignore
    const __VLS_406 = __VLS_asFunctionalComponent1(__VLS_405, new __VLS_405({
        ...{ 'onSubmit': {} },
    }));
    const __VLS_407 = __VLS_406({
        ...{ 'onSubmit': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_406));
    let __VLS_410;
    const __VLS_411 = ({ submit: {} },
        { onSubmit: (__VLS_ctx.createEntry) });
    const { default: __VLS_412 } = __VLS_408.slots;
    if (__VLS_ctx.newEntryDialog.inDir) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        (__VLS_ctx.selected.id);
        (__VLS_ctx.newEntryDialog.inDir);
    }
    let __VLS_413;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_414 = __VLS_asFunctionalComponent1(__VLS_413, new __VLS_413({
        ...{ 'onKeyup': {} },
        modelValue: (__VLS_ctx.newEntryDialog.name),
        placeholder: (__VLS_ctx.newEntryDialog.isDir ? '目录名（如 tools）' : '文件名（如 config.json）'),
        autofocus: true,
    }));
    const __VLS_415 = __VLS_414({
        ...{ 'onKeyup': {} },
        modelValue: (__VLS_ctx.newEntryDialog.name),
        placeholder: (__VLS_ctx.newEntryDialog.isDir ? '目录名（如 tools）' : '文件名（如 config.json）'),
        autofocus: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_414));
    let __VLS_418;
    const __VLS_419 = ({ keyup: {} },
        { onKeyup: (__VLS_ctx.createEntry) });
    var __VLS_416;
    var __VLS_417;
    // @ts-ignore
    [selected, dirLoading, dirFiles, newEntryDialog, newEntryDialog, newEntryDialog, newEntryDialog, newEntryDialog, newEntryDialog, createEntry, createEntry,];
    var __VLS_408;
    var __VLS_409;
    {
        const { footer: __VLS_420 } = __VLS_402.slots;
        let __VLS_421;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_422 = __VLS_asFunctionalComponent1(__VLS_421, new __VLS_421({
            ...{ 'onClick': {} },
        }));
        const __VLS_423 = __VLS_422({
            ...{ 'onClick': {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_422));
        let __VLS_426;
        const __VLS_427 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(!__VLS_ctx.selected))
                        return;
                    __VLS_ctx.newEntryDialog.visible = false;
                    // @ts-ignore
                    [newEntryDialog,];
                } });
        const { default: __VLS_428 } = __VLS_424.slots;
        // @ts-ignore
        [];
        var __VLS_424;
        var __VLS_425;
        let __VLS_429;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_430 = __VLS_asFunctionalComponent1(__VLS_429, new __VLS_429({
            ...{ 'onClick': {} },
            type: "primary",
            loading: (__VLS_ctx.newEntryDialog.creating),
        }));
        const __VLS_431 = __VLS_430({
            ...{ 'onClick': {} },
            type: "primary",
            loading: (__VLS_ctx.newEntryDialog.creating),
        }, ...__VLS_functionalComponentArgsRest(__VLS_430));
        let __VLS_434;
        const __VLS_435 = ({ click: {} },
            { onClick: (__VLS_ctx.createEntry) });
        const { default: __VLS_436 } = __VLS_432.slots;
        // @ts-ignore
        [newEntryDialog, createEntry,];
        var __VLS_432;
        var __VLS_433;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_402;
    let __VLS_437;
    /** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
    elDialog;
    // @ts-ignore
    const __VLS_438 = __VLS_asFunctionalComponent1(__VLS_437, new __VLS_437({
        modelValue: (__VLS_ctx.renameDialog.visible),
        title: "重命名文件",
        width: "360px",
        closeOnClickModal: (false),
    }));
    const __VLS_439 = __VLS_438({
        modelValue: (__VLS_ctx.renameDialog.visible),
        title: "重命名文件",
        width: "360px",
        closeOnClickModal: (false),
    }, ...__VLS_functionalComponentArgsRest(__VLS_438));
    const { default: __VLS_442 } = __VLS_440.slots;
    let __VLS_443;
    /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
    elInput;
    // @ts-ignore
    const __VLS_444 = __VLS_asFunctionalComponent1(__VLS_443, new __VLS_443({
        ...{ 'onKeyup': {} },
        modelValue: (__VLS_ctx.renameDialog.newName),
        placeholder: (__VLS_ctx.renameDialog.oldPath),
    }));
    const __VLS_445 = __VLS_444({
        ...{ 'onKeyup': {} },
        modelValue: (__VLS_ctx.renameDialog.newName),
        placeholder: (__VLS_ctx.renameDialog.oldPath),
    }, ...__VLS_functionalComponentArgsRest(__VLS_444));
    let __VLS_448;
    const __VLS_449 = ({ keyup: {} },
        { onKeyup: (__VLS_ctx.doRename) });
    var __VLS_446;
    var __VLS_447;
    {
        const { footer: __VLS_450 } = __VLS_440.slots;
        let __VLS_451;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_452 = __VLS_asFunctionalComponent1(__VLS_451, new __VLS_451({
            ...{ 'onClick': {} },
        }));
        const __VLS_453 = __VLS_452({
            ...{ 'onClick': {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_452));
        let __VLS_456;
        const __VLS_457 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(!__VLS_ctx.selected))
                        return;
                    __VLS_ctx.renameDialog.visible = false;
                    // @ts-ignore
                    [renameDialog, renameDialog, renameDialog, renameDialog, doRename,];
                } });
        const { default: __VLS_458 } = __VLS_454.slots;
        // @ts-ignore
        [];
        var __VLS_454;
        var __VLS_455;
        let __VLS_459;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_460 = __VLS_asFunctionalComponent1(__VLS_459, new __VLS_459({
            ...{ 'onClick': {} },
            type: "primary",
            loading: (__VLS_ctx.renameDialog.saving),
        }));
        const __VLS_461 = __VLS_460({
            ...{ 'onClick': {} },
            type: "primary",
            loading: (__VLS_ctx.renameDialog.saving),
        }, ...__VLS_functionalComponentArgsRest(__VLS_460));
        let __VLS_464;
        const __VLS_465 = ({ click: {} },
            { onClick: (__VLS_ctx.doRename) });
        const { default: __VLS_466 } = __VLS_462.slots;
        // @ts-ignore
        [renameDialog, doRename,];
        var __VLS_462;
        var __VLS_463;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_440;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onMousedown: (...[$event]) => {
                if (!!(!__VLS_ctx.selected))
                    return;
                __VLS_ctx.startResize($event, 'tree');
                // @ts-ignore
                [startResize,];
            } },
        ...{ class: "ss-handle" },
        ...{ class: ({ dragging: __VLS_ctx.dragging === 'tree' }) },
    });
    /** @type {__VLS_StyleScopedClasses['ss-handle']} */ ;
    /** @type {__VLS_StyleScopedClasses['dragging']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div)({
        ...{ class: "ss-handle-bar" },
    });
    /** @type {__VLS_StyleScopedClasses['ss-handle-bar']} */ ;
    if (__VLS_ctx.activeFile === 'meta') {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "file-editor" },
        });
        /** @type {__VLS_StyleScopedClasses['file-editor']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "file-editor-head" },
        });
        /** @type {__VLS_StyleScopedClasses['file-editor-head']} */ ;
        let __VLS_467;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_468 = __VLS_asFunctionalComponent1(__VLS_467, new __VLS_467({}));
        const __VLS_469 = __VLS_468({}, ...__VLS_functionalComponentArgsRest(__VLS_468));
        const { default: __VLS_472 } = __VLS_470.slots;
        let __VLS_473;
        /** @ts-ignore @type { | typeof __VLS_components.Document} */
        Document;
        // @ts-ignore
        const __VLS_474 = __VLS_asFunctionalComponent1(__VLS_473, new __VLS_473({}));
        const __VLS_475 = __VLS_474({}, ...__VLS_functionalComponentArgsRest(__VLS_474));
        // @ts-ignore
        [dragging, activeFile,];
        var __VLS_470;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "file-hint" },
        });
        /** @type {__VLS_StyleScopedClasses['file-hint']} */ ;
        let __VLS_478;
        /** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
        elForm;
        // @ts-ignore
        const __VLS_479 = __VLS_asFunctionalComponent1(__VLS_478, new __VLS_478({
            model: (__VLS_ctx.metaForm),
            labelWidth: "72px",
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_480 = __VLS_479({
            model: (__VLS_ctx.metaForm),
            labelWidth: "72px",
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_479));
        const { default: __VLS_483 } = __VLS_481.slots;
        let __VLS_484;
        /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
        elFormItem;
        // @ts-ignore
        const __VLS_485 = __VLS_asFunctionalComponent1(__VLS_484, new __VLS_484({
            label: "技能 ID",
        }));
        const __VLS_486 = __VLS_485({
            label: "技能 ID",
        }, ...__VLS_functionalComponentArgsRest(__VLS_485));
        const { default: __VLS_489 } = __VLS_487.slots;
        let __VLS_490;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_491 = __VLS_asFunctionalComponent1(__VLS_490, new __VLS_490({
            value: (__VLS_ctx.selected.id),
            disabled: true,
        }));
        const __VLS_492 = __VLS_491({
            value: (__VLS_ctx.selected.id),
            disabled: true,
        }, ...__VLS_functionalComponentArgsRest(__VLS_491));
        // @ts-ignore
        [selected, metaForm,];
        var __VLS_487;
        let __VLS_495;
        /** @ts-ignore @type { | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row'] | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row']} */
        elRow;
        // @ts-ignore
        const __VLS_496 = __VLS_asFunctionalComponent1(__VLS_495, new __VLS_495({
            gutter: (12),
        }));
        const __VLS_497 = __VLS_496({
            gutter: (12),
        }, ...__VLS_functionalComponentArgsRest(__VLS_496));
        const { default: __VLS_500 } = __VLS_498.slots;
        let __VLS_501;
        /** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
        elCol;
        // @ts-ignore
        const __VLS_502 = __VLS_asFunctionalComponent1(__VLS_501, new __VLS_501({
            span: (14),
        }));
        const __VLS_503 = __VLS_502({
            span: (14),
        }, ...__VLS_functionalComponentArgsRest(__VLS_502));
        const { default: __VLS_506 } = __VLS_504.slots;
        let __VLS_507;
        /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
        elFormItem;
        // @ts-ignore
        const __VLS_508 = __VLS_asFunctionalComponent1(__VLS_507, new __VLS_507({
            label: "名称",
        }));
        const __VLS_509 = __VLS_508({
            label: "名称",
        }, ...__VLS_functionalComponentArgsRest(__VLS_508));
        const { default: __VLS_512 } = __VLS_510.slots;
        let __VLS_513;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_514 = __VLS_asFunctionalComponent1(__VLS_513, new __VLS_513({
            modelValue: (__VLS_ctx.metaForm.name),
            placeholder: "如 翻译助手",
        }));
        const __VLS_515 = __VLS_514({
            modelValue: (__VLS_ctx.metaForm.name),
            placeholder: "如 翻译助手",
        }, ...__VLS_functionalComponentArgsRest(__VLS_514));
        // @ts-ignore
        [metaForm,];
        var __VLS_510;
        // @ts-ignore
        [];
        var __VLS_504;
        let __VLS_518;
        /** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
        elCol;
        // @ts-ignore
        const __VLS_519 = __VLS_asFunctionalComponent1(__VLS_518, new __VLS_518({
            span: (10),
        }));
        const __VLS_520 = __VLS_519({
            span: (10),
        }, ...__VLS_functionalComponentArgsRest(__VLS_519));
        const { default: __VLS_523 } = __VLS_521.slots;
        let __VLS_524;
        /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
        elFormItem;
        // @ts-ignore
        const __VLS_525 = __VLS_asFunctionalComponent1(__VLS_524, new __VLS_524({
            label: "图标",
        }));
        const __VLS_526 = __VLS_525({
            label: "图标",
        }, ...__VLS_functionalComponentArgsRest(__VLS_525));
        const { default: __VLS_529 } = __VLS_527.slots;
        let __VLS_530;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_531 = __VLS_asFunctionalComponent1(__VLS_530, new __VLS_530({
            modelValue: (__VLS_ctx.metaForm.icon),
            placeholder: "emoji",
        }));
        const __VLS_532 = __VLS_531({
            modelValue: (__VLS_ctx.metaForm.icon),
            placeholder: "emoji",
        }, ...__VLS_functionalComponentArgsRest(__VLS_531));
        // @ts-ignore
        [metaForm,];
        var __VLS_527;
        // @ts-ignore
        [];
        var __VLS_521;
        // @ts-ignore
        [];
        var __VLS_498;
        let __VLS_535;
        /** @ts-ignore @type { | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row'] | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row']} */
        elRow;
        // @ts-ignore
        const __VLS_536 = __VLS_asFunctionalComponent1(__VLS_535, new __VLS_535({
            gutter: (12),
        }));
        const __VLS_537 = __VLS_536({
            gutter: (12),
        }, ...__VLS_functionalComponentArgsRest(__VLS_536));
        const { default: __VLS_540 } = __VLS_538.slots;
        let __VLS_541;
        /** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
        elCol;
        // @ts-ignore
        const __VLS_542 = __VLS_asFunctionalComponent1(__VLS_541, new __VLS_541({
            span: (14),
        }));
        const __VLS_543 = __VLS_542({
            span: (14),
        }, ...__VLS_functionalComponentArgsRest(__VLS_542));
        const { default: __VLS_546 } = __VLS_544.slots;
        let __VLS_547;
        /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
        elFormItem;
        // @ts-ignore
        const __VLS_548 = __VLS_asFunctionalComponent1(__VLS_547, new __VLS_547({
            label: "分类",
        }));
        const __VLS_549 = __VLS_548({
            label: "分类",
        }, ...__VLS_functionalComponentArgsRest(__VLS_548));
        const { default: __VLS_552 } = __VLS_550.slots;
        let __VLS_553;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_554 = __VLS_asFunctionalComponent1(__VLS_553, new __VLS_553({
            modelValue: (__VLS_ctx.metaForm.category),
            placeholder: "如 语言",
        }));
        const __VLS_555 = __VLS_554({
            modelValue: (__VLS_ctx.metaForm.category),
            placeholder: "如 语言",
        }, ...__VLS_functionalComponentArgsRest(__VLS_554));
        // @ts-ignore
        [metaForm,];
        var __VLS_550;
        // @ts-ignore
        [];
        var __VLS_544;
        let __VLS_558;
        /** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
        elCol;
        // @ts-ignore
        const __VLS_559 = __VLS_asFunctionalComponent1(__VLS_558, new __VLS_558({
            span: (10),
        }));
        const __VLS_560 = __VLS_559({
            span: (10),
        }, ...__VLS_functionalComponentArgsRest(__VLS_559));
        const { default: __VLS_563 } = __VLS_561.slots;
        let __VLS_564;
        /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
        elFormItem;
        // @ts-ignore
        const __VLS_565 = __VLS_asFunctionalComponent1(__VLS_564, new __VLS_564({
            label: "版本",
        }));
        const __VLS_566 = __VLS_565({
            label: "版本",
        }, ...__VLS_functionalComponentArgsRest(__VLS_565));
        const { default: __VLS_569 } = __VLS_567.slots;
        let __VLS_570;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_571 = __VLS_asFunctionalComponent1(__VLS_570, new __VLS_570({
            modelValue: (__VLS_ctx.metaForm.version),
            placeholder: "1.0.0",
        }));
        const __VLS_572 = __VLS_571({
            modelValue: (__VLS_ctx.metaForm.version),
            placeholder: "1.0.0",
        }, ...__VLS_functionalComponentArgsRest(__VLS_571));
        // @ts-ignore
        [metaForm,];
        var __VLS_567;
        // @ts-ignore
        [];
        var __VLS_561;
        // @ts-ignore
        [];
        var __VLS_538;
        let __VLS_575;
        /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
        elFormItem;
        // @ts-ignore
        const __VLS_576 = __VLS_asFunctionalComponent1(__VLS_575, new __VLS_575({
            label: "描述",
        }));
        const __VLS_577 = __VLS_576({
            label: "描述",
        }, ...__VLS_functionalComponentArgsRest(__VLS_576));
        const { default: __VLS_580 } = __VLS_578.slots;
        let __VLS_581;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_582 = __VLS_asFunctionalComponent1(__VLS_581, new __VLS_581({
            modelValue: (__VLS_ctx.metaForm.description),
            type: "textarea",
            rows: (2),
            placeholder: "简要描述技能功能",
        }));
        const __VLS_583 = __VLS_582({
            modelValue: (__VLS_ctx.metaForm.description),
            type: "textarea",
            rows: (2),
            placeholder: "简要描述技能功能",
        }, ...__VLS_functionalComponentArgsRest(__VLS_582));
        // @ts-ignore
        [metaForm,];
        var __VLS_578;
        let __VLS_586;
        /** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
        elFormItem;
        // @ts-ignore
        const __VLS_587 = __VLS_asFunctionalComponent1(__VLS_586, new __VLS_586({
            label: "状态",
        }));
        const __VLS_588 = __VLS_587({
            label: "状态",
        }, ...__VLS_functionalComponentArgsRest(__VLS_587));
        const { default: __VLS_591 } = __VLS_589.slots;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        let __VLS_592;
        /** @ts-ignore @type { | typeof __VLS_components.elSwitch | typeof __VLS_components.ElSwitch | typeof __VLS_components['el-switch']} */
        elSwitch;
        // @ts-ignore
        const __VLS_593 = __VLS_asFunctionalComponent1(__VLS_592, new __VLS_592({
            modelValue: (__VLS_ctx.metaForm.enabled),
        }));
        const __VLS_594 = __VLS_593({
            modelValue: (__VLS_ctx.metaForm.enabled),
        }, ...__VLS_functionalComponentArgsRest(__VLS_593));
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        (__VLS_ctx.metaForm.enabled ? '已启用，SKILL.md 将注入系统提示' : '已禁用');
        // @ts-ignore
        [metaForm, metaForm,];
        var __VLS_589;
        let __VLS_597;
        /** @ts-ignore @type { | typeof __VLS_components.elCollapse | typeof __VLS_components.ElCollapse | typeof __VLS_components['el-collapse'] | typeof __VLS_components.elCollapse | typeof __VLS_components.ElCollapse | typeof __VLS_components['el-collapse']} */
        elCollapse;
        // @ts-ignore
        const __VLS_598 = __VLS_asFunctionalComponent1(__VLS_597, new __VLS_597({
            ...{ style: {} },
        }));
        const __VLS_599 = __VLS_598({
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_598));
        const { default: __VLS_602 } = __VLS_600.slots;
        let __VLS_603;
        /** @ts-ignore @type { | typeof __VLS_components.elCollapseItem | typeof __VLS_components.ElCollapseItem | typeof __VLS_components['el-collapse-item'] | typeof __VLS_components.elCollapseItem | typeof __VLS_components.ElCollapseItem | typeof __VLS_components['el-collapse-item']} */
        elCollapseItem;
        // @ts-ignore
        const __VLS_604 = __VLS_asFunctionalComponent1(__VLS_603, new __VLS_603({
            title: "查看 skill.json 原文",
        }));
        const __VLS_605 = __VLS_604({
            title: "查看 skill.json 原文",
        }, ...__VLS_functionalComponentArgsRest(__VLS_604));
        const { default: __VLS_608 } = __VLS_606.slots;
        __VLS_asFunctionalElement1(__VLS_intrinsics.pre, __VLS_intrinsics.pre)({
            ...{ class: "json-preview" },
        });
        /** @type {__VLS_StyleScopedClasses['json-preview']} */ ;
        (JSON.stringify({ id: __VLS_ctx.selected.id, ...__VLS_ctx.metaForm }, null, 2));
        // @ts-ignore
        [selected, metaForm,];
        var __VLS_606;
        // @ts-ignore
        [];
        var __VLS_600;
        // @ts-ignore
        [];
        var __VLS_481;
    }
    else if (__VLS_ctx.activeFile === 'prompt') {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "file-editor" },
        });
        /** @type {__VLS_StyleScopedClasses['file-editor']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "file-editor-head" },
        });
        /** @type {__VLS_StyleScopedClasses['file-editor-head']} */ ;
        let __VLS_609;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_610 = __VLS_asFunctionalComponent1(__VLS_609, new __VLS_609({}));
        const __VLS_611 = __VLS_610({}, ...__VLS_functionalComponentArgsRest(__VLS_610));
        const { default: __VLS_614 } = __VLS_612.slots;
        let __VLS_615;
        /** @ts-ignore @type { | typeof __VLS_components.Document} */
        Document;
        // @ts-ignore
        const __VLS_616 = __VLS_asFunctionalComponent1(__VLS_615, new __VLS_615({}));
        const __VLS_617 = __VLS_616({}, ...__VLS_functionalComponentArgsRest(__VLS_616));
        // @ts-ignore
        [activeFile,];
        var __VLS_612;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "file-hint" },
        });
        /** @type {__VLS_StyleScopedClasses['file-hint']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        if (__VLS_ctx.promptDirty) {
            let __VLS_620;
            /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
            elTag;
            // @ts-ignore
            const __VLS_621 = __VLS_asFunctionalComponent1(__VLS_620, new __VLS_620({
                type: "warning",
                size: "small",
            }));
            const __VLS_622 = __VLS_621({
                type: "warning",
                size: "small",
            }, ...__VLS_functionalComponentArgsRest(__VLS_621));
            const { default: __VLS_625 } = __VLS_623.slots;
            // @ts-ignore
            [promptDirty,];
            var __VLS_623;
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        (__VLS_ctx.promptLineCount);
        (__VLS_ctx.promptContent.length);
        let __VLS_626;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_627 = __VLS_asFunctionalComponent1(__VLS_626, new __VLS_626({
            ...{ 'onClick': {} },
            size: "small",
            circle: true,
            loading: (__VLS_ctx.promptLoading),
            title: "重新加载",
        }));
        const __VLS_628 = __VLS_627({
            ...{ 'onClick': {} },
            size: "small",
            circle: true,
            loading: (__VLS_ctx.promptLoading),
            title: "重新加载",
        }, ...__VLS_functionalComponentArgsRest(__VLS_627));
        let __VLS_631;
        const __VLS_632 = ({ click: {} },
            { onClick: (__VLS_ctx.reloadPrompt) });
        const { default: __VLS_633 } = __VLS_629.slots;
        let __VLS_634;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_635 = __VLS_asFunctionalComponent1(__VLS_634, new __VLS_634({}));
        const __VLS_636 = __VLS_635({}, ...__VLS_functionalComponentArgsRest(__VLS_635));
        const { default: __VLS_639 } = __VLS_637.slots;
        let __VLS_640;
        /** @ts-ignore @type { | typeof __VLS_components.Refresh} */
        Refresh;
        // @ts-ignore
        const __VLS_641 = __VLS_asFunctionalComponent1(__VLS_640, new __VLS_640({}));
        const __VLS_642 = __VLS_641({}, ...__VLS_functionalComponentArgsRest(__VLS_641));
        // @ts-ignore
        [promptLineCount, promptContent, promptLoading, reloadPrompt,];
        var __VLS_637;
        // @ts-ignore
        [];
        var __VLS_629;
        var __VLS_630;
        let __VLS_645;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_646 = __VLS_asFunctionalComponent1(__VLS_645, new __VLS_645({
            ...{ 'onClick': {} },
            size: "small",
            circle: true,
            type: (__VLS_ctx.editorFullscreen ? 'primary' : ''),
            title: (__VLS_ctx.editorFullscreen ? '退出全屏编辑' : '全屏编辑'),
        }));
        const __VLS_647 = __VLS_646({
            ...{ 'onClick': {} },
            size: "small",
            circle: true,
            type: (__VLS_ctx.editorFullscreen ? 'primary' : ''),
            title: (__VLS_ctx.editorFullscreen ? '退出全屏编辑' : '全屏编辑'),
        }, ...__VLS_functionalComponentArgsRest(__VLS_646));
        let __VLS_650;
        const __VLS_651 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!!(!__VLS_ctx.selected))
                        return;
                    if (!!(__VLS_ctx.activeFile === 'meta'))
                        return;
                    if (!(__VLS_ctx.activeFile === 'prompt'))
                        return;
                    __VLS_ctx.editorFullscreen = !__VLS_ctx.editorFullscreen;
                    // @ts-ignore
                    [editorFullscreen, editorFullscreen, editorFullscreen, editorFullscreen,];
                } });
        const { default: __VLS_652 } = __VLS_648.slots;
        let __VLS_653;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_654 = __VLS_asFunctionalComponent1(__VLS_653, new __VLS_653({}));
        const __VLS_655 = __VLS_654({}, ...__VLS_functionalComponentArgsRest(__VLS_654));
        const { default: __VLS_658 } = __VLS_656.slots;
        let __VLS_659;
        /** @ts-ignore @type { | typeof __VLS_components.FullScreen} */
        FullScreen;
        // @ts-ignore
        const __VLS_660 = __VLS_asFunctionalComponent1(__VLS_659, new __VLS_659({}));
        const __VLS_661 = __VLS_660({}, ...__VLS_functionalComponentArgsRest(__VLS_660));
        // @ts-ignore
        [];
        var __VLS_656;
        // @ts-ignore
        [];
        var __VLS_648;
        var __VLS_649;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "code-editor-wrap" },
        });
        /** @type {__VLS_StyleScopedClasses['code-editor-wrap']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "line-numbers" },
            'aria-hidden': "true",
        });
        /** @type {__VLS_StyleScopedClasses['line-numbers']} */ ;
        for (const [n] of __VLS_vFor((__VLS_ctx.promptLineCount))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                key: (n),
                ...{ class: "line-num" },
            });
            /** @type {__VLS_StyleScopedClasses['line-num']} */ ;
            (n);
            // @ts-ignore
            [promptLineCount,];
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.textarea)({
            ...{ onInput: (...[$event]) => {
                    if (!!(!__VLS_ctx.selected))
                        return;
                    if (!!(__VLS_ctx.activeFile === 'meta'))
                        return;
                    if (!(__VLS_ctx.activeFile === 'prompt'))
                        return;
                    __VLS_ctx.promptDirty = true;
                    // @ts-ignore
                    [promptDirty,];
                } },
            value: (__VLS_ctx.promptContent),
            ...{ class: "code-textarea" },
            spellcheck: "false",
            placeholder: "\u0023\u0020\u6280\u80fd\u540d\u79f0\u000a\u000a\u0023\u0023\u0020\u529f\u80fd\u8bf4\u660e\u000a\u63cf\u8ff0\u8be5\u6280\u80fd\u7684\u7528\u9014\u2026\u000a\u000a\u0023\u0023\u0020\u884c\u4e3a\u89c4\u8303\u000a\u002d\u0020\u89c4\u8303\u0020\u0031\u000a\u002d\u0020\u89c4\u8303\u0020\u0032",
        });
        /** @type {__VLS_StyleScopedClasses['code-textarea']} */ ;
        if (__VLS_ctx.pendingEdit?.file === 'SKILL.md') {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "diff-bar" },
            });
            /** @type {__VLS_StyleScopedClasses['diff-bar']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "diff-bar-left" },
            });
            /** @type {__VLS_StyleScopedClasses['diff-bar-left']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "diff-tag" },
            });
            /** @type {__VLS_StyleScopedClasses['diff-tag']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "diff-summary" },
            });
            /** @type {__VLS_StyleScopedClasses['diff-summary']} */ ;
            (__VLS_ctx.pendingEdit.summary);
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "diff-stats" },
            });
            /** @type {__VLS_StyleScopedClasses['diff-stats']} */ ;
            (__VLS_ctx.pendingEditStats);
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "diff-bar-right" },
            });
            /** @type {__VLS_StyleScopedClasses['diff-bar-right']} */ ;
            let __VLS_664;
            /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
            elButton;
            // @ts-ignore
            const __VLS_665 = __VLS_asFunctionalComponent1(__VLS_664, new __VLS_664({
                ...{ 'onClick': {} },
                size: "small",
                type: "primary",
            }));
            const __VLS_666 = __VLS_665({
                ...{ 'onClick': {} },
                size: "small",
                type: "primary",
            }, ...__VLS_functionalComponentArgsRest(__VLS_665));
            let __VLS_669;
            const __VLS_670 = ({ click: {} },
                { onClick: (__VLS_ctx.applyPendingEdit) });
            const { default: __VLS_671 } = __VLS_667.slots;
            // @ts-ignore
            [promptContent, pendingEdit, pendingEdit, pendingEditStats, applyPendingEdit,];
            var __VLS_667;
            var __VLS_668;
            let __VLS_672;
            /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
            elButton;
            // @ts-ignore
            const __VLS_673 = __VLS_asFunctionalComponent1(__VLS_672, new __VLS_672({
                ...{ 'onClick': {} },
                size: "small",
            }));
            const __VLS_674 = __VLS_673({
                ...{ 'onClick': {} },
                size: "small",
            }, ...__VLS_functionalComponentArgsRest(__VLS_673));
            let __VLS_677;
            const __VLS_678 = ({ click: {} },
                { onClick: (...[$event]) => {
                        if (!!(!__VLS_ctx.selected))
                            return;
                        if (!!(__VLS_ctx.activeFile === 'meta'))
                            return;
                        if (!(__VLS_ctx.activeFile === 'prompt'))
                            return;
                        if (!(__VLS_ctx.pendingEdit?.file === 'SKILL.md'))
                            return;
                        __VLS_ctx.pendingEdit = null;
                        // @ts-ignore
                        [pendingEdit,];
                    } });
            const { default: __VLS_679 } = __VLS_675.slots;
            // @ts-ignore
            [];
            var __VLS_675;
            var __VLS_676;
        }
    }
    else {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "file-editor" },
        });
        /** @type {__VLS_StyleScopedClasses['file-editor']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "file-editor-head" },
        });
        /** @type {__VLS_StyleScopedClasses['file-editor-head']} */ ;
        let __VLS_680;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_681 = __VLS_asFunctionalComponent1(__VLS_680, new __VLS_680({}));
        const __VLS_682 = __VLS_681({}, ...__VLS_functionalComponentArgsRest(__VLS_681));
        const { default: __VLS_685 } = __VLS_683.slots;
        let __VLS_686;
        /** @ts-ignore @type { | typeof __VLS_components.Document} */
        Document;
        // @ts-ignore
        const __VLS_687 = __VLS_asFunctionalComponent1(__VLS_686, new __VLS_686({}));
        const __VLS_688 = __VLS_687({}, ...__VLS_functionalComponentArgsRest(__VLS_687));
        // @ts-ignore
        [];
        var __VLS_683;
        (__VLS_ctx.activeFile);
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        if (__VLS_ctx.genericDirty) {
            let __VLS_691;
            /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
            elTag;
            // @ts-ignore
            const __VLS_692 = __VLS_asFunctionalComponent1(__VLS_691, new __VLS_691({
                type: "warning",
                size: "small",
            }));
            const __VLS_693 = __VLS_692({
                type: "warning",
                size: "small",
            }, ...__VLS_functionalComponentArgsRest(__VLS_692));
            const { default: __VLS_696 } = __VLS_694.slots;
            // @ts-ignore
            [activeFile, genericDirty,];
            var __VLS_694;
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        (__VLS_ctx.genericContent.length);
        let __VLS_697;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_698 = __VLS_asFunctionalComponent1(__VLS_697, new __VLS_697({
            ...{ 'onClick': {} },
            size: "small",
            circle: true,
            loading: (__VLS_ctx.genericLoading),
            title: "重新加载",
        }));
        const __VLS_699 = __VLS_698({
            ...{ 'onClick': {} },
            size: "small",
            circle: true,
            loading: (__VLS_ctx.genericLoading),
            title: "重新加载",
        }, ...__VLS_functionalComponentArgsRest(__VLS_698));
        let __VLS_702;
        const __VLS_703 = ({ click: {} },
            { onClick: (__VLS_ctx.reloadGenericFile) });
        const { default: __VLS_704 } = __VLS_700.slots;
        let __VLS_705;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_706 = __VLS_asFunctionalComponent1(__VLS_705, new __VLS_705({}));
        const __VLS_707 = __VLS_706({}, ...__VLS_functionalComponentArgsRest(__VLS_706));
        const { default: __VLS_710 } = __VLS_708.slots;
        let __VLS_711;
        /** @ts-ignore @type { | typeof __VLS_components.Refresh} */
        Refresh;
        // @ts-ignore
        const __VLS_712 = __VLS_asFunctionalComponent1(__VLS_711, new __VLS_711({}));
        const __VLS_713 = __VLS_712({}, ...__VLS_functionalComponentArgsRest(__VLS_712));
        // @ts-ignore
        [genericContent, genericLoading, reloadGenericFile,];
        var __VLS_708;
        // @ts-ignore
        [];
        var __VLS_700;
        var __VLS_701;
        let __VLS_716;
        /** @ts-ignore @type { | typeof __VLS_components.elPopconfirm | typeof __VLS_components.ElPopconfirm | typeof __VLS_components['el-popconfirm'] | typeof __VLS_components.elPopconfirm | typeof __VLS_components.ElPopconfirm | typeof __VLS_components['el-popconfirm']} */
        elPopconfirm;
        // @ts-ignore
        const __VLS_717 = __VLS_asFunctionalComponent1(__VLS_716, new __VLS_716({
            ...{ 'onConfirm': {} },
            title: "确认删除该文件？",
        }));
        const __VLS_718 = __VLS_717({
            ...{ 'onConfirm': {} },
            title: "确认删除该文件？",
        }, ...__VLS_functionalComponentArgsRest(__VLS_717));
        let __VLS_721;
        const __VLS_722 = ({ confirm: {} },
            { onConfirm: (...[$event]) => {
                    if (!!(!__VLS_ctx.selected))
                        return;
                    if (!!(__VLS_ctx.activeFile === 'meta'))
                        return;
                    if (!!(__VLS_ctx.activeFile === 'prompt'))
                        return;
                    __VLS_ctx.deleteFile(__VLS_ctx.activeFile);
                    // @ts-ignore
                    [activeFile, deleteFile,];
                } });
        const { default: __VLS_723 } = __VLS_719.slots;
        {
            const { reference: __VLS_724 } = __VLS_719.slots;
            let __VLS_725;
            /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
            elButton;
            // @ts-ignore
            const __VLS_726 = __VLS_asFunctionalComponent1(__VLS_725, new __VLS_725({
                size: "small",
                circle: true,
                type: "danger",
                plain: true,
            }));
            const __VLS_727 = __VLS_726({
                size: "small",
                circle: true,
                type: "danger",
                plain: true,
            }, ...__VLS_functionalComponentArgsRest(__VLS_726));
            const { default: __VLS_730 } = __VLS_728.slots;
            let __VLS_731;
            /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
            elIcon;
            // @ts-ignore
            const __VLS_732 = __VLS_asFunctionalComponent1(__VLS_731, new __VLS_731({}));
            const __VLS_733 = __VLS_732({}, ...__VLS_functionalComponentArgsRest(__VLS_732));
            const { default: __VLS_736 } = __VLS_734.slots;
            let __VLS_737;
            /** @ts-ignore @type { | typeof __VLS_components.Delete} */
            Delete;
            // @ts-ignore
            const __VLS_738 = __VLS_asFunctionalComponent1(__VLS_737, new __VLS_737({}));
            const __VLS_739 = __VLS_738({}, ...__VLS_functionalComponentArgsRest(__VLS_738));
            // @ts-ignore
            [];
            var __VLS_734;
            // @ts-ignore
            [];
            var __VLS_728;
            // @ts-ignore
            [];
        }
        // @ts-ignore
        [];
        var __VLS_719;
        var __VLS_720;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "code-editor-wrap" },
        });
        /** @type {__VLS_StyleScopedClasses['code-editor-wrap']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.textarea)({
            ...{ onInput: (...[$event]) => {
                    if (!!(!__VLS_ctx.selected))
                        return;
                    if (!!(__VLS_ctx.activeFile === 'meta'))
                        return;
                    if (!!(__VLS_ctx.activeFile === 'prompt'))
                        return;
                    __VLS_ctx.genericDirty = true;
                    // @ts-ignore
                    [genericDirty,];
                } },
            value: (__VLS_ctx.genericContent),
            ...{ class: "code-textarea" },
            spellcheck: "false",
            placeholder: (`编辑 ${__VLS_ctx.activeFile} …`),
        });
        /** @type {__VLS_StyleScopedClasses['code-textarea']} */ ;
    }
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ onMousedown: (...[$event]) => {
            __VLS_ctx.startResize($event, 'chat');
            // @ts-ignore
            [startResize, activeFile, genericContent,];
        } },
    ...{ class: "ss-handle" },
    ...{ class: ({ dragging: __VLS_ctx.dragging === 'chat' }) },
});
__VLS_asFunctionalDirective(__VLS_directives.vShow, {})(null, { ...__VLS_directiveBindingRestFields, value: (!__VLS_ctx.editorFullscreen) }, null, null);
/** @type {__VLS_StyleScopedClasses['ss-handle']} */ ;
/** @type {__VLS_StyleScopedClasses['dragging']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div)({
    ...{ class: "ss-handle-bar" },
});
/** @type {__VLS_StyleScopedClasses['ss-handle-bar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "studio-chat" },
    ...{ style: ({ width: __VLS_ctx.chatW + 'px' }) },
});
__VLS_asFunctionalDirective(__VLS_directives.vShow, {})(null, { ...__VLS_directiveBindingRestFields, value: (!__VLS_ctx.editorFullscreen) }, null, null);
/** @type {__VLS_StyleScopedClasses['studio-chat']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "chat-panel-head" },
});
/** @type {__VLS_StyleScopedClasses['chat-panel-head']} */ ;
let __VLS_742;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_743 = __VLS_asFunctionalComponent1(__VLS_742, new __VLS_742({}));
const __VLS_744 = __VLS_743({}, ...__VLS_functionalComponentArgsRest(__VLS_743));
const { default: __VLS_747 } = __VLS_745.slots;
let __VLS_748;
/** @ts-ignore @type { | typeof __VLS_components.ChatLineRound} */
ChatLineRound;
// @ts-ignore
const __VLS_749 = __VLS_asFunctionalComponent1(__VLS_748, new __VLS_748({}));
const __VLS_750 = __VLS_749({}, ...__VLS_functionalComponentArgsRest(__VLS_749));
// @ts-ignore
[dragging, editorFullscreen, editorFullscreen, chatW,];
var __VLS_745;
if (__VLS_ctx.selected) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ style: {} },
    });
    (__VLS_ctx.selected.name);
    if (__VLS_ctx.streamingSkills.size > 1) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
        (__VLS_ctx.streamingSkills.size);
    }
}
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "chat-wrap" },
});
/** @type {__VLS_StyleScopedClasses['chat-wrap']} */ ;
for (const [sk] of __VLS_vFor((__VLS_ctx.skills))) {
    const __VLS_753 = AiChat;
    // @ts-ignore
    const __VLS_754 = __VLS_asFunctionalComponent1(__VLS_753, new __VLS_753({
        ...{ 'onResponse': {} },
        ...{ 'onStreamingChange': {} },
        key: (sk.id),
        ref: ((el) => __VLS_ctx.setChatRef(sk.id, el)),
        agentId: (__VLS_ctx.agentId),
        context: (__VLS_ctx.selected?.id === sk.id ? __VLS_ctx.chatContext : ''),
        scenario: "skill-studio",
        skillId: (sk.id),
        welcomeMessage: (__VLS_ctx.selected?.id === sk.id ? __VLS_ctx.chatWelcome : ''),
        examples: (__VLS_ctx.selected?.id === sk.id ? __VLS_ctx.chatExamples : []),
        compact: true,
    }));
    const __VLS_755 = __VLS_754({
        ...{ 'onResponse': {} },
        ...{ 'onStreamingChange': {} },
        key: (sk.id),
        ref: ((el) => __VLS_ctx.setChatRef(sk.id, el)),
        agentId: (__VLS_ctx.agentId),
        context: (__VLS_ctx.selected?.id === sk.id ? __VLS_ctx.chatContext : ''),
        scenario: "skill-studio",
        skillId: (sk.id),
        welcomeMessage: (__VLS_ctx.selected?.id === sk.id ? __VLS_ctx.chatWelcome : ''),
        examples: (__VLS_ctx.selected?.id === sk.id ? __VLS_ctx.chatExamples : []),
        compact: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_754));
    let __VLS_758;
    const __VLS_759 = ({ response: {} },
        { onResponse: ((text) => __VLS_ctx.onAiResponse(sk.id, text)) });
    const __VLS_760 = ({ streamingChange: {} },
        { onStreamingChange: ((v) => __VLS_ctx.onStreamingChange(sk.id, v)) });
    __VLS_asFunctionalDirective(__VLS_directives.vShow, {})(null, { ...__VLS_directiveBindingRestFields, value: (__VLS_ctx.selected?.id === sk.id) }, null, null);
    var __VLS_756;
    var __VLS_757;
    // @ts-ignore
    [skills, selected, selected, selected, selected, selected, selected, streamingSkills, streamingSkills, setChatRef, agentId, chatContext, chatWelcome, chatExamples, onAiResponse, onStreamingChange,];
}
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({
    __typeProps: {},
});
export default {};
