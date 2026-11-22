# Dados e estado

## Busca de dados

| Uso | API |
|---|---|
| Dados para renderizar a página (SSR + hidratação) | `useFetch` |
| Lógica customizada ou várias chamadas combinadas | `useAsyncData` com chave explícita |
| Ação do usuário (submit, clique, delete) | `$fetch` |
| Dados só no cliente | `useFetch(..., { server: false })` |
| Não bloquear navegação | `lazy: true` (ou `useLazyFetch`) e tratar `status === 'pending'` |

Regras:

- `useFetch`/`useAsyncData` apenas no nível de setup, nunca dentro de handlers ou após `await` arbitrário.
- Passe refs/getters em `query`/URL para refazer a busca automaticamente; use `watch: false` quando não quiser.
- Use `default: () => []` para evitar `null` no template.
- Use `transform` ou `pick` para reduzir o payload serializado no SSR.
- Chaves em `useAsyncData` devem ser únicas e estáveis; reutilize a mesma chave para compartilhar cache entre componentes.
- Após mutações, invalide com `refresh()` ou `refreshNuxtData(key)`.
- Para cache de servidor sofisticado (stale-while-revalidate, mutações otimistas, paginação infinita), use Pinia Colada ou TanStack Query.

## Cliente HTTP centralizado

Crie uma instância com `$fetch.create` num plugin ou composable para base URL, token e tratamento de 401 em um só lugar:

```ts
// app/plugins/api.ts
export default defineNuxtPlugin(() => {
  const config = useRuntimeConfig()
  const token = useCookie('token')

  const api = $fetch.create({
    baseURL: config.public.apiBase,
    onRequest({ options }) {
      if (token.value) options.headers.set('Authorization', `Bearer ${token.value}`)
    },
    async onResponseError({ response }) {
      if (response.status === 401) await navigateTo('/login')
    },
  })

  return { provide: { api } }
})
```

Use com `useFetch('/x', { $fetch: useNuxtApp().$api })` ou encapsule num `useApi`.

## BFF com `server/api`

Prefira que o frontend fale com rotas Nitro em `server/api`, que chamam o backend real. Benefícios: segredos ficam no servidor, agregação de chamadas, tipagem automática das respostas em `useFetch`.

```ts
// server/api/users/index.get.ts
import { z } from 'zod'

const querySchema = z.object({ q: z.string().optional() })

export default defineEventHandler(async (event) => {
  const { q } = await getValidatedQuery(event, querySchema.parse)
  return await $fetch<User[]>(`${useRuntimeConfig().apiUrl}/users`, { query: { q } })
})
```

## Pinia (setup stores)

```ts
// app/stores/cart.ts
export const useCartStore = defineStore('cart', () => {
  const items = ref<CartItem[]>([])

  const total = computed(() => items.value.reduce((s, i) => s + i.price * i.qty, 0))

  function add(item: CartItem) {
    const existing = items.value.find(i => i.id === item.id)
    existing ? existing.qty++ : items.value.push({ ...item, qty: 1 })
  }

  return { items, total, add }
})
```

- Desestruture com `storeToRefs(store)` para estado e getters; ações direto da store.
- Não coloque na store dado que é só cache de servidor — isso é papel do `useFetch`/query.
- Persistência: `pinia-plugin-persistedstate` com cookies (SSR-safe) quando necessário.

## Erros

- Em páginas, trate o `error` retornado pelo `useFetch` com UI de erro e ação de "tentar novamente".
- Para 404/erros fatais de rota: `throw createError({ statusCode: 404, statusMessage: 'Não encontrado', fatal: true })` e personalize `app/error.vue`.
- Use `<NuxtErrorBoundary>` para isolar falhas de um widget sem derrubar a página.
- Mensagens para o usuário em linguagem clara; detalhes técnicos só no log.

## Formulários

Schema único, tipo derivado dele:

```ts
// shared/schemas/user.ts
export const userSchema = z.object({
  name: z.string().min(2, 'Informe ao menos 2 caracteres'),
  email: z.email('E-mail inválido'),
})
export type UserInput = z.infer<typeof userSchema>
```

```vue
<script setup lang="ts">
const state = reactive<Partial<UserInput>>({ name: '', email: '' })
const toast = useToast()

async function onSubmit(event: FormSubmitEvent<UserInput>) {
  await $fetch('/api/users', { method: 'POST', body: event.data })
  toast.add({ title: 'Usuário criado', color: 'success' })
}
</script>

<template>
  <UForm :schema="userSchema" :state="state" class="space-y-4" @submit="onSubmit">
    <UFormField label="Nome" name="name"><UInput v-model="state.name" /></UFormField>
    <UFormField label="E-mail" name="email"><UInput v-model="state.email" type="email" /></UFormField>
    <UButton type="submit" label="Salvar" />
  </UForm>
</template>
```

Valide o mesmo schema no `server/api` — validação no cliente é UX, no servidor é segurança. Confira a sintaxe da versão de Zod instalada (v3 usa `z.string().email()`, v4 aceita `z.email()`).
