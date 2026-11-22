# Arquitetura frontend com Nuxt

## Camadas

```
Página (pages/)          → orquestra: rota, SEO, layout, compõe features
  └─ Componentes          → apresentação; recebem props, emitem eventos
       └─ Composables     → estado reativo e casos de uso da feature
            └─ Services   → acesso a dados ($fetch tipado), sem Vue
                 └─ API   → server/api (BFF) ou backend externo
Domínio (utils/, domain/) → funções puras e regras de negócio, sem Vue nem I/O
```

Regra de dependência: camadas internas não conhecem as externas. Um service não importa componente; uma função de domínio não importa `ref`. Isso mantém a regra de negócio testável com um teste unitário simples.

## Organização por feature

Para projetos médios/grandes, agrupe por domínio:

```
app/features/orders/
├── components/OrderTable.vue
├── composables/useOrders.ts
├── services/orders.service.ts
├── stores/orders.store.ts
├── types.ts
└── index.ts            # API pública da feature
```

Registre os diretórios no `nuxt.config.ts` (`components.dirs`, `imports.dirs`) ou use **Nuxt Layers** (`extends`) para isolar domínios ou compartilhar um design system entre apps. Uma feature importa outra apenas pelo `index.ts`.

## SOLID aplicado ao frontend

- **S** — Um componente, um motivo para mudar. Separe container (busca/estado) de apresentação.
- **O** — Estenda por slots, props de variante e composição, não editando o componente base a cada caso novo.
- **L** — Um wrapper (`BaseInput`) deve aceitar tudo que o elemento nativo aceita: repasse atributos (`inheritAttrs` + `v-bind="$attrs"`) e mantenha o contrato de v-model.
- **I** — Props mínimas e coesas. Muitas props opcionais desconexas indicam que o componente deve ser dividido.
- **D** — Composables dependem de abstrações: receba o service/fetcher por parâmetro ou injeção quando precisar trocar em testes.

```ts
// Service tipado, isolado do Vue
export function createOrdersService(fetcher = $fetch) {
  return {
    list: (params: OrderFilters) => fetcher<Order[]>('/api/orders', { query: params }),
    get: (id: string) => fetcher<Order>(`/api/orders/${id}`),
  }
}
```

## Padrões de componentes

**Variantes tipadas** (com `tailwind-variants`, já usado pelo Nuxt UI):

```ts
const button = tv({
  base: 'inline-flex items-center rounded-md font-medium',
  variants: {
    intent: { primary: 'bg-primary text-inverted', ghost: 'hover:bg-elevated' },
    size: { sm: 'h-8 px-3 text-sm', md: 'h-10 px-4' },
  },
  defaultVariants: { intent: 'primary', size: 'md' },
})
```

**Slots tipados:**

```vue
<script setup lang="ts" generic="T extends { id: string }">
defineProps<{ items: T[] }>()
defineSlots<{ item(props: { item: T; index: number }): any; empty(): any }>()
</script>
```

Use `generic` em listas, tabelas e selects para preservar o tipo do item no consumidor.

**Compound components** (Tabs, Accordion): o pai faz `provide` de um contexto tipado com `InjectionKey`, os filhos fazem `inject`. Exponha um composable `useTabsContext()` que lança erro claro se usado fora do pai.

**Renderless / composable-first:** se o comportamento é reutilizável mas a aparência varia, extraia para composable e deixe cada componente renderizar do seu jeito.

## Provide/inject tipado

```ts
export const ThemeKey: InjectionKey<Ref<'light' | 'dark'>> = Symbol('theme')
```

## Quando criar o quê

| Situação | Solução |
|---|---|
| Lógica reativa usada em 2+ componentes | Composable |
| Estado que precisa sobreviver à navegação ou ser compartilhado pelo app | Pinia |
| Estado SSR simples compartilhado | `useState` |
| Cálculo puro reutilizado | Função em `utils/` |
| Marcação repetida | Componente |
| Comportamento transversal em rotas | Middleware |
| Integração com lib de terceiros na inicialização | Plugin |
