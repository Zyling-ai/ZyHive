import { defineStore } from 'pinia';
import { ref } from 'vue';
import { agents as agentsApi } from '../api';
export const useAgentsStore = defineStore('agents', () => {
    const list = ref([]);
    const loading = ref(false);
    async function fetchAll() {
        loading.value = true;
        try {
            const res = await agentsApi.list();
            list.value = res.data;
        }
        catch (e) {
            console.error('Failed to fetch agents', e);
        }
        finally {
            loading.value = false;
        }
    }
    async function createAgent(data) {
        const res = await agentsApi.create(data);
        list.value.push(res.data);
        return res.data;
    }
    return { list, loading, fetchAll, createAgent };
});
