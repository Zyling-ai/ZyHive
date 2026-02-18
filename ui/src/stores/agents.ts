import { defineStore } from 'pinia'
import { ref } from 'vue'
import { agents as agentsApi, type AgentInfo } from '../api'

export const useAgentsStore = defineStore('agents', () => {
  const list = ref<AgentInfo[]>([])
  const loading = ref(false)

  async function fetchAll() {
    loading.value = true
    try {
      const res = await agentsApi.list()
      list.value = res.data
    } catch (e) {
      console.error('Failed to fetch agents', e)
    } finally {
      loading.value = false
    }
  }

  async function createAgent(id: string, name: string, model: string) {
    const res = await agentsApi.create({ id, name, model })
    list.value.push(res.data)
    return res.data
  }

  return { list, loading, fetchAll, createAgent }
})
