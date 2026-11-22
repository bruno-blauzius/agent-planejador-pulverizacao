# ADR 0011: Vuetify como biblioteca de componentes do front, tematizado para o design do Figma

- **Status:** aceito (2026-11-21)
- **Data:** 2026-11-21

## Contexto

O `web/` vai implementar as telas do Figma "Agentes Agro" (página "01 · Acesso e Cadastros", depois "02 · Planejador de Pulverização"). A fundação de UI já existe com componentes próprios (`BaseButton`, `TextField`, `SegmentedControl`, `TabsNav`, `PageHeader`, `BaseModal`, `ToastAlert`, `FileDropzone`, `StatusBadge`, `MapView`, `AppSidebar`, `AppTopbar`, `AppIcon`), tokens em CSS custom properties (`app/assets/css/tokens.css`, `typography.css`) e nenhuma biblioteca de componentes.

As telas seguintes pedem componentes de comportamento complexo que hoje teriam de ser escritos à mão: tabela com ordenação/paginação (proprietários), autocomplete com busca remota (proprietário no assistente), menus, tooltips, popovers sobre o mapa, seleção de datas, navegação lateral responsiva. Escrever e manter isso com acessibilidade correta é caro.

Nenhuma tela foi implementada ainda: é o ponto mais barato para trocar a base visual.

O design do Figma **não é Material Design**: tem identidade própria (verde café, Bricolage Grotesque + Plus Jakarta Sans, raios e sombras próprios, sem caixa-alta em botões, sem ripple). Qualquer biblioteca adotada precisa ser dobrada a esse design, e não o contrário.

## Opções

1. **Manter só componentes próprios.** Fidelidade total ao Figma e zero dependência; em troca, implementar à mão tabela, autocomplete, menus, datas e acessibilidade de teclado de cada um.
2. **Biblioteca headless (Reka UI).** Comportamento e acessibilidade prontos, estilo 100% nosso; não traz componentes "completos" como tabela de dados e seletor de datas estilizados.
3. **Nuxt UI (Tailwind).** Muito tematizável, mas exige adotar Tailwind e reescrever os tokens e estilos já existentes.
4. **Vuetify, tematizado para o design do Figma** (escolhida por decisão do responsável pelo projeto).

## Decisão

Adotar **Vuetify** (versão estável mais recente no momento da instalação, fixada no `pnpm-lock.yaml`) como biblioteca de componentes do `web/`, com o objetivo explícito de **reproduzir fielmente o Figma**, não o visual Material. A fidelidade é obtida em camadas, da mais global para a mais local, e cada camada só é usada quando a anterior não resolve.

### 1. Integração

- Plugin próprio `app/plugins/vuetify.ts` com `createVuetify({ ssr: true, ... })` e `vite-plugin-vuetify` (`autoImport: true`, `styles: { configFile: 'assets/styles/vuetify-settings.scss' }`), seguindo a integração oficial com Vite. Não usar módulo Nuxt de terceiros.
- `build.transpile: ['vuetify']` no `nuxt.config.ts`.
- Estilos do Vuetify em **cascade layers**: o Vuetify 4 já envolve todo o seu CSS (inclusive o CSS por componente) nas layers nativas `vuetify-*` (`vuetify-core`, `vuetify-components`, `vuetify-utilities`, `vuetify-final`); os estilos do projeto (`main.css` e `<style scoped>`) ficam **fora de layer**. Assim os ajustes de design vencem por ordem de camada, **sem `!important`** e sem uma layer própria.
- O scrim dos diálogos usa a classe global `.base-modal-overlay` (com `--color-overlay`), porque o overlay é teleportado para fora do escopo do componente.
- `$border-radius-root` fica fixo em 12px (espelho de `--radius-md`) porque o Sass faz contas com ele; a escala nomeada de `rounded` aponta para os tokens.
- `<v-app>` envolve a aplicação em `app.vue` (necessário para overlays, tema e layout do Vuetify).

### 2. Tokens: fonte única da verdade

- A paleta do Figma (página 0:1) passa a viver em `app/theme/palette.ts`, que alimenta o tema do Vuetify (`theme.themes.agentesAgro.colors`).
- O Vuetify publica cada cor como `--v-theme-<nome>`; o `tokens.css` mantém os nomes atuais (`--color-green-900`, `--color-status-suitable`...) como **aliases** dessas variáveis (`rgb(var(--v-theme-green-900))`). Componentes próprios continuam usando `--color-*`; nada de duplicar hexadecimais.
- Cores semânticas do Vuetify mapeadas para o design: `primary` = verde-900, `secondary` = café-700, `accent` = broto-400, `success`/`warning`/`error`/`info` = status apta/atenção/inapta/info-chuva, `background` = areia, `surface` = branco.
- Espaçamento (`--space-*`), raios (`--radius-*`), sombras (`--shadow-*`), movimento e estilos de texto (`typography.css`) continuam em CSS e são referenciados também pelo `vuetify-settings.scss`.

### 3. Variáveis SASS: remover o "sotaque" Material globalmente

Em `assets/styles/vuetify-settings.scss`, a partir dos tokens:

- Fontes: `$body-font-family` = Plus Jakarta Sans; títulos com Bricolage Grotesque.
- Botões: `$button-text-transform: none`, `$button-letter-spacing: 0`, alturas e padding por tamanho conforme a biblioteca do Figma (lg/md/sm), peso da fonte do Figma.
- Raios: `$border-radius-root` e os mapas de `rounded` apontando para `--radius-*`.
- Sombras: elevações mapeadas para `--shadow-*` do Figma (cartões sem elevação por padrão, com borda).
- Campos: altura, padding, cor e espessura da borda, raio e estilo do rótulo conforme "Campo de texto" do Figma.
- Diálogo, snackbar, abas e chips: raio, padding e tipografia do Figma.

### 4. Padrões globais (`defaults`) e desligamentos

- `ripple: false` global (o Figma não tem ripple).
- `VBtn`: `variant: 'flat'`, `rounded`, `density` do Figma; `VTextField`/`VSelect`/`VAutocomplete`: `variant: 'outlined'`, `hideDetails: 'auto'`, `persistentPlaceholder`; `VCard`: `elevation: 0`, `border`; `VDialog`: `maxWidth: 640`, `scrim` com `--color-overlay`; `VSnackbar`: posição e largura do toast do Figma.
- **Ícones:** não usar MDI. Registrar um icon set próprio que renderiza o `AppIcon` (47 ícones exportados do Figma), para que `v-icon` e ícones internos do Vuetify (setas, fechar, checkbox) usem o desenho do Figma.
- **Tipografia e grid:** não usar as classes `text-h*`/`text-body-*` nem o grid (`v-row`/`v-col`) do Vuetify; layout e texto seguem `typography.css` e CSS próprio, fiel às medidas do Figma.

### 5. Componentes do projeto como fachada

- Os componentes `Base*`/`App*` **mantêm o nome e a API pública** (props, eventos, slots) e passam a usar Vuetify por dentro. As telas consomem a fachada, não o Vuetify diretamente, o que concentra o ajuste visual em um lugar e protege as telas de mudanças da biblioteca.
- Mapeamento inicial:

  | Componente do projeto | Base Vuetify |
  |---|---|
  | `BaseButton` | `v-btn` |
  | `TextField` | `v-text-field` |
  | `SegmentedControl` | `v-btn-toggle` |
  | `TabsNav` | `v-tabs` |
  | `BaseModal` | `v-dialog` + `v-card` |
  | `ToastAlert` (flutuante) | `v-snackbar`; inline: `v-alert` |
  | `StatusBadge` | `v-chip` |
  | `AppSidebar` | `v-navigation-drawer` (ganha o recolhimento em telas menores) |
  | `AppTopbar` | `v-app-bar` |
  | `PageHeader`, `FileDropzone`, `MapView`, `AppAuthHero` | permanecem próprios (sem equivalente útil) |

- Componentes Vuetify sem fachada (`v-data-table`, `v-autocomplete`, `v-menu`, `v-tooltip`, `v-date-picker`) podem ser usados nas telas; ao aparecerem em **duas ou mais telas**, ganham fachada própria (ex.: `DataTable`, `OwnerAutocomplete`).
- Ajustes locais (último recurso) ficam no `<style scoped>` da fachada, usando tokens, nunca valores soltos.

### 6. Verificação de fidelidade

- Cada fachada migrada é comparada com o screenshot do Figma (1440 px) no navegador antes de ser dada como pronta; estados de foco, hover, desabilitado e erro inclusos.
- Divergências inevitáveis da biblioteca são registradas em `.claude/progresso-telas.md` ("Divergências de design") com o motivo.
- Testes de componente (Vitest) que montam fachadas usam uma instância de Vuetify de teste criada em `test/support/`.

## Dependências novas

| Pacote | Tipo | Por quê |
|---|---|---|
| `vuetify` | runtime | a biblioteca de componentes |
| `vite-plugin-vuetify` | dev | tree-shaking por auto-import e uso do arquivo de variáveis SASS |
| `sass` | dev | compilar as variáveis SASS do Vuetify (camada 3) |

## Consequências

**Positivas**
- Tabela, autocomplete, menus, tooltips, datas e drawer responsivo prontos, com acessibilidade de teclado e ARIA mantidos pela biblioteca.
- A API das fachadas não muda: a fundação revisada continua válida para quem consome; a lógica (`services`, `composables`) não é afetada.
- Tokens seguem com fonte única (paleta em TS, aliases em CSS).

**Negativas**
- Reproduzir o Figma exige sobrescrever bastante do Material (densidade, estados, foco); a fidelidade depende da disciplina nas camadas 3 a 5 e da verificação visual.
- Bundle maior que a solução atual, mitigado por tree-shaking.
- Configuração de SSR e de cascade layers a manter; atualizações de versão maior do Vuetify podem quebrar ajustes de estilo.
- Retrabalho na fundação: as fachadas listadas acima são reimplementadas por dentro; o ajuste de estilo feito nelas se perde em parte (os tokens permanecem).

## Plano de migração

1. Instalar dependências, criar `app/plugins/vuetify.ts`, `app/theme/palette.ts`, `assets/styles/vuetify-settings.scss` e o icon set do `AppIcon`; ajustar `tokens.css` para aliases.
2. Migrar as fachadas uma a uma, mantendo a API e verificando contra o Figma.
3. Revisão independente da fundação migrada (lint, typecheck, testes e comparação visual).
4. Só então iniciar a primeira tela (Login).
