/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/template-helpers.d.ts" />
/// <reference types="../../../../home/ubuntu/.npm/_npx/2db181330ea4b15b/node_modules/@vue/language-core/types/props-fallback.d.ts" />
import { ref, computed } from 'vue';
const props = defineProps();
const dispatchers = ref(new Map());
const collapsed = ref(false);
const reportDialogVisible = ref(false);
const reportDialogAgent = ref('');
const reportDialogRecords = ref([]);
const artifactDialogVisible = ref(false);
const artifactDialogName = ref('');
const artifactDialogPath = ref('');
const artifactDialogContent = ref('');
const artifactLoading = ref(false);
const hasAny = computed(() => dispatchers.value.size > 0);
const activeList = computed(() => [...dispatchers.value.values()].filter(d => d.status !== 'done' && d.status !== 'error'));
const sortedDispatchers = computed(() => [...dispatchers.value.values()].sort((a, b) => a.spawnedAt - b.spawnedAt));
// ── Event handler (called by AiChat.vue) ─────────────────────────────────────
function handleEvent(raw) {
    const data = typeof raw === 'string' ? JSON.parse(raw) : raw;
    if (!data)
        return;
    const id = data.subagentSessionId;
    if (!id)
        return;
    if (data.type === 'subagent_spawn' || data.type === 'spawn') {
        const entry = {
            subagentSessionId: id,
            agentId: data.agentId || '',
            agentName: data.agentName || data.agentId || '未知成员',
            avatarColor: data.avatarColor || '#6366f1',
            status: 'running',
            progress: 0,
            reports: [],
            latestReport: '',
            reportNew: false,
            spawnedAt: data.timestamp || Date.now(),
            priority: data.priority || '',
            deliverable: data.deliverable || '',
            attachmentCount: data.attachmentCount || 0,
            hasContext: !!data.hasContext,
            sharedProjectId: data.sharedProjectId || '',
            artifacts: [],
        };
        dispatchers.value = new Map(dispatchers.value.set(id, entry));
    }
    else if (data.type === 'subagent_report' || data.type === 'report') {
        const d = dispatchers.value.get(id);
        if (d) {
            const rpt = {
                content: data.content || '',
                status: data.status || 'running',
                progress: data.progress || 0,
                timestamp: data.timestamp || Date.now(),
            };
            d.reports.push(rpt);
            d.latestReport = data.content || '';
            if (data.progress)
                d.progress = data.progress;
            if (data.status === 'done')
                d.status = 'done';
            else if (data.status === 'blocked')
                d.status = 'blocked';
            else
                d.status = 'running';
            d.reportNew = true;
            setTimeout(() => { if (d)
                d.reportNew = false; }, 900);
            dispatchers.value = new Map(dispatchers.value);
        }
    }
    else if (data.type === 'subagent_done' || data.type === 'done') {
        const d = dispatchers.value.get(id);
        if (d) {
            d.status = 'done';
            d.doneAt = data.timestamp || Date.now();
            dispatchers.value = new Map(dispatchers.value);
            // Auto-remove after 3s
            setTimeout(() => {
                dispatchers.value.delete(id);
                dispatchers.value = new Map(dispatchers.value);
            }, 3000);
        }
    }
    else if (data.type === 'subagent_error' || data.type === 'error') {
        const d = dispatchers.value.get(id);
        if (d) {
            d.status = 'error';
            dispatchers.value = new Map(dispatchers.value);
        }
    }
    else if (data.type === 'subagent_artifacts' || data.type === 'artifacts') {
        const d = dispatchers.value.get(id);
        if (d && Array.isArray(data.artifacts)) {
            d.artifacts = data.artifacts;
            dispatchers.value = new Map(dispatchers.value);
        }
    }
}
const __VLS_exposed = { handleEvent };
defineExpose(__VLS_exposed);
// ── Helpers ──────────────────────────────────────────────────────────────────
function statusLabel(s) {
    return { running: '执行中', blocked: '遇到阻碍', done: '已完成', error: '出错' }[s] ?? s;
}
function truncate(s, n) {
    return s.length > n ? s.slice(0, n) + '…' : s;
}
function formatTime(ts) {
    return new Date(ts).toLocaleTimeString('zh-CN');
}
function formatSize(bytes) {
    if (bytes < 1024)
        return bytes + 'B';
    if (bytes < 1024 * 1024)
        return (bytes / 1024).toFixed(1) + 'KB';
    return (bytes / 1024 / 1024).toFixed(1) + 'MB';
}
function artifactIcon(type) {
    return { code: '📄', report: '📊', data: '🗂', file: '📎' }[type] ?? '📎';
}
function viewReports(d) {
    reportDialogAgent.value = d.agentName;
    reportDialogRecords.value = [...d.reports].reverse();
    reportDialogVisible.value = true;
}
async function viewArtifact(art) {
    artifactDialogName.value = art.name;
    artifactDialogPath.value = art.path;
    artifactDialogContent.value = '';
    artifactLoading.value = true;
    artifactDialogVisible.value = true;
    try {
        // Fetch file content from shared project API
        const token = localStorage.getItem('zyhive_token') || '';
        const res = await fetch(`/api/projects/${art.projectId}/files/${encodeURIComponent(art.path)}`, {
            headers: { Authorization: `Bearer ${token}` }
        });
        if (res.ok) {
            const data = await res.json();
            artifactDialogContent.value = data.content ?? '';
        }
    }
    catch {
        artifactDialogContent.value = '';
    }
    finally {
        artifactLoading.value = false;
    }
}
const __VLS_ctx = {
    ...{},
    ...{},
    ...{},
    ...{},
};
let __VLS_components;
let __VLS_intrinsics;
let __VLS_directives;
/** @type {__VLS_StyleScopedClasses['dp-collapse-btn']} */ ;
/** @type {__VLS_StyleScopedClasses['dp-done-badge']} */ ;
/** @type {__VLS_StyleScopedClasses['dp-error-badge']} */ ;
/** @type {__VLS_StyleScopedClasses['dp-artifact-chip']} */ ;
let __VLS_0;
/** @ts-ignore @type { | typeof __VLS_components.Transition | typeof __VLS_components.Transition} */
Transition;
// @ts-ignore
const __VLS_1 = __VLS_asFunctionalComponent1(__VLS_0, new __VLS_0({
    name: "panel-slide",
}));
const __VLS_2 = __VLS_1({
    name: "panel-slide",
}, ...__VLS_functionalComponentArgsRest(__VLS_1));
const { default: __VLS_5 } = __VLS_3.slots;
if (__VLS_ctx.hasAny) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "dispatch-panel" },
    });
    /** @type {__VLS_StyleScopedClasses['dispatch-panel']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "dp-header" },
    });
    /** @type {__VLS_StyleScopedClasses['dp-header']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span)({
        ...{ class: "dp-pulse" },
    });
    /** @type {__VLS_StyleScopedClasses['dp-pulse']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "dp-title" },
    });
    /** @type {__VLS_StyleScopedClasses['dp-title']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "dp-count" },
    });
    /** @type {__VLS_StyleScopedClasses['dp-count']} */ ;
    (__VLS_ctx.activeList.length);
    __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.hasAny))
                    return;
                __VLS_ctx.collapsed = !__VLS_ctx.collapsed;
                // @ts-ignore
                [hasAny, activeList, collapsed, collapsed,];
            } },
        ...{ class: "dp-collapse-btn" },
    });
    /** @type {__VLS_StyleScopedClasses['dp-collapse-btn']} */ ;
    (__VLS_ctx.collapsed ? '展开 ∨' : '收起 ∧');
    let __VLS_6;
    /** @ts-ignore @type { | typeof __VLS_components.Transition | typeof __VLS_components.Transition} */
    Transition;
    // @ts-ignore
    const __VLS_7 = __VLS_asFunctionalComponent1(__VLS_6, new __VLS_6({
        name: "dp-expand",
    }));
    const __VLS_8 = __VLS_7({
        name: "dp-expand",
    }, ...__VLS_functionalComponentArgsRest(__VLS_7));
    const { default: __VLS_11 } = __VLS_9.slots;
    if (!__VLS_ctx.collapsed) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "dp-body" },
        });
        /** @type {__VLS_StyleScopedClasses['dp-body']} */ ;
        let __VLS_12;
        /** @ts-ignore @type { | typeof __VLS_components.TransitionGroup | typeof __VLS_components.TransitionGroup} */
        TransitionGroup;
        // @ts-ignore
        const __VLS_13 = __VLS_asFunctionalComponent1(__VLS_12, new __VLS_12({
            name: "member-fly",
            tag: "div",
            ...{ class: "dp-members" },
        }));
        const __VLS_14 = __VLS_13({
            name: "member-fly",
            tag: "div",
            ...{ class: "dp-members" },
        }, ...__VLS_functionalComponentArgsRest(__VLS_13));
        /** @type {__VLS_StyleScopedClasses['dp-members']} */ ;
        const { default: __VLS_17 } = __VLS_15.slots;
        for (const [d, idx] of __VLS_vFor((__VLS_ctx.sortedDispatchers))) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                key: (d.subagentSessionId),
                ...{ class: "dp-member" },
                ...{ style: ({ transitionDelay: idx * 80 + 'ms' }) },
            });
            /** @type {__VLS_StyleScopedClasses['dp-member']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "dp-avatar" },
                ...{ class: ('status-' + d.status) },
                ...{ style: ({ background: d.avatarColor || '#6366f1' }) },
            });
            /** @type {__VLS_StyleScopedClasses['dp-avatar']} */ ;
            ((d.agentName || '?')[0]);
            if (d.status === 'done') {
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "dp-done-badge" },
                });
                /** @type {__VLS_StyleScopedClasses['dp-done-badge']} */ ;
            }
            if (d.status === 'error') {
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "dp-error-badge" },
                });
                /** @type {__VLS_StyleScopedClasses['dp-error-badge']} */ ;
            }
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "dp-info" },
            });
            /** @type {__VLS_StyleScopedClasses['dp-info']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                ...{ class: "dp-name-row" },
            });
            /** @type {__VLS_StyleScopedClasses['dp-name-row']} */ ;
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "dp-name" },
            });
            /** @type {__VLS_StyleScopedClasses['dp-name']} */ ;
            (d.agentName);
            if (d.priority === 'high') {
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "dp-priority-badge" },
                });
                /** @type {__VLS_StyleScopedClasses['dp-priority-badge']} */ ;
            }
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "dp-tag" },
                ...{ class: ('tag-' + d.status) },
            });
            /** @type {__VLS_StyleScopedClasses['dp-tag']} */ ;
            (__VLS_ctx.statusLabel(d.status));
            if (d.progress > 0 || d.status === 'running') {
                __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                    ...{ class: "dp-progress-wrap" },
                });
                /** @type {__VLS_StyleScopedClasses['dp-progress-wrap']} */ ;
                __VLS_asFunctionalElement1(__VLS_intrinsics.div)({
                    ...{ class: "dp-progress-bar" },
                    ...{ style: ({ width: d.progress + '%' }) },
                });
                /** @type {__VLS_StyleScopedClasses['dp-progress-bar']} */ ;
                if (d.progress > 0) {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "dp-progress-num" },
                    });
                    /** @type {__VLS_StyleScopedClasses['dp-progress-num']} */ ;
                    (d.progress);
                }
            }
            if (d.deliverable || (d.attachmentCount ?? 0) > 0 || d.hasContext) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                    ...{ class: "dp-meta-row" },
                });
                /** @type {__VLS_StyleScopedClasses['dp-meta-row']} */ ;
                if ((d.attachmentCount ?? 0) > 0) {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "dp-meta-chip" },
                    });
                    /** @type {__VLS_StyleScopedClasses['dp-meta-chip']} */ ;
                    (d.attachmentCount);
                }
                if (d.hasContext) {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "dp-meta-chip" },
                    });
                    /** @type {__VLS_StyleScopedClasses['dp-meta-chip']} */ ;
                }
                if (d.deliverable) {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "dp-meta-chip dp-deliverable" },
                        title: (d.deliverable),
                    });
                    /** @type {__VLS_StyleScopedClasses['dp-meta-chip']} */ ;
                    /** @type {__VLS_StyleScopedClasses['dp-deliverable']} */ ;
                    (__VLS_ctx.truncate(d.deliverable, 24));
                }
            }
            if (d.latestReport) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                    ...{ class: "dp-report" },
                    ...{ class: ({ 'dp-report-new': d.reportNew }) },
                });
                /** @type {__VLS_StyleScopedClasses['dp-report']} */ ;
                /** @type {__VLS_StyleScopedClasses['dp-report-new']} */ ;
                (__VLS_ctx.truncate(d.latestReport, 60));
                if (d.reports.length > 1) {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
                        ...{ onClick: (...[$event]) => {
                                if (!(__VLS_ctx.hasAny))
                                    return;
                                if (!(!__VLS_ctx.collapsed))
                                    return;
                                if (!(d.latestReport))
                                    return;
                                if (!(d.reports.length > 1))
                                    return;
                                __VLS_ctx.viewReports(d);
                                // @ts-ignore
                                [collapsed, collapsed, sortedDispatchers, statusLabel, truncate, truncate, viewReports,];
                            } },
                        ...{ class: "dp-view-all" },
                    });
                    /** @type {__VLS_StyleScopedClasses['dp-view-all']} */ ;
                    (d.reports.length);
                }
            }
            if (d.artifacts && d.artifacts.length > 0) {
                __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
                    ...{ class: "dp-artifacts" },
                });
                /** @type {__VLS_StyleScopedClasses['dp-artifacts']} */ ;
                __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                    ...{ class: "dp-artifacts-label" },
                });
                /** @type {__VLS_StyleScopedClasses['dp-artifacts-label']} */ ;
                for (const [art] of __VLS_vFor((d.artifacts))) {
                    __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
                        ...{ onClick: (...[$event]) => {
                                if (!(__VLS_ctx.hasAny))
                                    return;
                                if (!(!__VLS_ctx.collapsed))
                                    return;
                                if (!(d.artifacts && d.artifacts.length > 0))
                                    return;
                                __VLS_ctx.viewArtifact(art);
                                // @ts-ignore
                                [viewArtifact,];
                            } },
                        key: (art.path),
                        ...{ class: "dp-artifact-chip" },
                        title: (art.path),
                    });
                    /** @type {__VLS_StyleScopedClasses['dp-artifact-chip']} */ ;
                    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                        ...{ class: "dp-art-icon" },
                    });
                    /** @type {__VLS_StyleScopedClasses['dp-art-icon']} */ ;
                    (__VLS_ctx.artifactIcon(art.type));
                    (art.name);
                    if (art.size) {
                        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                            ...{ class: "dp-art-size" },
                        });
                        /** @type {__VLS_StyleScopedClasses['dp-art-size']} */ ;
                        (__VLS_ctx.formatSize(art.size));
                    }
                    // @ts-ignore
                    [artifactIcon, formatSize,];
                }
            }
            // @ts-ignore
            [];
        }
        // @ts-ignore
        [];
        var __VLS_15;
    }
    // @ts-ignore
    [];
    var __VLS_9;
}
// @ts-ignore
[];
var __VLS_3;
let __VLS_18;
/** @ts-ignore @type { | typeof __VLS_components.Transition | typeof __VLS_components.Transition} */
Transition;
// @ts-ignore
const __VLS_19 = __VLS_asFunctionalComponent1(__VLS_18, new __VLS_18({
    name: "dialog-fade",
}));
const __VLS_20 = __VLS_19({
    name: "dialog-fade",
}, ...__VLS_functionalComponentArgsRest(__VLS_19));
const { default: __VLS_23 } = __VLS_21.slots;
if (__VLS_ctx.artifactDialogVisible) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.artifactDialogVisible))
                    return;
                __VLS_ctx.artifactDialogVisible = false;
                // @ts-ignore
                [artifactDialogVisible, artifactDialogVisible,];
            } },
        ...{ class: "dp-dialog-mask" },
    });
    /** @type {__VLS_StyleScopedClasses['dp-dialog-mask']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "dp-dialog dp-artifact-dialog" },
    });
    /** @type {__VLS_StyleScopedClasses['dp-dialog']} */ ;
    /** @type {__VLS_StyleScopedClasses['dp-artifact-dialog']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "dp-dialog-header" },
    });
    /** @type {__VLS_StyleScopedClasses['dp-dialog-header']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    (__VLS_ctx.artifactDialogName);
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ style: {} },
    });
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
        ...{ class: "dp-art-path" },
    });
    /** @type {__VLS_StyleScopedClasses['dp-art-path']} */ ;
    (__VLS_ctx.artifactDialogPath);
    __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.artifactDialogVisible))
                    return;
                __VLS_ctx.artifactDialogVisible = false;
                // @ts-ignore
                [artifactDialogVisible, artifactDialogName, artifactDialogPath,];
            } },
        ...{ class: "dp-dialog-close" },
    });
    /** @type {__VLS_StyleScopedClasses['dp-dialog-close']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "dp-dialog-body dp-artifact-body" },
    });
    /** @type {__VLS_StyleScopedClasses['dp-dialog-body']} */ ;
    /** @type {__VLS_StyleScopedClasses['dp-artifact-body']} */ ;
    if (__VLS_ctx.artifactDialogContent) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.pre, __VLS_intrinsics.pre)({
            ...{ class: "dp-artifact-pre" },
        });
        /** @type {__VLS_StyleScopedClasses['dp-artifact-pre']} */ ;
        (__VLS_ctx.artifactDialogContent);
    }
    else if (__VLS_ctx.artifactLoading) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "dp-dialog-empty" },
        });
        /** @type {__VLS_StyleScopedClasses['dp-dialog-empty']} */ ;
    }
    else {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "dp-dialog-empty" },
        });
        /** @type {__VLS_StyleScopedClasses['dp-dialog-empty']} */ ;
    }
}
// @ts-ignore
[artifactDialogContent, artifactDialogContent, artifactLoading,];
var __VLS_21;
let __VLS_24;
/** @ts-ignore @type { | typeof __VLS_components.Transition | typeof __VLS_components.Transition} */
Transition;
// @ts-ignore
const __VLS_25 = __VLS_asFunctionalComponent1(__VLS_24, new __VLS_24({
    name: "dialog-fade",
}));
const __VLS_26 = __VLS_25({
    name: "dialog-fade",
}, ...__VLS_functionalComponentArgsRest(__VLS_25));
const { default: __VLS_29 } = __VLS_27.slots;
if (__VLS_ctx.reportDialogVisible) {
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.reportDialogVisible))
                    return;
                __VLS_ctx.reportDialogVisible = false;
                // @ts-ignore
                [reportDialogVisible, reportDialogVisible,];
            } },
        ...{ class: "dp-dialog-mask" },
    });
    /** @type {__VLS_StyleScopedClasses['dp-dialog-mask']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "dp-dialog" },
    });
    /** @type {__VLS_StyleScopedClasses['dp-dialog']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "dp-dialog-header" },
    });
    /** @type {__VLS_StyleScopedClasses['dp-dialog-header']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({});
    (__VLS_ctx.reportDialogAgent);
    __VLS_asFunctionalElement1(__VLS_intrinsics.button, __VLS_intrinsics.button)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.reportDialogVisible))
                    return;
                __VLS_ctx.reportDialogVisible = false;
                // @ts-ignore
                [reportDialogVisible, reportDialogAgent,];
            } },
        ...{ class: "dp-dialog-close" },
    });
    /** @type {__VLS_StyleScopedClasses['dp-dialog-close']} */ ;
    __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
        ...{ class: "dp-dialog-body" },
    });
    /** @type {__VLS_StyleScopedClasses['dp-dialog-body']} */ ;
    for (const [r] of __VLS_vFor((__VLS_ctx.reportDialogRecords))) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            key: (r.timestamp),
            ...{ class: "dp-timeline-item" },
        });
        /** @type {__VLS_StyleScopedClasses['dp-timeline-item']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div)({
            ...{ class: "dp-tl-dot" },
            ...{ class: ('tl-' + r.status) },
        });
        /** @type {__VLS_StyleScopedClasses['dp-tl-dot']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "dp-tl-content" },
        });
        /** @type {__VLS_StyleScopedClasses['dp-tl-content']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "dp-tl-text" },
        });
        /** @type {__VLS_StyleScopedClasses['dp-tl-text']} */ ;
        (r.content);
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "dp-tl-meta" },
        });
        /** @type {__VLS_StyleScopedClasses['dp-tl-meta']} */ ;
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "dp-tl-time" },
        });
        /** @type {__VLS_StyleScopedClasses['dp-tl-time']} */ ;
        (__VLS_ctx.formatTime(r.timestamp));
        if (r.progress > 0) {
            __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
                ...{ class: "dp-tl-progress" },
            });
            /** @type {__VLS_StyleScopedClasses['dp-tl-progress']} */ ;
            (r.progress);
        }
        __VLS_asFunctionalElement1(__VLS_intrinsics.span, __VLS_intrinsics.span)({
            ...{ class: "dp-tl-status dp-tag" },
            ...{ class: ('tag-' + r.status) },
        });
        /** @type {__VLS_StyleScopedClasses['dp-tl-status']} */ ;
        /** @type {__VLS_StyleScopedClasses['dp-tag']} */ ;
        (__VLS_ctx.statusLabel(r.status));
        // @ts-ignore
        [statusLabel, reportDialogRecords, formatTime,];
    }
    if (!__VLS_ctx.reportDialogRecords.length) {
        __VLS_asFunctionalElement1(__VLS_intrinsics.div, __VLS_intrinsics.div)({
            ...{ class: "dp-dialog-empty" },
        });
        /** @type {__VLS_StyleScopedClasses['dp-dialog-empty']} */ ;
    }
}
// @ts-ignore
[reportDialogRecords,];
var __VLS_27;
// @ts-ignore
[];
const __VLS_export = (await import('vue')).defineComponent({
    setup: () => __VLS_exposed,
    __typeProps: {},
});
export default {};
