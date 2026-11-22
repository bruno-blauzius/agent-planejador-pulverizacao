<script setup lang="ts">
/**
 * Profile identity (Figma 54:903): avatar, name, e-mail, role and company tags,
 * photo actions and the "managed by the administrator" note.
 * Presentation only. There is no photo endpoint yet, so the photo actions are
 * shown as unavailable ("Em breve").
 */
withDefaults(
  defineProps<{
    name: string
    email: string
    initials: string
    /** Stable avatar tone (e.g. the user id). */
    seed?: string
    roleLabel?: string | null
    companyLabel?: string | null
    photoUnavailableHint?: string
    managedTitle?: string
    managedText?: string
  }>(),
  {
    seed: undefined,
    roleLabel: null,
    companyLabel: null,
    photoUnavailableHint: 'Em breve',
    managedTitle: 'Gerenciado pelo administrador',
    managedText: 'E-mail, papel e empresa só podem ser alterados por um administrador.',
  },
)

const nameId = useId()
</script>

<template>
  <section class="profile-identity" :aria-labelledby="nameId">
    <div class="profile-identity__avatar">
      <BaseAvatar :initials="initials" :seed="seed" tone="green" size="xl" />
      <span class="profile-identity__badge" aria-hidden="true">
        <AppIcon name="upload" :size="16" />
      </span>
    </div>

    <div class="profile-identity__who">
      <h2 :id="nameId" class="profile-identity__name text-h3">{{ name }}</h2>
      <p class="profile-identity__email text-body-sm">{{ email }}</p>
    </div>

    <ul v-if="roleLabel || companyLabel" class="profile-identity__tags" aria-label="Papel e empresa">
      <li v-if="roleLabel" class="profile-identity__tag profile-identity__tag--green text-label-badge">{{ roleLabel }}</li>
      <li v-if="companyLabel" class="profile-identity__tag text-label-badge">{{ companyLabel }}</li>
    </ul>

    <div class="profile-identity__actions">
      <BaseButton variant="secondary" size="sm" icon="plus" :unavailable-hint="photoUnavailableHint">
        Trocar foto
      </BaseButton>
      <BaseButton variant="ghost" size="sm" :unavailable-hint="photoUnavailableHint">
        Remover
      </BaseButton>
    </div>

    <div class="profile-identity__note">
      <p class="profile-identity__note-title text-label-badge">{{ managedTitle }}</p>
      <p class="profile-identity__note-text text-body-sm">{{ managedText }}</p>
    </div>
  </section>
</template>

<style scoped>
.profile-identity {
  display: flex;
  flex-direction: column;
  gap: var(--space-18);
  align-items: center;
  padding: var(--space-22);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.profile-identity__avatar {
  position: relative;
}

.profile-identity__badge {
  position: absolute;
  right: 0;
  bottom: 0;
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-md);
  height: var(--size-icon-tile-md);
  border: var(--border-width-indicator) solid var(--color-white);
  border-radius: var(--radius-pill);
  background: var(--color-sprout-400);
  color: var(--color-green-950);
}

.profile-identity__who {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  align-items: center;
  min-width: 0;
  max-width: 100%;
  text-align: center;
}

.profile-identity__name {
  margin: 0;
  color: var(--color-text-primary);
  overflow-wrap: anywhere;
}

.profile-identity__email {
  margin: 0;
  color: var(--color-text-tertiary);
  overflow-wrap: anywhere;
}

.profile-identity__tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-8);
  justify-content: center;
  margin: 0;
  padding: 0;
  list-style: none;
}

.profile-identity__tag {
  padding: var(--padding-badge-y) var(--space-10);
  border-radius: var(--radius-sm);
  background: var(--color-sand);
  color: var(--color-text-secondary);
}

.profile-identity__tag--green {
  background: var(--color-green-100);
  color: var(--color-green-800);
}

.profile-identity__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-8);
  justify-content: center;
}

.profile-identity__note {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  align-self: stretch;
  padding: var(--space-12);
  border-radius: var(--radius-md);
  background: var(--color-sand);
}

.profile-identity__note-title,
.profile-identity__note-text {
  margin: 0;
}

.profile-identity__note-title {
  color: var(--color-text-tertiary);
}

.profile-identity__note-text {
  color: var(--color-text-secondary);
}
</style>
