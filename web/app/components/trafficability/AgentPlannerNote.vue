<script setup lang="ts">
/**
 * "O que vai para o Planejador" (Figma "Leitura do agente", 84:1594), on the
 * library's AI agent alert. A fixed sentence that explains how the planner uses
 * this view; no logic and no generated text.
 */
</script>

<template>
  <ToastAlert
    tone="agent"
    title="O que vai para o Planejador"
    message="O planejador descarta métodos inaptos antes de procurar janelas de clima."
    :dismissible="false"
  />
</template>
