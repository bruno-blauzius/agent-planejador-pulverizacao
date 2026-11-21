# Projeto 1 — Consultor de Bulas e Boletins Técnicos (RAG)

[← Voltar ao README](./README.md)

## O problema

O produtor e o técnico de campo precisam tomar decisões rápidas sobre defensivos e manejo: qual produto é registrado para determinada praga naquela cultura, qual a dose, o período de carência, o intervalo de reentrada e as restrições por estado. Essa informação existe, mas está espalhada em bulas em PDF, no AGROFIT (MAPA), em boletins da Embrapa e em recomendações de fundações de pesquisa. Errar aqui custa caro: resíduo acima do limite, uso de produto não registrado, perda de eficácia.

## O objetivo

Construir um agente que responda perguntas técnicas em linguagem natural usando **apenas** a base documental, sempre citando a fonte (documento e página).

> Exemplo: *"Posso usar o produto X contra lagarta-do-cartucho no milho safrinha? Qual a dose e a carência?"*

## Dificuldades esperadas

- PDFs com tabelas mal extraídas
- Nomes comerciais diferentes para o mesmo princípio ativo
- Perguntas compostas (alvo + cultura + estado)
- Sinônimos regionais de pragas (ex.: "lagarta-do-cartucho" vs. *Spodoptera frugiperda*)
- Documentos desatualizados convivendo com versões novas

> **Atenção:** neste projeto o trabalho pesado não é o agente, é o dado. Planeje gastar metade do tempo na ingestão.

---

## Plano de ação

### Fase 0 — Recorte e curadoria (3 a 4 dias)

**Objetivo:** definir um escopo pequeno e montar a base.

- [ ] Escolher **uma cultura** (milho ou soja, pela quantidade de material disponível) e um tipo de alvo (ex.: pragas)
- [ ] Baixar de 20 a 30 bulas e de 5 a 10 boletins técnicos
- [ ] Criar uma planilha de inventário com origem e data de cada documento
- [ ] **Antes de qualquer código**, escrever as 20 perguntas de avaliação com resposta esperada e página de origem, incluindo 5 perguntas cuja resposta **não** está na base

**Entregável:** pasta de documentos, inventário e conjunto de avaliação.

---

### Fase 1 — Ingestão e extração (1,5 semana)

**Objetivo:** transformar PDFs em texto confiável, com tabelas preservadas.

- [ ] Testar pelo menos dois extratores de PDF em três bulas diferentes e comparar visualmente o resultado (pdfplumber, Docling, Unstructured)
- [ ] Registrar a escolha do extrator em um ADR
- [ ] Tratar tabelas de dose como unidade própria: cada linha (cultura, alvo, dose, carência) deve virar um trecho autocontido ou ficar inteira dentro de um trecho
- [ ] Extrair metadados por documento: cultura, princípio ativo, nome comercial, classe, fabricante, data
- [ ] Inspecionar manualmente uma amostra do resultado

**Risco principal:** subestimar esta fase. Se a extração estiver ruim, nada que vier depois conserta.

**Entregável:** documentos processados em texto estruturado, com metadados.

---

### Fase 2 — Indexação e recuperação (1 semana)

**Objetivo:** a busca traz os trechos certos, medida de forma objetiva.

- [ ] Escolher o banco vetorial (Chroma para estudo, pgvector para algo próximo de produção)
- [ ] Escolher um modelo de embeddings que funcione bem em português
- [ ] Testar duas estratégias de chunking
- [ ] Medir **só a recuperação**, sem gerar resposta: o trecho correto aparece entre os 5 primeiros resultados?
- [ ] Implementar filtro por metadados (cultura, princípio ativo) antes da busca vetorial

**Entregável:** relatório curto comparando as estratégias, com a taxa de acerto da recuperação.

---

### Fase 3 — Geração com citação e recusa (1 semana)

**Objetivo:** respostas corretas, citadas e honestas.

- [ ] Montar a cadeia de resposta com prompt que exija citação de documento e página em toda afirmação técnica
- [ ] Implementar a recusa: se os trechos recuperados não sustentam a resposta, o agente diz que não encontrou
- [ ] Rodar a avaliação completa medindo separadamente:
  - acerto da resposta
  - presença de citação
  - recusa correta nas 5 perguntas sem resposta

**Entregável:** primeira versão completa com o placar da avaliação.

---

### Fase 4 — Transformar em agente e melhorar (1 semana)

**Objetivo:** sair de uma cadeia fixa para um agente que decide como buscar.

- [ ] Transformar a busca em ferramenta e deixar o agente decidir quando buscar, reformular a pergunta ou fazer mais de uma busca (útil em perguntas compostas)
- [ ] Escolher **uma** melhoria, implementar e medir:
  - busca híbrida (palavra-chave + vetor), ótima para nomes comerciais
  - re-ranking dos trechos recuperados
  - expansão de sinônimos de pragas
- [ ] Manter a melhoria só se o placar subir

**Entregável:** agente final com placar comparado à versão da Fase 3.

---

## Definição de pronto

- [ ] Maioria das 20 perguntas respondidas corretamente
- [ ] Citação presente em todas as respostas
- [ ] Recusa correta nas 5 perguntas sem resposta
- [ ] Processo de ingestão documentado a ponto de adicionar uma bula nova sem depender de memória

## Extras para subir o nível

- Avaliador automático com LangSmith ou Langfuse
- Reformulação da pergunta antes da busca
- Ampliar para uma segunda cultura
- Expor o agente como ferramenta para o [Projeto 2](./projeto-2-planejador-pulverizacao.md) consultar carência