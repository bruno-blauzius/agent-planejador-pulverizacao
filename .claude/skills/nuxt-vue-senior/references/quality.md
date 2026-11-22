# Qualidade: testes, lint, acessibilidade, performance

## Testes

**Pirâmide:** muitos testes de funções puras e composables, um número razoável de testes de componente, poucos E2E cobrindo fluxos críticos.

Config com dois ambientes no Vitest:

```ts
// vitest.config.ts
import { defineConfig } from 'vitest/config'
import { defineVitestProject } from '@nuxt/test-utils/config'

export default defineConfig({
  test: {
    projects: [
      { test: { name: 'unit', include: ['test/unit/**/*.spec.ts'], environment: 'node' } },
      await defineVitestProject({
        test: { name: 'nuxt', include: ['test/nuxt/**/*.spec.ts'], environment: 'nuxt' },
      }),
    ],
  },
})
```

Teste de componente:

```ts
import { mountSuspended } from '@nuxt/test-utils/runtime'
import UserCard from '~/components/user/UserCard.vue'

it('emite select com o id ao clicar', async () => {
  const wrapper = await mountSuspended(UserCard, {
    props: { user: { id: '1', name: 'Ana Souza', email: 'ana@x.com', role: 'member' } },
  })
  await wrapper.get('button').trigger('click')
  expect(wrapper.emitted('select')?.[0]).toEqual(['1'])
  expect(wrapper.text()).toContain('AS')
})
```

- Use `mockNuxtImport` para mockar auto-imports e `registerEndpoint` para mockar rotas de API.
- Teste comportamento observável (texto, eventos, chamadas), não detalhes internos.
- Selecione por papel/texto/label; use `data-testid` só quando não houver alternativa semântica.
- E2E com Playwright: login, fluxo principal de negócio, checkout/submit críticos.

## Lint, formatação e tipos

- `@nuxt/eslint` com flat config (`eslint.config.mjs` gerado com `withNuxt`).
- `nuxi typecheck` (vue-tsc) no CI.
- Hooks de pre-commit (lint-staged + husky ou simple-git-hooks).
- `strict: true` no TypeScript; `noUncheckedIndexedAccess` quando o time topar.

## Acessibilidade

- Um `<h1>` por página, hierarquia de headings sem pular níveis.
- Todo input com label (visível ou `aria-label`); erros ligados ao campo.
- Elementos clicáveis são `<button>` ou `<a>`, nunca `<div @click>`.
- Foco visível e ordem de tabulação lógica; modais prendem o foco e fecham com Esc.
- Contraste mínimo AA; não comunicar informação só por cor.
- Imagens com `alt` significativo (ou `alt=""` se decorativas).
- Verifique com `@nuxt/a11y`/axe ou Lighthouse.

## Performance

- Componentes pesados abaixo da dobra: prefixo `Lazy` e, no Nuxt 3.16+/4, hidratação tardia (`hydrate-on-visible`, `hydrate-on-idle`).
- `<NuxtImg>`/`<NuxtPicture>` com tamanhos e formatos modernos.
- `routeRules` para estratégia por rota: `prerender`, `swr`, `isr`, `ssr: false`.
- `shallowRef` para listas/objetos grandes substituídos por inteiro.
- Virtualização (`useVirtualList` do VueUse) para listas longas.
- Evite importar libs inteiras (lodash completo, ícones em massa); prefira imports pontuais.
- Analise o bundle com `nuxi analyze`.
- SEO: `useSeoMeta`, `useHead`, sitemap e OG images em páginas públicas.

## Checklist de revisão de código

Ao revisar ou entregar código, verifique:

- [ ] `<script setup lang="ts">`, sem `any`, props/emits tipados
- [ ] Componente com uma responsabilidade clara e tamanho razoável
- [ ] Lógica reutilizável em composable; regra de negócio em função pura
- [ ] Estados de carregando, erro, vazio e sucesso cobertos
- [ ] Nada de `ref` global em código SSR; acesso a APIs do browser protegido
- [ ] `useFetch` apenas no setup; `$fetch` em ações
- [ ] `key` estável em `v-for`; sem `v-if` + `v-for` no mesmo elemento
- [ ] Acessibilidade básica (semântica, labels, teclado)
- [ ] Nomes claros, sem comentários explicando o óbvio, sem código morto
- [ ] Testes para composables/utils e comportamentos críticos do componente

Ao apontar problemas numa revisão, classifique por severidade (bloqueante / importante / sugestão) e mostre o trecho corrigido, não só a crítica.
