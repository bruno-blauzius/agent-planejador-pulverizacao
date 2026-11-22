<script setup lang="ts">
/**
 * File upload area (Figma 39:1113). Click or drop files; emits the selection.
 * Validation (type/size) and upload belong to the caller's composable.
 */
const props = withDefaults(
  defineProps<{
    title: string
    description?: string
    actionLabel?: string
    accept?: string
    multiple?: boolean
    disabled?: boolean
    /** Name of the file already chosen, shown in place of the description. */
    fileName?: string
    error?: string
    /**
     * `stacked`: centered icon, title and detail without the action label (Figma 35:672).
     * `hero`: large drop target with icon tile, display title and an action pill (Figma 37:778).
     */
    layout?: 'inline' | 'stacked' | 'hero'
  }>(),
  {
    description: undefined,
    actionLabel: 'Enviar',
    accept: undefined,
    multiple: false,
    disabled: false,
    fileName: undefined,
    error: undefined,
    layout: 'inline',
  },
)

const emit = defineEmits<{
  select: [files: File[]]
}>()

const ICON_SIZE: Record<'inline' | 'stacked' | 'hero', number> = { inline: 18, stacked: 22, hero: 24 }

const inputId = useId()
const messageId = `${inputId}-message`
const isDragging = ref(false)

const detail = computed(() => props.error ?? props.fileName ?? props.description)

function emitFiles(list: FileList | null | undefined) {
  const files = Array.from(list ?? [])
  if (files.length === 0) return
  emit('select', props.multiple ? files : files.slice(0, 1))
}

function onChange(event: Event) {
  const input = event.target as HTMLInputElement
  emitFiles(input.files)
  // Reset so choosing the same file again still fires `change`.
  input.value = ''
}

function onDragOver() {
  if (!props.disabled) isDragging.value = true
}

function onDrop(event: DragEvent) {
  isDragging.value = false
  if (props.disabled) return
  emitFiles(event.dataTransfer?.files)
}
</script>

<template>
  <div
    class="dropzone"
    :class="{
      'dropzone--dragging': isDragging,
      'dropzone--disabled': disabled,
      'dropzone--error': Boolean(error),
      'dropzone--stacked': layout === 'stacked',
      'dropzone--hero': layout === 'hero',
    }"
    @dragover.prevent="onDragOver"
    @dragleave="isDragging = false"
    @drop.prevent="onDrop"
  >
    <input
      :id="inputId"
      type="file"
      class="visually-hidden"
      :accept="accept"
      :multiple="multiple"
      :disabled="disabled"
      :aria-describedby="detail ? messageId : undefined"
      @change="onChange"
    >
    <span class="dropzone__icon" aria-hidden="true">
      <AppIcon name="upload" :size="ICON_SIZE[layout]" />
    </span>
    <label :for="inputId" class="dropzone__text">
      <span class="dropzone__title" :class="layout === 'hero' ? 'text-h4' : 'text-label-strong'">{{ title }}</span>
      <span v-if="detail" :id="messageId" class="dropzone__detail text-body-sm">{{ detail }}</span>
    </label>
    <label v-if="layout === 'inline'" :for="inputId" class="dropzone__action text-label-field" aria-hidden="true">
      {{ actionLabel }}
    </label>
    <label v-else-if="layout === 'hero'" :for="inputId" class="dropzone__pill text-label-button-sm" aria-hidden="true">
      <AppIcon name="plus" :size="16" />
      {{ actionLabel }}
    </label>
  </div>
</template>

<style scoped>
.dropzone {
  position: relative;
  display: flex;
  gap: var(--space-12);
  align-items: center;
  padding: var(--space-14);
  border: var(--border-width-control) dashed var(--color-border);
  border-radius: var(--radius-tile-lg);
  background: var(--color-white);
  transition:
    border-color var(--duration-fast) var(--easing-standard),
    background-color var(--duration-fast) var(--easing-standard);
}

.dropzone:focus-within {
  border-color: var(--color-green-700);
  box-shadow: var(--focus-ring);
}

.dropzone--dragging {
  border-color: var(--color-green-700);
  background: var(--color-green-100);
}

.dropzone--error {
  border-color: var(--color-status-unsuitable);
}

.dropzone__icon {
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-md);
  height: var(--size-icon-tile-md);
  flex-shrink: 0;
  border-radius: var(--radius-tile);
  background: var(--color-sand);
  color: var(--color-text-secondary);
}

.dropzone__text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  cursor: pointer;
}

/* Makes the whole area clickable without nesting interactive elements. */
.dropzone__text::after {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
}

.dropzone__title {
  color: var(--color-text-primary);
}

.dropzone__detail {
  overflow: hidden;
  color: var(--color-text-tertiary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dropzone--error .dropzone__detail {
  color: var(--color-status-unsuitable);
}

.dropzone__action {
  color: var(--color-green-700);
  white-space: nowrap;
  cursor: pointer;
}

.dropzone--disabled,
.dropzone--disabled .dropzone__text,
.dropzone--disabled .dropzone__action {
  cursor: not-allowed;
}

.dropzone--disabled {
  background: var(--color-sand);
}

.dropzone--disabled .dropzone__title,
.dropzone--disabled .dropzone__action {
  color: var(--color-text-tertiary);
}
.dropzone--stacked {
  flex-direction: column;
  gap: var(--space-8);
  padding: var(--space-24);
  text-align: center;
}

.dropzone--stacked .dropzone__icon {
  width: auto;
  height: auto;
  background: transparent;
  color: var(--color-text-primary);
}

.dropzone--stacked .dropzone__text {
  align-items: center;
  gap: var(--space-8);
}

.dropzone--stacked .dropzone__detail {
  white-space: normal;
}
.dropzone--hero {
  flex-direction: column;
  gap: var(--space-10);
  padding: var(--space-28);
  border-color: var(--color-green-700);
  border-radius: var(--radius-lg);
  background: var(--color-green-tint);
  text-align: center;
}

.dropzone--hero .dropzone__icon {
  width: var(--size-icon-tile-xl);
  height: var(--size-icon-tile-xl);
  border-radius: var(--radius-tile-lg);
  background: var(--color-green-800);
  color: var(--color-text-inverse);
}

.dropzone--hero .dropzone__text {
  align-items: center;
  gap: var(--space-10);
}

.dropzone--hero .dropzone__detail {
  white-space: normal;
}

.dropzone--hero.dropzone--dragging {
  background: var(--color-green-100);
}

.dropzone__pill {
  position: relative;
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
  height: var(--size-control-sm);
  padding: 0 var(--space-14);
  border: var(--border-width-control) solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-white);
  color: var(--color-green-800);
  cursor: pointer;
}

.dropzone--disabled .dropzone__pill {
  color: var(--color-text-tertiary);
  cursor: not-allowed;
}
</style>
