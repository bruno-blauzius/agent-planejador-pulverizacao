<script setup lang="ts">
/**
 * Component showcase for development only (/dev/components): renders the base
 * components with placeholder content. Not available in production builds.
 */
if (!import.meta.dev) throw createError({ statusCode: 404, statusMessage: 'Página não encontrada' })

useHead({ title: 'Componentes · Agentes Agro' })

type Choice = 'first' | 'second'
type Tab = 'overview' | 'details' | 'history'

const choice = ref<Choice>('first')
const tab = ref<Tab>('overview')
const text = ref('')
const isModalOpen = ref(false)
const isToastVisible = ref(true)
const selectedFileName = ref<string>()

const choiceOptions = [
  { value: 'first', label: 'Opção A' },
  { value: 'second', label: 'Opção B' },
] as const

const tabItems = [
  { value: 'overview', label: 'Visão geral' },
  { value: 'details', label: 'Detalhes', count: 0 },
  { value: 'history', label: 'Histórico' },
] as const

type Role = 'admin' | 'operator'
const role = ref<Role | null>('admin')
const company = ref<string | null>(null)

const roleOptions = [
  { value: 'admin', title: 'Opção A', description: 'Descrição da opção A.', icon: 'shield' },
  { value: 'operator', title: 'Opção B', description: 'Descrição da opção B.', icon: 'sprout' },
] as const

const companyItems = [
  { value: 'a', label: 'Item A' },
  { value: 'b', label: 'Item B' },
] as const

function onFilesSelected(files: File[]) {
  selectedFileName.value = files[0]?.name
}
</script>

<template>
  <div class="page">
    <PageHeader title="Componentes" eyebrow="Desenvolvimento" subtitle="Página temporária para validar o layout e os componentes base.">
      <template #actions>
        <BaseButton variant="ghost">Secundária</BaseButton>
        <BaseButton icon="plus" @click="isModalOpen = true">Abrir modal</BaseButton>
      </template>
    </PageHeader>

    <ToastAlert
      v-if="isToastVisible"
      tone="success"
      title="Layout carregado"
      message="Componentes base renderizados com os tokens do Figma."
      floating
      @dismiss="isToastVisible = false"
    />

    <section class="showcase" aria-label="Componentes base">
      <div class="showcase__row">
        <BaseButton variant="primary" icon="plus">Primário</BaseButton>
        <BaseButton variant="accent" icon="sparkles">Acento</BaseButton>
        <BaseButton variant="secondary" icon="plus">Secundário</BaseButton>
        <BaseButton variant="ghost" icon="plus">Fantasma</BaseButton>
        <BaseButton variant="danger" icon="trash">Perigo</BaseButton>
        <BaseButton size="sm" disabled icon="plus">Desabilitado</BaseButton>
      </div>

      <div class="showcase__row">
        <StatusBadge tone="suitable" />
        <StatusBadge tone="attention" />
        <StatusBadge tone="unsuitable" />
        <StatusBadge tone="rain" />
        <SegmentedControl v-model="choice" :options="choiceOptions" label="Exemplo de seleção" />
      </div>

      <TabsNav v-model="tab" :items="tabItems" label="Exemplo de abas" />

      <div class="showcase__grid">
        <TextField v-model="text" label="Rótulo" placeholder="Digite aqui" icon="mail" hint="Texto de ajuda" />
        <TextField label="Com erro" placeholder="Digite aqui" error="Mensagem de erro" />
        <TextField label="Desabilitado" placeholder="Digite aqui" disabled />
      </div>

      <FileDropzone
        title="Documento (opcional)"
        description="PDF ou imagem"
        :file-name="selectedFileName"
        @select="onFilesSelected"
      />

      <MapView alt="Ilustração estática de talhões" rounded>
        <StatusBadge tone="suitable" class="showcase__map-badge" />
      </MapView>
    </section>

    <section class="showcase" aria-label="Componentes da página 03">
      <div class="showcase__grid">
        <StatCard label="Indicador" value="12" caption="Detalhe do indicador" />
        <StatCard label="Indicador do agente" value="3" caption="Detalhe" highlight />
      </div>
      <div class="showcase__grid">
        <OptionCardGroup v-model="role" :options="roleOptions" label="Cartões de opção" />
        <BaseSelect v-model="company" label="Campo de seleção" :items="companyItems" />
      </div>
      <div class="showcase__row">
        <BaseAvatar initials="AB" seed="a" />
        <BaseAvatar initials="CD" seed="b" size="md" />
      </div>
      <ForbiddenState />
    </section>

    <BaseModal
      v-model:open="isModalOpen"
      eyebrow="Exemplo"
      title="Título do modal"
      description="Subtítulo explicando a ação."
      icon="user"
    >
      <TextField v-model="text" label="Campo" placeholder="Digite aqui" data-autofocus />
      <template #actions>
        <BaseButton variant="secondary" block @click="isModalOpen = false">Cancelar</BaseButton>
        <BaseButton block @click="isModalOpen = false">Continuar</BaseButton>
      </template>
    </BaseModal>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: var(--space-28);
}

.showcase {
  display: flex;
  flex-direction: column;
  gap: var(--space-24);
}

.showcase__row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-12);
  align-items: center;
}

.showcase__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(var(--grid-min-column), 1fr));
  gap: var(--space-16);
}

.showcase__map-badge {
  position: absolute;
  top: var(--space-16);
  left: var(--space-16);
}
</style>
