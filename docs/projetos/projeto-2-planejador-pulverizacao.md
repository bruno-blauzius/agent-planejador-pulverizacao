# Projeto 2 — Planejador de Janela de Pulverização (Agente com ferramentas)

[← Voltar ao README](README.md)

## O problema

Aplicar defensivo na hora errada é desperdício de dinheiro e risco ambiental. Vento forte causa deriva, chuva logo depois lava o produto, umidade baixa e temperatura alta evaporam a calda (Delta T fora da faixa). Hoje o operador decide olhando o céu ou vários aplicativos separados, e ainda precisa cruzar isso com o estádio da cultura e com o que já foi aplicado em cada talhão.

## O objetivo

Construir um agente que, dado um conjunto de talhões, indique as melhores janelas de pulverização dos próximos dias, justificando cada recomendação.

> Exemplo: *"Preciso aplicar fungicida nos talhões 3 e 7 essa semana. Quando faço?"*

## Ferramentas do agente

| Ferramenta | Função |
|---|---|
| Previsão do tempo | Dados horários por coordenada via Open-Meteo (vento, rajada, temperatura, umidade, probabilidade e volume de chuva) |
| Calculadora de aptidão | Recebe os dados horários e indica se a hora/janela está apta e o motivo quando não está |
| Cadastro de talhões | Banco com área, cultura, coordenadas, data de plantio e histórico de aplicações |
| Regras do produto | Tabela com intervalo mínimo entre aplicações e chuva mínima sem lavagem (futuramente, o [Projeto 1](./docs/projeto-1-consultor-bulas.md)) |

## Dificuldades esperadas

- Fazer o modelo escolher a ferramenta certa, na ordem certa
- Evitar que o modelo "calcule de cabeça" em vez de usar a calculadora
- Lidar com falhas da API de clima
- Controlar o tamanho do contexto quando a previsão horária de vários dias vier inteira
- Interpretar perguntas vagas ("essa semana", "os talhões de soja")

---

## Plano de ação

### Fase 0 — Fundação (2 a 3 dias)

**Objetivo:** ambiente pronto e domínio entendido antes de programar.

- [ ] Criar o repositório separando ferramentas, agente, dados e avaliação
- [ ] Configurar variáveis de ambiente, gerenciador de dependências e LangSmith/Langfuse
- [ ] Estudar o domínio: Delta T, faixas de vento aceitáveis, tempo sem chuva necessário após aplicação
- [ ] Definir parâmetros iniciais como **configuráveis**, não fixos no código (referência inicial: Delta T entre 2 e 8 °C, vento entre 3 e 10 km/h)
- [ ] Escrever um documento de uma página com o escopo: o que o agente faz e o que **não** faz (ex.: não recomenda produto, não decide dose)

**Entregável:** repositório rodando um "olá mundo" com um modelo via LangChain e o rastro aparecendo na ferramenta de observabilidade.

---

### Fase 1 — Ferramentas determinísticas (1 semana)

**Objetivo:** construir e testar as ferramentas sem nenhum agente envolvido.

- [ ] Cliente de clima: consumir a API do Open-Meteo e devolver dados horários já limpos
- [ ] Calculadora de aptidão, incluindo o cálculo de Delta T a partir de temperatura e umidade
- [ ] Testes unitários da calculadora com casos conhecidos: vento forte, chuva em duas horas, Delta T alto, hora perfeita

**Decisão a tomar:** a calculadora devolve hora a hora ou já agrupa em janelas contínuas ("das 6h às 10h apto")? Recomendação: agrupar, porque reduz bastante o contexto enviado ao modelo. Registrar em ADR.

**Entregável:** as duas ferramentas funcionando isoladas, com testes passando.

---

### Fase 2 — Primeiro agente, um talhão (1 semana)

**Objetivo:** o agente responde "quando posso pulverizar neste ponto?" usando as ferramentas.

- [ ] Transformar as funções em ferramentas do LangChain com nomes e descrições claras (a descrição da ferramenta funciona como um prompt; reescreva até o modelo escolhê-la corretamente)
- [ ] Montar o agente com `create_agent` (LangChain 1.x) e um prompt de sistema curto que defina o papel e proíba avaliar condições sem usar a calculadora
- [ ] Montar o primeiro conjunto de avaliação: 10 perguntas com o comportamento esperado (qual ferramenta deve ser chamada, o que a resposta deve conter)

**Risco principal:** o modelo responder sobre o clima sem chamar a ferramenta ou fazer contas por conta própria. Verificar no rastro.

**Entregável:** agente respondendo para uma coordenada, com a avaliação rodando.

---

### Fase 3 — Talhões e banco de dados (1 semana)

**Objetivo:** trabalhar com vários talhões do cadastro.

- [ ] Modelar o banco (SQLite para começar, Postgres para simular produção): talhões, culturas, histórico de aplicações, parâmetros do pulverizador
- [ ] Criar dados de teste realistas para 5 a 10 talhões
- [ ] Criar ferramentas de consulta específicas ("buscar talhões por cultura", "última aplicação do talhão")
- [ ] Ampliar a avaliação com perguntas sobre vários talhões e perguntas vagas

**Decisão importante:** não dar ao agente uma ferramenta que execute SQL livre. Ferramentas específicas são mais seguras e o modelo erra menos.

**Entregável:** agente respondendo perguntas que envolvem vários talhões.

---

### Fase 4 — Restrições e confirmação humana (1 semana)

**Objetivo:** o agente respeita regras de negócio e não grava nada sem aprovação.

- [ ] Implementar regras: intervalo mínimo desde a última aplicação e capacidade operacional (hectares por hora) para saber se a janela comporta a área
- [ ] Criar a ferramenta que registra a aplicação planejada, protegida por interrupção para aprovação humana (suporte nativo no LangGraph)
- [ ] Tratar falhas: API de clima fora do ar deve gerar resposta honesta, não alucinação

**Entregável:** fluxo completo, do pedido à aplicação registrada com aprovação.

---

## Definição de pronto

- [ ] Cenário com 5 talhões e previsão real gera janelas coerentes
- [ ] O agente explica por que descartou certos horários
- [ ] Nenhuma regra cadastrada é violada nos testes
- [ ] Avaliação com pelo menos 25 casos rodando automaticamente
- [ ] Calculadora coberta por testes unitários e determinística

## Extras para subir o nível

- Memória entre conversas (lembrar preferências do operador)
- Priorização entre talhões quando a janela não comporta todos
- Integração com o [Projeto 1](./docs/projeto-1-consultor-bulas.md) para checar carência antes de sugerir a aplicação, formando um sistema multiagente