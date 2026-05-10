/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed, onMounted } from 'vue';
import { Plus, FolderOpened, MoreFilled, Document, DocumentAdd, FolderAdd, Refresh, Delete, EditPen, Key, ArrowLeft } from '@element-plus/icons-vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { projects as projectsApi, agents as agentsApi } from '../api';
// ── State ─────────────────────────────────────────────────────────────────
const projectList = ref([]);
const currentProjectId = ref('');
const currentProject = computed(() => projectList.value.find(p => p.id === currentProjectId.value));
const treeData = ref([]);
const currentFile = ref('');
const fileContent = ref('');
const fileInfo = ref(null);
const isBinary = ref(false);
// Dialogs
const showCreate = ref(false);
const showEdit = ref(false);
// Permissions
const showPermissions = ref(false);
const permMode = ref('open');
const permEditors = ref([]);
const permSaving = ref(false);
const allAgents = ref([]);
function openPermissions() {
    const editors = currentProject.value?.editors ?? [];
    if (editors.length === 0) {
        permMode.value = 'open';
        permEditors.value = [];
    }
    else if (editors.includes('__none__')) {
        permMode.value = 'readonly';
        permEditors.value = [];
    }
    else {
        permMode.value = 'limited';
        permEditors.value = [...editors];
    }
    showPermissions.value = true;
    // Load agents list
    agentsApi.list().then(r => { allAgents.value = r.data || []; }).catch(() => { });
}
async function savePermissions() {
    if (!currentProject.value)
        return;
    permSaving.value = true;
    try {
        let editors = [];
        if (permMode.value === 'open')
            editors = [];
        else if (permMode.value === 'readonly')
            editors = ['__none__'];
        else
            editors = permEditors.value;
        const res = await projectsApi.setPermissions(currentProject.value.id, editors);
        // Update local project
        const idx = projectList.value.findIndex(p => p.id === currentProject.value.id);
        if (idx >= 0)
            projectList.value[idx] = res.data;
        ElMessage.success('权限已保存');
        showPermissions.value = false;
    }
    catch {
        ElMessage.error('保存失败');
    }
    finally {
        permSaving.value = false;
    }
}
const showNewFile = ref(false);
const showNewFolder = ref(false);
const newFilePath = ref('');
const newFolderPath = ref('');
const createForm = ref({ id: '', name: '', description: '', tagsStr: '' });
const editForm = ref({ id: '', name: '', description: '', tagsStr: '' });
// ── Init ──────────────────────────────────────────────────────────────────
onMounted(loadProjects);
async function loadProjects() {
    try {
        const res = await projectsApi.list();
        projectList.value = res.data || [];
        // Auto-select first project
        if (projectList.value.length && !currentProjectId.value) {
            const first = projectList.value[0];
            if (first)
                selectProject(first.id);
        }
    }
    catch { /* ignore */ }
}
// ── Project selection ─────────────────────────────────────────────────────
async function selectProject(id) {
    currentProjectId.value = id;
    currentFile.value = '';
    fileContent.value = '';
    fileInfo.value = null;
    await loadTree();
}
async function loadTree() {
    if (!currentProjectId.value)
        return;
    try {
        const res = await projectsApi.readTree(currentProjectId.value);
        treeData.value = Array.isArray(res.data) ? res.data : [];
    }
    catch {
        treeData.value = [];
    }
}
// ── File operations ───────────────────────────────────────────────────────
async function onFileClick(data) {
    if (data.isDir)
        return;
    currentFile.value = data.path || data.name;
    fileInfo.value = data;
    isBinary.value = false;
    try {
        const res = await projectsApi.readFile(currentProjectId.value, currentFile.value);
        if (res.data?.encoding === 'base64') {
            isBinary.value = true;
            fileContent.value = '';
        }
        else {
            fileContent.value = res.data?.content ?? '';
        }
    }
    catch {
        fileContent.value = '';
    }
}
async function saveFile() {
    if (!currentFile.value)
        return;
    try {
        await projectsApi.writeFile(currentProjectId.value, currentFile.value, fileContent.value);
        ElMessage.success('已保存');
        loadTree();
    }
    catch {
        ElMessage.error('保存失败');
    }
}
async function deleteCurrentFile() {
    if (!currentFile.value)
        return;
    try {
        await ElMessageBox.confirm(`删除「${currentFile.value}」？`, '删除文件', {
            confirmButtonText: '确认', cancelButtonText: '取消', type: 'warning',
            confirmButtonClass: 'el-button--danger',
        });
        await projectsApi.deleteFile(currentProjectId.value, currentFile.value);
        ElMessage.success('已删除');
        currentFile.value = '';
        fileContent.value = '';
        fileInfo.value = null;
        loadTree();
    }
    catch (e) {
        if (e !== 'cancel')
            ElMessage.error('删除失败');
    }
}
async function doNewFile() {
    const p = newFilePath.value.trim();
    if (!p)
        return;
    try {
        await projectsApi.writeFile(currentProjectId.value, p, '');
        ElMessage.success(`已创建 ${p}`);
        showNewFile.value = false;
        newFilePath.value = '';
        await loadTree();
        currentFile.value = p;
        fileContent.value = '';
        fileInfo.value = null;
        isBinary.value = false;
    }
    catch {
        ElMessage.error('创建失败');
    }
}
async function doNewFolder() {
    const p = newFolderPath.value.trim();
    if (!p)
        return;
    // Create a .gitkeep to materialise the directory
    try {
        await projectsApi.writeFile(currentProjectId.value, p + '/.gitkeep', '');
        ElMessage.success(`已创建文件夹 ${p}`);
        showNewFolder.value = false;
        newFolderPath.value = '';
        loadTree();
    }
    catch {
        ElMessage.error('创建失败');
    }
}
// ── Project CRUD ──────────────────────────────────────────────────────────
function resetForm() {
    createForm.value = { id: '', name: '', description: '', tagsStr: '' };
}
async function doCreate() {
    const { id, name, description, tagsStr } = createForm.value;
    if (!id || !name) {
        ElMessage.warning('请填写 ID 和名称');
        return;
    }
    const tags = tagsStr ? tagsStr.split(',').map(t => t.trim()).filter(Boolean) : [];
    try {
        await projectsApi.create({ id, name, description, tags });
        ElMessage.success('项目已创建');
        showCreate.value = false;
        resetForm();
        await loadProjects();
        selectProject(id);
    }
    catch (e) {
        ElMessage.error(e.response?.data?.error || '创建失败');
    }
}
function openEdit(p) {
    editForm.value = {
        id: p.id, name: p.name,
        description: p.description || '',
        tagsStr: (p.tags || []).join(', '),
    };
    showEdit.value = true;
}
async function doEdit() {
    const { id, name, description, tagsStr } = editForm.value;
    const tags = tagsStr ? tagsStr.split(',').map(t => t.trim()).filter(Boolean) : [];
    try {
        await projectsApi.update(id, { name, description, tags });
        ElMessage.success('已更新');
        showEdit.value = false;
        loadProjects();
    }
    catch {
        ElMessage.error('更新失败');
    }
}
async function confirmDelete(p) {
    try {
        await ElMessageBox.confirm(`删除项目「${p.name}」将删除其所有文件，不可恢复。`, '删除项目', { confirmButtonText: '确认删除', cancelButtonText: '取消', type: 'warning', confirmButtonClass: 'el-button--danger' });
        await projectsApi.delete(p.id);
        ElMessage.success('已删除');
        if (currentProjectId.value === p.id) {
            currentProjectId.value = '';
            treeData.value = [];
            currentFile.value = '';
        }
        loadProjects();
    }
    catch (e) {
        if (e !== 'cancel')
            ElMessage.error('删除失败');
    }
}
// ── Helpers ───────────────────────────────────────────────────────────────
function fileColor(name) {
    const ext = name.split('.').pop()?.toLowerCase() || '';
    if (['md', 'txt', 'rst'].includes(ext))
        return '#409eff';
    if (['json', 'yaml', 'yml', 'toml'].includes(ext))
        return '#67c23a';
    if (['go', 'py', 'js', 'ts', 'sh', 'vue', 'rs', 'java', 'c', 'cpp'].includes(ext))
        return '#e6a23c';
    if (['jpg', 'jpeg', 'png', 'gif', 'svg', 'webp'].includes(ext))
        return '#f56c6c';
    if (name.startsWith('.'))
        return '#c0c4cc';
    return '#909399';
}
function fileExt(path) {
    const name = path.split('/').pop() || path;
    const dot = name.lastIndexOf('.');
    return dot > 0 ? name.slice(dot) : 'file';
}
function fmtSize(bytes) {
    if (!bytes)
        return '0 B';
    if (bytes < 1024)
        return `${bytes} B`;
    if (bytes < 1024 * 1024)
        return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}
function fmtTime(t) {
    if (!t)
        return '';
    return new Date(t).toLocaleString('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}
const __VLS_ctx = {
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['project-item']} */ ;
/** @type {__VLS_StyleScopedClasses['project-item']} */ ;
/** @type {__VLS_StyleScopedClasses['project-menu-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['editor-panel']} */ ;
/** @type {__VLS_StyleScopedClasses['editor-panel']} */ ;
/** @type {__VLS_StyleScopedClasses['el-textarea']} */ ;
/** @type {__VLS_StyleScopedClasses['editor-panel']} */ ;
/** @type {__VLS_StyleScopedClasses['projects-layout']} */ ;
/** @type {__VLS_StyleScopedClasses['projects-sidebar']} */ ;
/** @type {__VLS_StyleScopedClasses['project-list']} */ ;
/** @type {__VLS_StyleScopedClasses['projects-main']} */ ;
/** @type {__VLS_StyleScopedClasses['mobile-back-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['file-panel']} */ ;
/** @type {__VLS_StyleScopedClasses['editor-panel']} */ ;
/** @type {__VLS_StyleScopedClasses['main-header']} */ ;
/** @type {__VLS_StyleScopedClasses['main-title']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "projects-layout" },
});
/** @type {__VLS_StyleScopedClasses['projects-layout']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "projects-sidebar" },
});
/** @type {__VLS_StyleScopedClasses['projects-sidebar']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "sidebar-header" },
});
/** @type {__VLS_StyleScopedClasses['sidebar-header']} */ ;
__VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
    ...{ class: "sidebar-title" },
});
/** @type {__VLS_StyleScopedClasses['sidebar-title']} */ ;
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
elButton;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
    ...{ 'onClick': {} },
    text: true,
    size: "small",
    title: "新建项目",
}));
const __VLS_2 = __VLS_1({
    ...{ 'onClick': {} },
    text: true,
    size: "small",
    title: "新建项目",
}, ...__VLS_functionalComponentArgsRest(__VLS_1));
let __VLS_5;
const __VLS_6 = ({ click: {} },
    { onClick: (...[$event]) => {
            __VLS_ctx.showCreate = true;
            // @ts-ignore
            [showCreate,];
        } });
const { default: __VLS_7 } = __VLS_3.slots;
let __VLS_8;
/** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
elIcon;
// @ts-ignore
const __VLS_9 = __VLS_asFunctionalComponent1(__VLS_8, new __VLS_8({}));
const __VLS_10 = __VLS_9({}, ...__VLS_functionalComponentArgsRest(__VLS_9));
const { default: __VLS_13 } = __VLS_11.slots;
let __VLS_14;
/** @ts-ignore @type { | typeof __VLS_components.Plus} */
Plus;
// @ts-ignore
const __VLS_15 = __VLS_asFunctionalComponent1(__VLS_14, new __VLS_14({}));
const __VLS_16 = __VLS_15({}, ...__VLS_functionalComponentArgsRest(__VLS_15));
// @ts-ignore
[];
var __VLS_11;
// @ts-ignore
[];
var __VLS_3;
var __VLS_4;
__VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
    ...{ class: "project-list" },
});
/** @type {__VLS_StyleScopedClasses['project-list']} */ ;
for (const [p] of __VLS_vFor((__VLS_ctx.projectList))) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                __VLS_ctx.selectProject(p.id);
                // @ts-ignore
                [projectList, selectProject,];
            } },
        key: (p.id),
        ...{ class: "project-item" },
        ...{ class: ({ active: __VLS_ctx.currentProjectId === p.id }) },
    });
    /** @type {__VLS_StyleScopedClasses['project-item']} */ ;
    /** @type {__VLS_StyleScopedClasses['active']} */ ;
    let __VLS_19;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_20 = __VLS_asFunctionalComponent1(__VLS_19, new __VLS_19({
        ...{ class: "project-icon" },
    }));
    const __VLS_21 = __VLS_20({
        ...{ class: "project-icon" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_20));
    /** @type {__VLS_StyleScopedClasses['project-icon']} */ ;
    const { default: __VLS_24 } = __VLS_22.slots;
    let __VLS_25;
    /** @ts-ignore @type { | typeof __VLS_components.FolderOpened} */
    FolderOpened;
    // @ts-ignore
    const __VLS_26 = __VLS_asFunctionalComponent1(__VLS_25, new __VLS_25({}));
    const __VLS_27 = __VLS_26({}, ...__VLS_functionalComponentArgsRest(__VLS_26));
    // @ts-ignore
    [currentProjectId,];
    var __VLS_22;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "project-item-body" },
    });
    /** @type {__VLS_StyleScopedClasses['project-item-body']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "project-name" },
    });
    /** @type {__VLS_StyleScopedClasses['project-name']} */ ;
    (p.name);
    if (p.description) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "project-desc" },
        });
        /** @type {__VLS_StyleScopedClasses['project-desc']} */ ;
        (p.description);
    }
    let __VLS_30;
    /** @ts-ignore @type { | typeof __VLS_components.elDropdown | typeof __VLS_components.ElDropdown | typeof __VLS_components['el-dropdown'] | typeof __VLS_components.elDropdown | typeof __VLS_components.ElDropdown | typeof __VLS_components['el-dropdown']} */
    elDropdown;
    // @ts-ignore
    const __VLS_31 = __VLS_asFunctionalComponent1(__VLS_30, new __VLS_30({
        ...{ 'onClick': {} },
        trigger: "click",
    }));
    const __VLS_32 = __VLS_31({
        ...{ 'onClick': {} },
        trigger: "click",
    }, ...__VLS_functionalComponentArgsRest(__VLS_31));
    let __VLS_35;
    const __VLS_36 = ({ click: {} },
        { onClick: () => { } });
    const { default: __VLS_37 } = __VLS_33.slots;
    let __VLS_38;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_39 = __VLS_asFunctionalComponent1(__VLS_38, new __VLS_38({
        ...{ class: "project-menu-btn" },
    }));
    const __VLS_40 = __VLS_39({
        ...{ class: "project-menu-btn" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_39));
    /** @type {__VLS_StyleScopedClasses['project-menu-btn']} */ ;
    const { default: __VLS_43 } = __VLS_41.slots;
    let __VLS_44;
    /** @ts-ignore @type { | typeof __VLS_components.MoreFilled} */
    MoreFilled;
    // @ts-ignore
    const __VLS_45 = __VLS_asFunctionalComponent1(__VLS_44, new __VLS_44({}));
    const __VLS_46 = __VLS_45({}, ...__VLS_functionalComponentArgsRest(__VLS_45));
    // @ts-ignore
    [];
    var __VLS_41;
    {
        const { dropdown: __VLS_49 } = __VLS_33.slots;
        let __VLS_50;
        /** @ts-ignore @type { | typeof __VLS_components.elDropdownMenu | typeof __VLS_components.ElDropdownMenu | typeof __VLS_components['el-dropdown-menu'] | typeof __VLS_components.elDropdownMenu | typeof __VLS_components.ElDropdownMenu | typeof __VLS_components['el-dropdown-menu']} */
        elDropdownMenu;
        // @ts-ignore
        const __VLS_51 = __VLS_asFunctionalComponent1(__VLS_50, new __VLS_50({}));
        const __VLS_52 = __VLS_51({}, ...__VLS_functionalComponentArgsRest(__VLS_51));
        const { default: __VLS_55 } = __VLS_53.slots;
        let __VLS_56;
        /** @ts-ignore @type { | typeof __VLS_components.elDropdownItem | typeof __VLS_components.ElDropdownItem | typeof __VLS_components['el-dropdown-item'] | typeof __VLS_components.elDropdownItem | typeof __VLS_components.ElDropdownItem | typeof __VLS_components['el-dropdown-item']} */
        elDropdownItem;
        // @ts-ignore
        const __VLS_57 = __VLS_asFunctionalComponent1(__VLS_56, new __VLS_56({
            ...{ 'onClick': {} },
        }));
        const __VLS_58 = __VLS_57({
            ...{ 'onClick': {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_57));
        let __VLS_61;
        const __VLS_62 = ({ click: {} },
            { onClick: (...[$event]) => {
                    __VLS_ctx.openEdit(p);
                    // @ts-ignore
                    [openEdit,];
                } });
        const { default: __VLS_63 } = __VLS_59.slots;
        // @ts-ignore
        [];
        var __VLS_59;
        var __VLS_60;
        let __VLS_64;
        /** @ts-ignore @type { | typeof __VLS_components.elDropdownItem | typeof __VLS_components.ElDropdownItem | typeof __VLS_components['el-dropdown-item'] | typeof __VLS_components.elDropdownItem | typeof __VLS_components.ElDropdownItem | typeof __VLS_components['el-dropdown-item']} */
        elDropdownItem;
        // @ts-ignore
        const __VLS_65 = __VLS_asFunctionalComponent1(__VLS_64, new __VLS_64({
            ...{ 'onClick': {} },
            divided: true,
            ...{ style: {} },
        }));
        const __VLS_66 = __VLS_65({
            ...{ 'onClick': {} },
            divided: true,
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_65));
        let __VLS_69;
        const __VLS_70 = ({ click: {} },
            { onClick: (...[$event]) => {
                    __VLS_ctx.confirmDelete(p);
                    // @ts-ignore
                    [confirmDelete,];
                } });
        const { default: __VLS_71 } = __VLS_67.slots;
        // @ts-ignore
        [];
        var __VLS_67;
        var __VLS_68;
        // @ts-ignore
        [];
        var __VLS_53;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_33;
    var __VLS_34;
    // @ts-ignore
    [];
}
if (!__VLS_ctx.projectList.length) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "empty-projects" },
    });
    /** @type {__VLS_StyleScopedClasses['empty-projects']} */ ;
    let __VLS_72;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_73 = __VLS_asFunctionalComponent1(__VLS_72, new __VLS_72({
        size: "32",
        ...{ style: {} },
    }));
    const __VLS_74 = __VLS_73({
        size: "32",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_73));
    const { default: __VLS_77 } = __VLS_75.slots;
    let __VLS_78;
    /** @ts-ignore @type { | typeof __VLS_components.FolderOpened} */
    FolderOpened;
    // @ts-ignore
    const __VLS_79 = __VLS_asFunctionalComponent1(__VLS_78, new __VLS_78({}));
    const __VLS_80 = __VLS_79({}, ...__VLS_functionalComponentArgsRest(__VLS_79));
    // @ts-ignore
    [projectList,];
    var __VLS_75;
    __VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({});
    let __VLS_83;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_84 = __VLS_asFunctionalComponent1(__VLS_83, new __VLS_83({
        ...{ 'onClick': {} },
        size: "small",
        type: "primary",
        plain: true,
    }));
    const __VLS_85 = __VLS_84({
        ...{ 'onClick': {} },
        size: "small",
        type: "primary",
        plain: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_84));
    let __VLS_88;
    const __VLS_89 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!(!__VLS_ctx.projectList.length))
                    return;
                __VLS_ctx.showCreate = true;
                // @ts-ignore
                [showCreate,];
            } });
    const { default: __VLS_90 } = __VLS_86.slots;
    // @ts-ignore
    [];
    var __VLS_86;
    var __VLS_87;
}
if (__VLS_ctx.currentProject) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "projects-main" },
    });
    /** @type {__VLS_StyleScopedClasses['projects-main']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "main-header" },
    });
    /** @type {__VLS_StyleScopedClasses['main-header']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "main-title" },
    });
    /** @type {__VLS_StyleScopedClasses['main-title']} */ ;
    let __VLS_91;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_92 = __VLS_asFunctionalComponent1(__VLS_91, new __VLS_91({
        ...{ 'onClick': {} },
        ...{ class: "mobile-back-btn" },
        size: "small",
        icon: (__VLS_ctx.ArrowLeft),
        circle: true,
    }));
    const __VLS_93 = __VLS_92({
        ...{ 'onClick': {} },
        ...{ class: "mobile-back-btn" },
        size: "small",
        icon: (__VLS_ctx.ArrowLeft),
        circle: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_92));
    let __VLS_96;
    const __VLS_97 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!(__VLS_ctx.currentProject))
                    return;
                __VLS_ctx.currentProjectId = '';
                // @ts-ignore
                [currentProjectId, currentProject, ArrowLeft,];
            } });
    /** @type {__VLS_StyleScopedClasses['mobile-back-btn']} */ ;
    var __VLS_94;
    var __VLS_95;
    let __VLS_98;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_99 = __VLS_asFunctionalComponent1(__VLS_98, new __VLS_98({
        ...{ style: {} },
    }));
    const __VLS_100 = __VLS_99({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_99));
    const { default: __VLS_103 } = __VLS_101.slots;
    let __VLS_104;
    /** @ts-ignore @type { | typeof __VLS_components.FolderOpened} */
    FolderOpened;
    // @ts-ignore
    const __VLS_105 = __VLS_asFunctionalComponent1(__VLS_104, new __VLS_104({}));
    const __VLS_106 = __VLS_105({}, ...__VLS_functionalComponentArgsRest(__VLS_105));
    // @ts-ignore
    [];
    var __VLS_101;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "main-title-text" },
    });
    /** @type {__VLS_StyleScopedClasses['main-title-text']} */ ;
    (__VLS_ctx.currentProject.name);
    for (const [tag] of __VLS_vFor(((__VLS_ctx.currentProject.tags || [])))) {
        let __VLS_109;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_110 = __VLS_asFunctionalComponent1(__VLS_109, new __VLS_109({
            key: (tag),
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_111 = __VLS_110({
            key: (tag),
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_110));
        const { default: __VLS_114 } = __VLS_112.slots;
        (tag);
        // @ts-ignore
        [currentProject, currentProject,];
        var __VLS_112;
        // @ts-ignore
        [];
    }
    if (__VLS_ctx.currentProject.editors?.length === 0) {
        let __VLS_115;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_116 = __VLS_asFunctionalComponent1(__VLS_115, new __VLS_115({
            size: "small",
            type: "success",
            ...{ style: {} },
        }));
        const __VLS_117 = __VLS_116({
            size: "small",
            type: "success",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_116));
        const { default: __VLS_120 } = __VLS_118.slots;
        // @ts-ignore
        [currentProject,];
        var __VLS_118;
    }
    else {
        let __VLS_121;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_122 = __VLS_asFunctionalComponent1(__VLS_121, new __VLS_121({
            size: "small",
            type: "warning",
            ...{ style: {} },
        }));
        const __VLS_123 = __VLS_122({
            size: "small",
            type: "warning",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_122));
        const { default: __VLS_126 } = __VLS_124.slots;
        (__VLS_ctx.currentProject.editors?.length);
        // @ts-ignore
        [currentProject,];
        var __VLS_124;
    }
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    if (__VLS_ctx.currentProject.description) {
        let __VLS_127;
        /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
        elText;
        // @ts-ignore
        const __VLS_128 = __VLS_asFunctionalComponent1(__VLS_127, new __VLS_127({
            type: "info",
            size: "small",
        }));
        const __VLS_129 = __VLS_128({
            type: "info",
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_128));
        const { default: __VLS_132 } = __VLS_130.slots;
        (__VLS_ctx.currentProject.description);
        // @ts-ignore
        [currentProject, currentProject,];
        var __VLS_130;
    }
    let __VLS_133;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_134 = __VLS_asFunctionalComponent1(__VLS_133, new __VLS_133({
        ...{ 'onClick': {} },
        size: "small",
        plain: true,
    }));
    const __VLS_135 = __VLS_134({
        ...{ 'onClick': {} },
        size: "small",
        plain: true,
    }, ...__VLS_functionalComponentArgsRest(__VLS_134));
    let __VLS_138;
    const __VLS_139 = ({ click: {} },
        { onClick: (__VLS_ctx.openPermissions) });
    const { default: __VLS_140 } = __VLS_136.slots;
    let __VLS_141;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_142 = __VLS_asFunctionalComponent1(__VLS_141, new __VLS_141({
        ...{ style: {} },
    }));
    const __VLS_143 = __VLS_142({
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_142));
    const { default: __VLS_146 } = __VLS_144.slots;
    let __VLS_147;
    /** @ts-ignore @type { | typeof __VLS_components.Key} */
    Key;
    // @ts-ignore
    const __VLS_148 = __VLS_asFunctionalComponent1(__VLS_147, new __VLS_147({}));
    const __VLS_149 = __VLS_148({}, ...__VLS_functionalComponentArgsRest(__VLS_148));
    // @ts-ignore
    [openPermissions,];
    var __VLS_144;
    // @ts-ignore
    [];
    var __VLS_136;
    var __VLS_137;
    let __VLS_152;
    /** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
    elDialog;
    // @ts-ignore
    const __VLS_153 = __VLS_asFunctionalComponent1(__VLS_152, new __VLS_152({
        modelValue: (__VLS_ctx.showPermissions),
        title: "成员写入权限",
        width: "500px",
    }));
    const __VLS_154 = __VLS_153({
        modelValue: (__VLS_ctx.showPermissions),
        title: "成员写入权限",
        width: "500px",
    }, ...__VLS_functionalComponentArgsRest(__VLS_153));
    const { default: __VLS_157 } = __VLS_155.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.br)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.strong, __VLS_intrinsics.strong)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_158;
    /** @ts-ignore @type { | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group'] | typeof __VLS_components.elRadioGroup | typeof __VLS_components.ElRadioGroup | typeof __VLS_components['el-radio-group']} */
    elRadioGroup;
    // @ts-ignore
    const __VLS_159 = __VLS_asFunctionalComponent1(__VLS_158, new __VLS_158({
        modelValue: (__VLS_ctx.permMode),
        size: "small",
    }));
    const __VLS_160 = __VLS_159({
        modelValue: (__VLS_ctx.permMode),
        size: "small",
    }, ...__VLS_functionalComponentArgsRest(__VLS_159));
    const { default: __VLS_163 } = __VLS_161.slots;
    let __VLS_164;
    /** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
    elRadioButton;
    // @ts-ignore
    const __VLS_165 = __VLS_asFunctionalComponent1(__VLS_164, new __VLS_164({
        label: "open",
    }));
    const __VLS_166 = __VLS_165({
        label: "open",
    }, ...__VLS_functionalComponentArgsRest(__VLS_165));
    const { default: __VLS_169 } = __VLS_167.slots;
    // @ts-ignore
    [showPermissions, permMode,];
    var __VLS_167;
    let __VLS_170;
    /** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
    elRadioButton;
    // @ts-ignore
    const __VLS_171 = __VLS_asFunctionalComponent1(__VLS_170, new __VLS_170({
        label: "limited",
    }));
    const __VLS_172 = __VLS_171({
        label: "limited",
    }, ...__VLS_functionalComponentArgsRest(__VLS_171));
    const { default: __VLS_175 } = __VLS_173.slots;
    // @ts-ignore
    [];
    var __VLS_173;
    let __VLS_176;
    /** @ts-ignore @type { | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button'] | typeof __VLS_components.elRadioButton | typeof __VLS_components.ElRadioButton | typeof __VLS_components['el-radio-button']} */
    elRadioButton;
    // @ts-ignore
    const __VLS_177 = __VLS_asFunctionalComponent1(__VLS_176, new __VLS_176({
        label: "readonly",
    }));
    const __VLS_178 = __VLS_177({
        label: "readonly",
    }, ...__VLS_functionalComponentArgsRest(__VLS_177));
    const { default: __VLS_181 } = __VLS_179.slots;
    // @ts-ignore
    [];
    var __VLS_179;
    // @ts-ignore
    [];
    var __VLS_161;
    if (__VLS_ctx.permMode === 'limited') {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ style: {} },
        });
        let __VLS_182;
        /** @ts-ignore @type { | typeof __VLS_components.elCheckboxGroup | typeof __VLS_components.ElCheckboxGroup | typeof __VLS_components['el-checkbox-group'] | typeof __VLS_components.elCheckboxGroup | typeof __VLS_components.ElCheckboxGroup | typeof __VLS_components['el-checkbox-group']} */
        elCheckboxGroup;
        // @ts-ignore
        const __VLS_183 = __VLS_asFunctionalComponent1(__VLS_182, new __VLS_182({
            modelValue: (__VLS_ctx.permEditors),
        }));
        const __VLS_184 = __VLS_183({
            modelValue: (__VLS_ctx.permEditors),
        }, ...__VLS_functionalComponentArgsRest(__VLS_183));
        const { default: __VLS_187 } = __VLS_185.slots;
        for (const [a] of __VLS_vFor((__VLS_ctx.allAgents))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                key: (a.id),
                ...{ style: {} },
            });
            let __VLS_188;
            /** @ts-ignore @type { | typeof __VLS_components.elCheckbox | typeof __VLS_components.ElCheckbox | typeof __VLS_components['el-checkbox'] | typeof __VLS_components.elCheckbox | typeof __VLS_components.ElCheckbox | typeof __VLS_components['el-checkbox']} */
            elCheckbox;
            // @ts-ignore
            const __VLS_189 = __VLS_asFunctionalComponent1(__VLS_188, new __VLS_188({
                label: (a.id),
            }));
            const __VLS_190 = __VLS_189({
                label: (a.id),
            }, ...__VLS_functionalComponentArgsRest(__VLS_189));
            const { default: __VLS_193 } = __VLS_191.slots;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ style: {} },
            });
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ style: ({
                        width: '20px', height: '20px', borderRadius: '50%',
                        background: a.avatarColor || '#6366f1',
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                        fontSize: '10px', color: '#fff'
                    }) },
            });
            (a.name.charAt(0));
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
            (a.name);
            if (a.system) {
                let __VLS_194;
                /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
                elTag;
                // @ts-ignore
                const __VLS_195 = __VLS_asFunctionalComponent1(__VLS_194, new __VLS_194({
                    size: "small",
                    type: "info",
                }));
                const __VLS_196 = __VLS_195({
                    size: "small",
                    type: "info",
                }, ...__VLS_functionalComponentArgsRest(__VLS_195));
                const { default: __VLS_199 } = __VLS_197.slots;
                // @ts-ignore
                [permMode, permEditors, allAgents,];
                var __VLS_197;
            }
            // @ts-ignore
            [];
            var __VLS_191;
            // @ts-ignore
            [];
        }
        // @ts-ignore
        [];
        var __VLS_185;
    }
    {
        const { footer: __VLS_200 } = __VLS_155.slots;
        let __VLS_201;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_202 = __VLS_asFunctionalComponent1(__VLS_201, new __VLS_201({
            ...{ 'onClick': {} },
        }));
        const __VLS_203 = __VLS_202({
            ...{ 'onClick': {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_202));
        let __VLS_206;
        const __VLS_207 = ({ click: {} },
            { onClick: (...[$event]) => {
                    if (!(__VLS_ctx.currentProject))
                        return;
                    __VLS_ctx.showPermissions = false;
                    // @ts-ignore
                    [showPermissions,];
                } });
        const { default: __VLS_208 } = __VLS_204.slots;
        // @ts-ignore
        [];
        var __VLS_204;
        var __VLS_205;
        let __VLS_209;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_210 = __VLS_asFunctionalComponent1(__VLS_209, new __VLS_209({
            ...{ 'onClick': {} },
            type: "primary",
            loading: (__VLS_ctx.permSaving),
        }));
        const __VLS_211 = __VLS_210({
            ...{ 'onClick': {} },
            type: "primary",
            loading: (__VLS_ctx.permSaving),
        }, ...__VLS_functionalComponentArgsRest(__VLS_210));
        let __VLS_214;
        const __VLS_215 = ({ click: {} },
            { onClick: (__VLS_ctx.savePermissions) });
        const { default: __VLS_216 } = __VLS_212.slots;
        // @ts-ignore
        [permSaving, savePermissions,];
        var __VLS_212;
        var __VLS_213;
        // @ts-ignore
        [];
    }
    // @ts-ignore
    [];
    var __VLS_155;
    let __VLS_217;
    /** @ts-ignore @type { | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row'] | typeof __VLS_components.elRow | typeof __VLS_components.ElRow | typeof __VLS_components['el-row']} */
    elRow;
    // @ts-ignore
    const __VLS_218 = __VLS_asFunctionalComponent1(__VLS_217, new __VLS_217({
        gutter: (12),
        ...{ class: "projects-inner-row" },
        ...{ style: {} },
    }));
    const __VLS_219 = __VLS_218({
        gutter: (12),
        ...{ class: "projects-inner-row" },
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_218));
    /** @type {__VLS_StyleScopedClasses['projects-inner-row']} */ ;
    const { default: __VLS_222 } = __VLS_220.slots;
    let __VLS_223;
    /** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
    elCol;
    // @ts-ignore
    const __VLS_224 = __VLS_asFunctionalComponent1(__VLS_223, new __VLS_223({
        xs: (24),
        sm: (6),
        md: (6),
        ...{ class: "file-col" },
    }));
    const __VLS_225 = __VLS_224({
        xs: (24),
        sm: (6),
        md: (6),
        ...{ class: "file-col" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_224));
    /** @type {__VLS_StyleScopedClasses['file-col']} */ ;
    const { default: __VLS_228 } = __VLS_226.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "file-panel" },
    });
    /** @type {__VLS_StyleScopedClasses['file-panel']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "file-panel-header" },
    });
    /** @type {__VLS_StyleScopedClasses['file-panel-header']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    let __VLS_229;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_230 = __VLS_asFunctionalComponent1(__VLS_229, new __VLS_229({
        ...{ 'onClick': {} },
        text: true,
        size: "small",
        title: "新建文件",
    }));
    const __VLS_231 = __VLS_230({
        ...{ 'onClick': {} },
        text: true,
        size: "small",
        title: "新建文件",
    }, ...__VLS_functionalComponentArgsRest(__VLS_230));
    let __VLS_234;
    const __VLS_235 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!(__VLS_ctx.currentProject))
                    return;
                __VLS_ctx.showNewFile = true;
                // @ts-ignore
                [showNewFile,];
            } });
    const { default: __VLS_236 } = __VLS_232.slots;
    let __VLS_237;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_238 = __VLS_asFunctionalComponent1(__VLS_237, new __VLS_237({}));
    const __VLS_239 = __VLS_238({}, ...__VLS_functionalComponentArgsRest(__VLS_238));
    const { default: __VLS_242 } = __VLS_240.slots;
    let __VLS_243;
    /** @ts-ignore @type { | typeof __VLS_components.DocumentAdd} */
    DocumentAdd;
    // @ts-ignore
    const __VLS_244 = __VLS_asFunctionalComponent1(__VLS_243, new __VLS_243({}));
    const __VLS_245 = __VLS_244({}, ...__VLS_functionalComponentArgsRest(__VLS_244));
    // @ts-ignore
    [];
    var __VLS_240;
    // @ts-ignore
    [];
    var __VLS_232;
    var __VLS_233;
    let __VLS_248;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_249 = __VLS_asFunctionalComponent1(__VLS_248, new __VLS_248({
        ...{ 'onClick': {} },
        text: true,
        size: "small",
        title: "新建文件夹",
    }));
    const __VLS_250 = __VLS_249({
        ...{ 'onClick': {} },
        text: true,
        size: "small",
        title: "新建文件夹",
    }, ...__VLS_functionalComponentArgsRest(__VLS_249));
    let __VLS_253;
    const __VLS_254 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!(__VLS_ctx.currentProject))
                    return;
                __VLS_ctx.showNewFolder = true;
                // @ts-ignore
                [showNewFolder,];
            } });
    const { default: __VLS_255 } = __VLS_251.slots;
    let __VLS_256;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_257 = __VLS_asFunctionalComponent1(__VLS_256, new __VLS_256({}));
    const __VLS_258 = __VLS_257({}, ...__VLS_functionalComponentArgsRest(__VLS_257));
    const { default: __VLS_261 } = __VLS_259.slots;
    let __VLS_262;
    /** @ts-ignore @type { | typeof __VLS_components.FolderAdd} */
    FolderAdd;
    // @ts-ignore
    const __VLS_263 = __VLS_asFunctionalComponent1(__VLS_262, new __VLS_262({}));
    const __VLS_264 = __VLS_263({}, ...__VLS_functionalComponentArgsRest(__VLS_263));
    // @ts-ignore
    [];
    var __VLS_259;
    // @ts-ignore
    [];
    var __VLS_251;
    var __VLS_252;
    let __VLS_267;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_268 = __VLS_asFunctionalComponent1(__VLS_267, new __VLS_267({
        ...{ 'onClick': {} },
        text: true,
        size: "small",
        title: "刷新",
    }));
    const __VLS_269 = __VLS_268({
        ...{ 'onClick': {} },
        text: true,
        size: "small",
        title: "刷新",
    }, ...__VLS_functionalComponentArgsRest(__VLS_268));
    let __VLS_272;
    const __VLS_273 = ({ click: {} },
        { onClick: (__VLS_ctx.loadTree) });
    const { default: __VLS_274 } = __VLS_270.slots;
    let __VLS_275;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_276 = __VLS_asFunctionalComponent1(__VLS_275, new __VLS_275({}));
    const __VLS_277 = __VLS_276({}, ...__VLS_functionalComponentArgsRest(__VLS_276));
    const { default: __VLS_280 } = __VLS_278.slots;
    let __VLS_281;
    /** @ts-ignore @type { | typeof __VLS_components.Refresh} */
    Refresh;
    // @ts-ignore
    const __VLS_282 = __VLS_asFunctionalComponent1(__VLS_281, new __VLS_281({}));
    const __VLS_283 = __VLS_282({}, ...__VLS_functionalComponentArgsRest(__VLS_282));
    // @ts-ignore
    [loadTree,];
    var __VLS_278;
    // @ts-ignore
    [];
    var __VLS_270;
    var __VLS_271;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    if (__VLS_ctx.treeData.length) {
        let __VLS_286;
        /** @ts-ignore @type { | typeof __VLS_components.elTree | typeof __VLS_components.ElTree | typeof __VLS_components['el-tree'] | typeof __VLS_components.elTree | typeof __VLS_components.ElTree | typeof __VLS_components['el-tree']} */
        elTree;
        // @ts-ignore
        const __VLS_287 = __VLS_asFunctionalComponent1(__VLS_286, new __VLS_286({
            ...{ 'onNodeClick': {} },
            data: (__VLS_ctx.treeData),
            props: ({ label: 'name', children: 'children' }),
            highlightCurrent: true,
            defaultExpandAll: true,
            ...{ style: {} },
        }));
        const __VLS_288 = __VLS_287({
            ...{ 'onNodeClick': {} },
            data: (__VLS_ctx.treeData),
            props: ({ label: 'name', children: 'children' }),
            highlightCurrent: true,
            defaultExpandAll: true,
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_287));
        let __VLS_291;
        const __VLS_292 = ({ nodeClick: {} },
            { onNodeClick: (__VLS_ctx.onFileClick) });
        const { default: __VLS_293 } = __VLS_289.slots;
        {
            const { default: __VLS_294 } = __VLS_289.slots;
            const [{ data }] = __VLS_vSlot(__VLS_294);
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "tree-node" },
            });
            /** @type {__VLS_StyleScopedClasses['tree-node']} */ ;
            if (data.isDir) {
                let __VLS_295;
                /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
                elIcon;
                // @ts-ignore
                const __VLS_296 = __VLS_asFunctionalComponent1(__VLS_295, new __VLS_295({
                    ...{ style: {} },
                }));
                const __VLS_297 = __VLS_296({
                    ...{ style: {} },
                }, ...__VLS_functionalComponentArgsRest(__VLS_296));
                const { default: __VLS_300 } = __VLS_298.slots;
                let __VLS_301;
                /** @ts-ignore @type { | typeof __VLS_components.FolderOpened} */
                FolderOpened;
                // @ts-ignore
                const __VLS_302 = __VLS_asFunctionalComponent1(__VLS_301, new __VLS_301({}));
                const __VLS_303 = __VLS_302({}, ...__VLS_functionalComponentArgsRest(__VLS_302));
                // @ts-ignore
                [treeData, treeData, onFileClick,];
                var __VLS_298;
            }
            else {
                let __VLS_306;
                /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
                elIcon;
                // @ts-ignore
                const __VLS_307 = __VLS_asFunctionalComponent1(__VLS_306, new __VLS_306({
                    ...{ style: ({ color: __VLS_ctx.fileColor(data.name), fontSize: '13px', flexShrink: 0 }) },
                }));
                const __VLS_308 = __VLS_307({
                    ...{ style: ({ color: __VLS_ctx.fileColor(data.name), fontSize: '13px', flexShrink: 0 }) },
                }, ...__VLS_functionalComponentArgsRest(__VLS_307));
                const { default: __VLS_311 } = __VLS_309.slots;
                let __VLS_312;
                /** @ts-ignore @type { | typeof __VLS_components.Document} */
                Document;
                // @ts-ignore
                const __VLS_313 = __VLS_asFunctionalComponent1(__VLS_312, new __VLS_312({}));
                const __VLS_314 = __VLS_313({}, ...__VLS_functionalComponentArgsRest(__VLS_313));
                // @ts-ignore
                [fileColor,];
                var __VLS_309;
            }
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "tree-label" },
            });
            /** @type {__VLS_StyleScopedClasses['tree-label']} */ ;
            (data.name);
            if (!data.isDir) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "tree-size" },
                });
                /** @type {__VLS_StyleScopedClasses['tree-size']} */ ;
                (__VLS_ctx.fmtSize(data.size));
            }
            // @ts-ignore
            [fmtSize,];
        }
        // @ts-ignore
        [];
        var __VLS_289;
        var __VLS_290;
    }
    else {
        let __VLS_317;
        /** @ts-ignore @type { | typeof __VLS_components.elEmpty | typeof __VLS_components.ElEmpty | typeof __VLS_components['el-empty']} */
        elEmpty;
        // @ts-ignore
        const __VLS_318 = __VLS_asFunctionalComponent1(__VLS_317, new __VLS_317({
            description: "暂无文件",
            imageSize: (48),
        }));
        const __VLS_319 = __VLS_318({
            description: "暂无文件",
            imageSize: (48),
        }, ...__VLS_functionalComponentArgsRest(__VLS_318));
    }
    // @ts-ignore
    [];
    var __VLS_226;
    let __VLS_322;
    /** @ts-ignore @type { | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col'] | typeof __VLS_components.elCol | typeof __VLS_components.ElCol | typeof __VLS_components['el-col']} */
    elCol;
    // @ts-ignore
    const __VLS_323 = __VLS_asFunctionalComponent1(__VLS_322, new __VLS_322({
        xs: (24),
        sm: (18),
        md: (18),
        ...{ class: "editor-col" },
    }));
    const __VLS_324 = __VLS_323({
        xs: (24),
        sm: (18),
        md: (18),
        ...{ class: "editor-col" },
    }, ...__VLS_functionalComponentArgsRest(__VLS_323));
    /** @type {__VLS_StyleScopedClasses['editor-col']} */ ;
    const { default: __VLS_327 } = __VLS_325.slots;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "editor-panel" },
    });
    /** @type {__VLS_StyleScopedClasses['editor-panel']} */ ;
    if (__VLS_ctx.currentFile) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "editor-header" },
        });
        /** @type {__VLS_StyleScopedClasses['editor-header']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "editor-path" },
        });
        /** @type {__VLS_StyleScopedClasses['editor-path']} */ ;
        (__VLS_ctx.currentFile);
        let __VLS_328;
        /** @ts-ignore @type { | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag'] | typeof __VLS_components.elTag | typeof __VLS_components.ElTag | typeof __VLS_components['el-tag']} */
        elTag;
        // @ts-ignore
        const __VLS_329 = __VLS_asFunctionalComponent1(__VLS_328, new __VLS_328({
            size: "small",
            ...{ style: {} },
        }));
        const __VLS_330 = __VLS_329({
            size: "small",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_329));
        const { default: __VLS_333 } = __VLS_331.slots;
        (__VLS_ctx.fileExt(__VLS_ctx.currentFile));
        // @ts-ignore
        [currentFile, currentFile, currentFile, fileExt,];
        var __VLS_331;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div)({
            ...{ style: {} },
        });
        let __VLS_334;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_335 = __VLS_asFunctionalComponent1(__VLS_334, new __VLS_334({
            ...{ 'onClick': {} },
            text: true,
            size: "small",
            type: "danger",
            title: "删除",
        }));
        const __VLS_336 = __VLS_335({
            ...{ 'onClick': {} },
            text: true,
            size: "small",
            type: "danger",
            title: "删除",
        }, ...__VLS_functionalComponentArgsRest(__VLS_335));
        let __VLS_339;
        const __VLS_340 = ({ click: {} },
            { onClick: (__VLS_ctx.deleteCurrentFile) });
        const { default: __VLS_341 } = __VLS_337.slots;
        let __VLS_342;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_343 = __VLS_asFunctionalComponent1(__VLS_342, new __VLS_342({}));
        const __VLS_344 = __VLS_343({}, ...__VLS_functionalComponentArgsRest(__VLS_343));
        const { default: __VLS_347 } = __VLS_345.slots;
        let __VLS_348;
        /** @ts-ignore @type { | typeof __VLS_components.Delete} */
        Delete;
        // @ts-ignore
        const __VLS_349 = __VLS_asFunctionalComponent1(__VLS_348, new __VLS_348({}));
        const __VLS_350 = __VLS_349({}, ...__VLS_functionalComponentArgsRest(__VLS_349));
        // @ts-ignore
        [deleteCurrentFile,];
        var __VLS_345;
        // @ts-ignore
        [];
        var __VLS_337;
        var __VLS_338;
    }
    else {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "editor-header" },
        });
        /** @type {__VLS_StyleScopedClasses['editor-header']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ style: {} },
        });
    }
    if (__VLS_ctx.currentFile && !__VLS_ctx.isBinary) {
        let __VLS_353;
        /** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
        elInput;
        // @ts-ignore
        const __VLS_354 = __VLS_asFunctionalComponent1(__VLS_353, new __VLS_353({
            modelValue: (__VLS_ctx.fileContent),
            type: "textarea",
            placeholder: ('（空文件）'),
            autosize: (false),
            ...{ style: {} },
        }));
        const __VLS_355 = __VLS_354({
            modelValue: (__VLS_ctx.fileContent),
            type: "textarea",
            placeholder: ('（空文件）'),
            autosize: (false),
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_354));
    }
    else if (__VLS_ctx.currentFile && __VLS_ctx.isBinary) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "binary-hint" },
        });
        /** @type {__VLS_StyleScopedClasses['binary-hint']} */ ;
        let __VLS_358;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_359 = __VLS_asFunctionalComponent1(__VLS_358, new __VLS_358({
            size: "32",
        }));
        const __VLS_360 = __VLS_359({
            size: "32",
        }, ...__VLS_functionalComponentArgsRest(__VLS_359));
        const { default: __VLS_363 } = __VLS_361.slots;
        let __VLS_364;
        /** @ts-ignore @type { | typeof __VLS_components.Document} */
        Document;
        // @ts-ignore
        const __VLS_365 = __VLS_asFunctionalComponent1(__VLS_364, new __VLS_364({}));
        const __VLS_366 = __VLS_365({}, ...__VLS_functionalComponentArgsRest(__VLS_365));
        // @ts-ignore
        [currentFile, currentFile, isBinary, isBinary, fileContent,];
        var __VLS_361;
        __VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({});
    }
    else {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "editor-empty" },
        });
        /** @type {__VLS_StyleScopedClasses['editor-empty']} */ ;
        let __VLS_369;
        /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
        elIcon;
        // @ts-ignore
        const __VLS_370 = __VLS_asFunctionalComponent1(__VLS_369, new __VLS_369({
            size: "48",
            ...{ style: {} },
        }));
        const __VLS_371 = __VLS_370({
            size: "48",
            ...{ style: {} },
        }, ...__VLS_functionalComponentArgsRest(__VLS_370));
        const { default: __VLS_374 } = __VLS_372.slots;
        let __VLS_375;
        /** @ts-ignore @type { | typeof __VLS_components.EditPen} */
        EditPen;
        // @ts-ignore
        const __VLS_376 = __VLS_asFunctionalComponent1(__VLS_375, new __VLS_375({}));
        const __VLS_377 = __VLS_376({}, ...__VLS_functionalComponentArgsRest(__VLS_376));
        // @ts-ignore
        [];
        var __VLS_372;
        __VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({});
    }
    if (__VLS_ctx.currentFile && !__VLS_ctx.isBinary) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "editor-footer" },
        });
        /** @type {__VLS_StyleScopedClasses['editor-footer']} */ ;
        let __VLS_380;
        /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
        elButton;
        // @ts-ignore
        const __VLS_381 = __VLS_asFunctionalComponent1(__VLS_380, new __VLS_380({
            ...{ 'onClick': {} },
            type: "primary",
            size: "small",
        }));
        const __VLS_382 = __VLS_381({
            ...{ 'onClick': {} },
            type: "primary",
            size: "small",
        }, ...__VLS_functionalComponentArgsRest(__VLS_381));
        let __VLS_385;
        const __VLS_386 = ({ click: {} },
            { onClick: (__VLS_ctx.saveFile) });
        const { default: __VLS_387 } = __VLS_383.slots;
        // @ts-ignore
        [currentFile, isBinary, saveFile,];
        var __VLS_383;
        var __VLS_384;
        if (__VLS_ctx.fileInfo) {
            let __VLS_388;
            /** @ts-ignore @type { | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text'] | typeof __VLS_components.elText | typeof __VLS_components.ElText | typeof __VLS_components['el-text']} */
            elText;
            // @ts-ignore
            const __VLS_389 = __VLS_asFunctionalComponent1(__VLS_388, new __VLS_388({
                type: "info",
                size: "small",
            }));
            const __VLS_390 = __VLS_389({
                type: "info",
                size: "small",
            }, ...__VLS_functionalComponentArgsRest(__VLS_389));
            const { default: __VLS_393 } = __VLS_391.slots;
            (__VLS_ctx.fmtSize(__VLS_ctx.fileInfo.size));
            (__VLS_ctx.fmtTime(__VLS_ctx.fileInfo.modTime));
            // @ts-ignore
            [fmtSize, fileInfo, fileInfo, fileInfo, fmtTime,];
            var __VLS_391;
        }
    }
    // @ts-ignore
    [];
    var __VLS_325;
    // @ts-ignore
    [];
    var __VLS_220;
}
else {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "projects-main projects-empty" },
    });
    /** @type {__VLS_StyleScopedClasses['projects-main']} */ ;
    /** @type {__VLS_StyleScopedClasses['projects-empty']} */ ;
    let __VLS_394;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_395 = __VLS_asFunctionalComponent1(__VLS_394, new __VLS_394({
        size: "64",
        ...{ style: {} },
    }));
    const __VLS_396 = __VLS_395({
        size: "64",
        ...{ style: {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_395));
    const { default: __VLS_399 } = __VLS_397.slots;
    let __VLS_400;
    /** @ts-ignore @type { | typeof __VLS_components.FolderOpened} */
    FolderOpened;
    // @ts-ignore
    const __VLS_401 = __VLS_asFunctionalComponent1(__VLS_400, new __VLS_400({}));
    const __VLS_402 = __VLS_401({}, ...__VLS_functionalComponentArgsRest(__VLS_401));
    // @ts-ignore
    [];
    var __VLS_397;
    __VLS_asFunctionalElement1(__VLS_intrinsics.p, __VLS_intrinsics.p)({
        ...{ style: {} },
    });
    let __VLS_405;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_406 = __VLS_asFunctionalComponent1(__VLS_405, new __VLS_405({
        ...{ 'onClick': {} },
        type: "primary",
    }));
    const __VLS_407 = __VLS_406({
        ...{ 'onClick': {} },
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_406));
    let __VLS_410;
    const __VLS_411 = ({ click: {} },
        { onClick: (...[$event]) => {
                if (!!(__VLS_ctx.currentProject))
                    return;
                __VLS_ctx.showCreate = true;
                // @ts-ignore
                [showCreate,];
            } });
    const { default: __VLS_412 } = __VLS_408.slots;
    let __VLS_413;
    /** @ts-ignore @type { | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon'] | typeof __VLS_components.elIcon | typeof __VLS_components.ElIcon | typeof __VLS_components['el-icon']} */
    elIcon;
    // @ts-ignore
    const __VLS_414 = __VLS_asFunctionalComponent1(__VLS_413, new __VLS_413({}));
    const __VLS_415 = __VLS_414({}, ...__VLS_functionalComponentArgsRest(__VLS_414));
    const { default: __VLS_418 } = __VLS_416.slots;
    let __VLS_419;
    /** @ts-ignore @type { | typeof __VLS_components.Plus} */
    Plus;
    // @ts-ignore
    const __VLS_420 = __VLS_asFunctionalComponent1(__VLS_419, new __VLS_419({}));
    const __VLS_421 = __VLS_420({}, ...__VLS_functionalComponentArgsRest(__VLS_420));
    // @ts-ignore
    [];
    var __VLS_416;
    // @ts-ignore
    [];
    var __VLS_408;
    var __VLS_409;
}
let __VLS_424;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_425 = __VLS_asFunctionalComponent1(__VLS_424, new __VLS_424({
    ...{ 'onClose': {} },
    modelValue: (__VLS_ctx.showCreate),
    title: "新建项目",
    width: "440px",
}));
const __VLS_426 = __VLS_425({
    ...{ 'onClose': {} },
    modelValue: (__VLS_ctx.showCreate),
    title: "新建项目",
    width: "440px",
}, ...__VLS_functionalComponentArgsRest(__VLS_425));
let __VLS_429;
const __VLS_430 = ({ close: {} },
    { onClose: (__VLS_ctx.resetForm) });
const { default: __VLS_431 } = __VLS_427.slots;
let __VLS_432;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_433 = __VLS_asFunctionalComponent1(__VLS_432, new __VLS_432({
    model: (__VLS_ctx.createForm),
    labelWidth: "80px",
}));
const __VLS_434 = __VLS_433({
    model: (__VLS_ctx.createForm),
    labelWidth: "80px",
}, ...__VLS_functionalComponentArgsRest(__VLS_433));
const { default: __VLS_437 } = __VLS_435.slots;
let __VLS_438;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_439 = __VLS_asFunctionalComponent1(__VLS_438, new __VLS_438({
    label: "项目 ID",
    required: true,
}));
const __VLS_440 = __VLS_439({
    label: "项目 ID",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_439));
const { default: __VLS_443 } = __VLS_441.slots;
let __VLS_444;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_445 = __VLS_asFunctionalComponent1(__VLS_444, new __VLS_444({
    modelValue: (__VLS_ctx.createForm.id),
    placeholder: "如 ai-panel（小写字母/数字/连字符）",
}));
const __VLS_446 = __VLS_445({
    modelValue: (__VLS_ctx.createForm.id),
    placeholder: "如 ai-panel（小写字母/数字/连字符）",
}, ...__VLS_functionalComponentArgsRest(__VLS_445));
// @ts-ignore
[showCreate, resetForm, createForm, createForm,];
var __VLS_441;
let __VLS_449;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_450 = __VLS_asFunctionalComponent1(__VLS_449, new __VLS_449({
    label: "名称",
    required: true,
}));
const __VLS_451 = __VLS_450({
    label: "名称",
    required: true,
}, ...__VLS_functionalComponentArgsRest(__VLS_450));
const { default: __VLS_454 } = __VLS_452.slots;
let __VLS_455;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_456 = __VLS_asFunctionalComponent1(__VLS_455, new __VLS_455({
    modelValue: (__VLS_ctx.createForm.name),
    placeholder: "项目名称",
}));
const __VLS_457 = __VLS_456({
    modelValue: (__VLS_ctx.createForm.name),
    placeholder: "项目名称",
}, ...__VLS_functionalComponentArgsRest(__VLS_456));
// @ts-ignore
[createForm,];
var __VLS_452;
let __VLS_460;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_461 = __VLS_asFunctionalComponent1(__VLS_460, new __VLS_460({
    label: "描述",
}));
const __VLS_462 = __VLS_461({
    label: "描述",
}, ...__VLS_functionalComponentArgsRest(__VLS_461));
const { default: __VLS_465 } = __VLS_463.slots;
let __VLS_466;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_467 = __VLS_asFunctionalComponent1(__VLS_466, new __VLS_466({
    modelValue: (__VLS_ctx.createForm.description),
    placeholder: "简短描述（可选）",
}));
const __VLS_468 = __VLS_467({
    modelValue: (__VLS_ctx.createForm.description),
    placeholder: "简短描述（可选）",
}, ...__VLS_functionalComponentArgsRest(__VLS_467));
// @ts-ignore
[createForm,];
var __VLS_463;
let __VLS_471;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_472 = __VLS_asFunctionalComponent1(__VLS_471, new __VLS_471({
    label: "标签",
}));
const __VLS_473 = __VLS_472({
    label: "标签",
}, ...__VLS_functionalComponentArgsRest(__VLS_472));
const { default: __VLS_476 } = __VLS_474.slots;
let __VLS_477;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_478 = __VLS_asFunctionalComponent1(__VLS_477, new __VLS_477({
    modelValue: (__VLS_ctx.createForm.tagsStr),
    placeholder: "多个标签用逗号分隔，如 go,vue",
}));
const __VLS_479 = __VLS_478({
    modelValue: (__VLS_ctx.createForm.tagsStr),
    placeholder: "多个标签用逗号分隔，如 go,vue",
}, ...__VLS_functionalComponentArgsRest(__VLS_478));
// @ts-ignore
[createForm,];
var __VLS_474;
// @ts-ignore
[];
var __VLS_435;
{
    const { footer: __VLS_482 } = __VLS_427.slots;
    let __VLS_483;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_484 = __VLS_asFunctionalComponent1(__VLS_483, new __VLS_483({
        ...{ 'onClick': {} },
    }));
    const __VLS_485 = __VLS_484({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_484));
    let __VLS_488;
    const __VLS_489 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.showCreate = false;
                // @ts-ignore
                [showCreate,];
            } });
    const { default: __VLS_490 } = __VLS_486.slots;
    // @ts-ignore
    [];
    var __VLS_486;
    var __VLS_487;
    let __VLS_491;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_492 = __VLS_asFunctionalComponent1(__VLS_491, new __VLS_491({
        ...{ 'onClick': {} },
        type: "primary",
    }));
    const __VLS_493 = __VLS_492({
        ...{ 'onClick': {} },
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_492));
    let __VLS_496;
    const __VLS_497 = ({ click: {} },
        { onClick: (__VLS_ctx.doCreate) });
    const { default: __VLS_498 } = __VLS_494.slots;
    // @ts-ignore
    [doCreate,];
    var __VLS_494;
    var __VLS_495;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_427;
var __VLS_428;
let __VLS_499;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_500 = __VLS_asFunctionalComponent1(__VLS_499, new __VLS_499({
    modelValue: (__VLS_ctx.showEdit),
    title: "编辑项目信息",
    width: "440px",
}));
const __VLS_501 = __VLS_500({
    modelValue: (__VLS_ctx.showEdit),
    title: "编辑项目信息",
    width: "440px",
}, ...__VLS_functionalComponentArgsRest(__VLS_500));
const { default: __VLS_504 } = __VLS_502.slots;
let __VLS_505;
/** @ts-ignore @type { | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form'] | typeof __VLS_components.elForm | typeof __VLS_components.ElForm | typeof __VLS_components['el-form']} */
elForm;
// @ts-ignore
const __VLS_506 = __VLS_asFunctionalComponent1(__VLS_505, new __VLS_505({
    model: (__VLS_ctx.editForm),
    labelWidth: "80px",
}));
const __VLS_507 = __VLS_506({
    model: (__VLS_ctx.editForm),
    labelWidth: "80px",
}, ...__VLS_functionalComponentArgsRest(__VLS_506));
const { default: __VLS_510 } = __VLS_508.slots;
let __VLS_511;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_512 = __VLS_asFunctionalComponent1(__VLS_511, new __VLS_511({
    label: "名称",
}));
const __VLS_513 = __VLS_512({
    label: "名称",
}, ...__VLS_functionalComponentArgsRest(__VLS_512));
const { default: __VLS_516 } = __VLS_514.slots;
let __VLS_517;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_518 = __VLS_asFunctionalComponent1(__VLS_517, new __VLS_517({
    modelValue: (__VLS_ctx.editForm.name),
}));
const __VLS_519 = __VLS_518({
    modelValue: (__VLS_ctx.editForm.name),
}, ...__VLS_functionalComponentArgsRest(__VLS_518));
// @ts-ignore
[showEdit, editForm, editForm,];
var __VLS_514;
let __VLS_522;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_523 = __VLS_asFunctionalComponent1(__VLS_522, new __VLS_522({
    label: "描述",
}));
const __VLS_524 = __VLS_523({
    label: "描述",
}, ...__VLS_functionalComponentArgsRest(__VLS_523));
const { default: __VLS_527 } = __VLS_525.slots;
let __VLS_528;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_529 = __VLS_asFunctionalComponent1(__VLS_528, new __VLS_528({
    modelValue: (__VLS_ctx.editForm.description),
}));
const __VLS_530 = __VLS_529({
    modelValue: (__VLS_ctx.editForm.description),
}, ...__VLS_functionalComponentArgsRest(__VLS_529));
// @ts-ignore
[editForm,];
var __VLS_525;
let __VLS_533;
/** @ts-ignore @type { | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item'] | typeof __VLS_components.elFormItem | typeof __VLS_components.ElFormItem | typeof __VLS_components['el-form-item']} */
elFormItem;
// @ts-ignore
const __VLS_534 = __VLS_asFunctionalComponent1(__VLS_533, new __VLS_533({
    label: "标签",
}));
const __VLS_535 = __VLS_534({
    label: "标签",
}, ...__VLS_functionalComponentArgsRest(__VLS_534));
const { default: __VLS_538 } = __VLS_536.slots;
let __VLS_539;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_540 = __VLS_asFunctionalComponent1(__VLS_539, new __VLS_539({
    modelValue: (__VLS_ctx.editForm.tagsStr),
    placeholder: "多个标签用逗号分隔",
}));
const __VLS_541 = __VLS_540({
    modelValue: (__VLS_ctx.editForm.tagsStr),
    placeholder: "多个标签用逗号分隔",
}, ...__VLS_functionalComponentArgsRest(__VLS_540));
// @ts-ignore
[editForm,];
var __VLS_536;
// @ts-ignore
[];
var __VLS_508;
{
    const { footer: __VLS_544 } = __VLS_502.slots;
    let __VLS_545;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_546 = __VLS_asFunctionalComponent1(__VLS_545, new __VLS_545({
        ...{ 'onClick': {} },
    }));
    const __VLS_547 = __VLS_546({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_546));
    let __VLS_550;
    const __VLS_551 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.showEdit = false;
                // @ts-ignore
                [showEdit,];
            } });
    const { default: __VLS_552 } = __VLS_548.slots;
    // @ts-ignore
    [];
    var __VLS_548;
    var __VLS_549;
    let __VLS_553;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_554 = __VLS_asFunctionalComponent1(__VLS_553, new __VLS_553({
        ...{ 'onClick': {} },
        type: "primary",
    }));
    const __VLS_555 = __VLS_554({
        ...{ 'onClick': {} },
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_554));
    let __VLS_558;
    const __VLS_559 = ({ click: {} },
        { onClick: (__VLS_ctx.doEdit) });
    const { default: __VLS_560 } = __VLS_556.slots;
    // @ts-ignore
    [doEdit,];
    var __VLS_556;
    var __VLS_557;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_502;
let __VLS_561;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_562 = __VLS_asFunctionalComponent1(__VLS_561, new __VLS_561({
    modelValue: (__VLS_ctx.showNewFile),
    title: "新建文件",
    width: "380px",
}));
const __VLS_563 = __VLS_562({
    modelValue: (__VLS_ctx.showNewFile),
    title: "新建文件",
    width: "380px",
}, ...__VLS_functionalComponentArgsRest(__VLS_562));
const { default: __VLS_566 } = __VLS_564.slots;
let __VLS_567;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_568 = __VLS_asFunctionalComponent1(__VLS_567, new __VLS_567({
    ...{ 'onKeyup': {} },
    modelValue: (__VLS_ctx.newFilePath),
    placeholder: "如 README.md 或 src/main.go",
}));
const __VLS_569 = __VLS_568({
    ...{ 'onKeyup': {} },
    modelValue: (__VLS_ctx.newFilePath),
    placeholder: "如 README.md 或 src/main.go",
}, ...__VLS_functionalComponentArgsRest(__VLS_568));
let __VLS_572;
const __VLS_573 = ({ keyup: {} },
    { onKeyup: (__VLS_ctx.doNewFile) });
var __VLS_570;
var __VLS_571;
{
    const { footer: __VLS_574 } = __VLS_564.slots;
    let __VLS_575;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_576 = __VLS_asFunctionalComponent1(__VLS_575, new __VLS_575({
        ...{ 'onClick': {} },
    }));
    const __VLS_577 = __VLS_576({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_576));
    let __VLS_580;
    const __VLS_581 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.showNewFile = false;
                // @ts-ignore
                [showNewFile, showNewFile, newFilePath, doNewFile,];
            } });
    const { default: __VLS_582 } = __VLS_578.slots;
    // @ts-ignore
    [];
    var __VLS_578;
    var __VLS_579;
    let __VLS_583;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_584 = __VLS_asFunctionalComponent1(__VLS_583, new __VLS_583({
        ...{ 'onClick': {} },
        type: "primary",
    }));
    const __VLS_585 = __VLS_584({
        ...{ 'onClick': {} },
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_584));
    let __VLS_588;
    const __VLS_589 = ({ click: {} },
        { onClick: (__VLS_ctx.doNewFile) });
    const { default: __VLS_590 } = __VLS_586.slots;
    // @ts-ignore
    [doNewFile,];
    var __VLS_586;
    var __VLS_587;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_564;
let __VLS_591;
/** @ts-ignore @type { | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog'] | typeof __VLS_components.elDialog | typeof __VLS_components.ElDialog | typeof __VLS_components['el-dialog']} */
elDialog;
// @ts-ignore
const __VLS_592 = __VLS_asFunctionalComponent1(__VLS_591, new __VLS_591({
    modelValue: (__VLS_ctx.showNewFolder),
    title: "新建文件夹",
    width: "380px",
}));
const __VLS_593 = __VLS_592({
    modelValue: (__VLS_ctx.showNewFolder),
    title: "新建文件夹",
    width: "380px",
}, ...__VLS_functionalComponentArgsRest(__VLS_592));
const { default: __VLS_596 } = __VLS_594.slots;
let __VLS_597;
/** @ts-ignore @type { | typeof __VLS_components.elInput | typeof __VLS_components.ElInput | typeof __VLS_components['el-input']} */
elInput;
// @ts-ignore
const __VLS_598 = __VLS_asFunctionalComponent1(__VLS_597, new __VLS_597({
    ...{ 'onKeyup': {} },
    modelValue: (__VLS_ctx.newFolderPath),
    placeholder: "如 src 或 docs/api",
}));
const __VLS_599 = __VLS_598({
    ...{ 'onKeyup': {} },
    modelValue: (__VLS_ctx.newFolderPath),
    placeholder: "如 src 或 docs/api",
}, ...__VLS_functionalComponentArgsRest(__VLS_598));
let __VLS_602;
const __VLS_603 = ({ keyup: {} },
    { onKeyup: (__VLS_ctx.doNewFolder) });
var __VLS_600;
var __VLS_601;
{
    const { footer: __VLS_604 } = __VLS_594.slots;
    let __VLS_605;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_606 = __VLS_asFunctionalComponent1(__VLS_605, new __VLS_605({
        ...{ 'onClick': {} },
    }));
    const __VLS_607 = __VLS_606({
        ...{ 'onClick': {} },
    }, ...__VLS_functionalComponentArgsRest(__VLS_606));
    let __VLS_610;
    const __VLS_611 = ({ click: {} },
        { onClick: (...[$event]) => {
                __VLS_ctx.showNewFolder = false;
                // @ts-ignore
                [showNewFolder, showNewFolder, newFolderPath, doNewFolder,];
            } });
    const { default: __VLS_612 } = __VLS_608.slots;
    // @ts-ignore
    [];
    var __VLS_608;
    var __VLS_609;
    let __VLS_613;
    /** @ts-ignore @type { | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button'] | typeof __VLS_components.elButton | typeof __VLS_components.ElButton | typeof __VLS_components['el-button']} */
    elButton;
    // @ts-ignore
    const __VLS_614 = __VLS_asFunctionalComponent1(__VLS_613, new __VLS_613({
        ...{ 'onClick': {} },
        type: "primary",
    }));
    const __VLS_615 = __VLS_614({
        ...{ 'onClick': {} },
        type: "primary",
    }, ...__VLS_functionalComponentArgsRest(__VLS_614));
    let __VLS_618;
    const __VLS_619 = ({ click: {} },
        { onClick: (__VLS_ctx.doNewFolder) });
    const { default: __VLS_620 } = __VLS_616.slots;
    // @ts-ignore
    [doNewFolder,];
    var __VLS_616;
    var __VLS_617;
    // @ts-ignore
    [];
}
// @ts-ignore
[];
var __VLS_594;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({});
export default {};
