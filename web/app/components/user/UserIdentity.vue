<script setup lang="ts">
/**
 * User cell of the users table (Figma 52:410): avatar with initials, name and
 * e-mail.
 */
defineProps<{
  id: string
  name: string
  email: string
  initials: string
}>()
</script>

<template>
  <span class="user-identity">
    <BaseAvatar :initials="initials" :seed="id" size="md" />
    <span class="user-identity__text">
      <span class="user-identity__name text-label-strong">{{ name }}</span>
      <span class="user-identity__email text-body-sm">{{ email }}</span>
    </span>
  </span>
</template>

<style scoped>
.user-identity {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  min-width: 0;
}

.user-identity__text {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.user-identity__name,
.user-identity__email {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-identity__name {
  color: var(--color-text-primary);
}

.user-identity__email {
  color: var(--color-text-tertiary);
}
</style>
