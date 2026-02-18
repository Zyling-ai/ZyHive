import { defineStore } from 'pinia'
import { ref } from 'vue'
import { config as configApi } from '../api'

export const useConfigStore = defineStore('config', () => {
  const data = ref<any>(null)
  const loading = ref(false)

  async function fetch() {
    loading.value = true
    try {
      const res = await configApi.get()
      data.value = res.data
    } finally {
      loading.value = false
    }
  }

  async function save(patch: any) {
    const res = await configApi.patch(patch)
    data.value = res.data
  }

  return { data, loading, fetch, save }
})
