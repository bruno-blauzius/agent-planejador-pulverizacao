# Agentes de IA para o Agro com LangChain

Dois projetos práticos de estudo sobre agentes de IA, voltados a problemas reais do campo. Cada projeto tem seu próprio documento com o problema, o desafio e o plano de ação por fases.

| Projeto | Tipo | Documento |
|---|---|---|
| 1. Consultor de Bulas e Boletins Técnicos | RAG | [projeto-1-consultor-bulas.md](./projeto-1-consultor-bulas.md) |
| 2. Planejador de Janela de Pulverização | Agente com ferramentas | [projeto-2-planejador-pulverizacao.md](./projeto-2-planejador-pulverizacao.md) |

---

## Visão geral

**Projeto 1 — Consultor de Bulas e Boletins Técnicos.** Um agente RAG que responde perguntas técnicas sobre defensivos e manejo (registro, dose, carência, intervalo de reentrada) usando apenas uma base de bulas e boletins técnicos, sempre citando a fonte e recusando quando a resposta não estiver na base.

**Projeto 2 — Planejador de Janela de Pulverização.** Um agente com ferramentas que cruza previsão do tempo, condições de aplicação (Delta T, vento, chuva) e o cadastro de talhões para indicar as melhores janelas de pulverização, com confirmação humana antes de registrar qualquer aplicação.

Ao final, os dois se conectam: o planejador consulta o consultor de bulas para verificar a carência antes de sugerir uma janela.

---

## Regras que valem para os dois projetos

1. **Avaliação vem antes do código.** O conjunto de perguntas de teste é montado na primeira semana, não na última. Sem ele, não há como saber se uma mudança melhorou ou piorou o agente.
2. **Registre decisões.** Mantenha uma pasta `docs/adr/` com registros curtos de decisão (contexto, opções, escolha, motivo).
3. **Observabilidade desde o primeiro dia.** LangSmith ou Langfuse ligado desde o início. Ver o rastro de cada chamada de ferramenta e de cada trecho recuperado é a forma mais rápida de entender um agente.
4. **Escopo pequeno e bem feito.** Uma cultura, poucos talhões, poucos documentos. Ampliar depois é fácil; consertar um escopo grande e raso não é.

---

## Ordem de execução e cronograma

O Projeto 2 começa primeiro porque ensina o ciclo do agente (raciocinar → chamar ferramenta → observar → responder) sem a complexidade da ingestão de documentos.

| Semanas | Atividade |
|---|---|
| 1 a 2 | Projeto 2 — Fases 0 a 2 |
| 3 | Projeto 1 — Fase 0 (curadoria), estabilizando o agente do Projeto 2 em paralelo |
| 4 a 7 | Projeto 1 — Fases 1 a 4 |
| 8 a 9 | Projeto 2 — Fases 3 e 4 |
| 10 | Integração entre os dois agentes |

Estimativa de cerca de dez semanas em ritmo de estudo paralelo ao trabalho. Se faltar tempo, corte os extras das Fases 4, nunca a avaliação.

---

## Estrutura sugerida do repositório

```
agentes-agro/
├── README.md
├── docs/
│   ├── adr/                  # registros de decisão
│   └── revisoes/             # revisão ao fim de cada fase
├── consultor-bulas/          # Projeto 1
│   ├── dados/                # documentos brutos e inventário
│   ├── ingestao/
│   ├── recuperacao/
│   ├── agente/
│   └── avaliacao/
└── planejador-pulverizacao/  # Projeto 2
    ├── ferramentas/
    ├── banco/
    ├── agente/
    └── avaliacao/
```

---

## Stack de referência

- **Orquestração:** LangChain 1.x (`create_agent`) e LangGraph para fluxos com interrupção humana
- **Observabilidade e avaliação:** LangSmith ou Langfuse
- **Banco vetorial:** Chroma (estudo) ou pgvector (mais próximo de produção)
- **Banco relacional:** SQLite para começar, Postgres se quiser simular produção
- **Clima:** API Open-Meteo (gratuita)
- **Extração de PDF:** pdfplumber, Docling ou Unstructured (escolha registrada em ADR)

---

## Revisão ao fim de cada fase

Ao concluir cada fase, registre em `docs/revisoes/`:

- O que funcionou
- O que travou e como foi resolvido
- Quais decisões foram tomadas (com link para o ADR)
- Placar da avaliação, quando houver