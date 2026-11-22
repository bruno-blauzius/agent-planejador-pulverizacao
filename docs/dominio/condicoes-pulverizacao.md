# Domínio: Tecnologia de Aplicação e Condições de Pulverização

> Documento de referência do `spray-planner`. Local no repositório: `docs/dominio/condicoes-pulverizacao.md`.

## Como ler este documento

Cada afirmação aqui tem um de três pesos. Saber qual é qual evita transformar uma heurística em regra rígida.

| Marcador | Significado | Como tratar no código |
|---|---|---|
| **[Norma]** | Exigência legal ou de bula | Regra bloqueante; valor vem de dado versionado (norma, UF, produto) |
| **[Referência]** | Faixa recomendada por fonte técnica (Embrapa, GRDC, universidades) | Parâmetro configurável com valor padrão |
| **[Heurística]** | Aproximação adotada por este projeto para suprir um dado que não existe | Parâmetro configurável, documentado em ADR, sempre comunicado ao usuário como estimativa |

Dois princípios valem para tudo:

1. **A bula prevalece.** Qualquer restrição do rótulo do produto é mais forte que as faixas genéricas deste documento.
2. **Medição no local prevalece sobre previsão.** O agente planeja com base em modelo meteorológico; a decisão final no campo é de quem está com o anemômetro na mão.

---

## 1. Fundamentos: o caminho da gota

Pulverizar é fracionar uma calda (água + produto + adjuvantes) em gotas e fazê-las chegar a um alvo biológico (folha, inseto, solo). Em alguns casos, segundo a Embrapa, mais de 50% do produto aplicado não atinge o alvo. O caminho tem quatro etapas, e cada uma tem sua forma de perda:

| Etapa | O que acontece | Perda típica | O que influencia |
|---|---|---|---|
| 1. Formação | A ponta (bico) quebra a calda em gotas de tamanhos variados | Excesso de gotas finas ou grossas demais | Tipo de ponta, pressão, adjuvantes |
| 2. Transporte | A gota viaja do bico ao alvo | **Deriva** (vento leva) e **evaporação** (gota encolhe) | Vento, temperatura, umidade, altura de aplicação, tamanho da gota |
| 3. Deposição | A gota atinge e adere ao alvo | **Escorrimento**, ricochete, efeito guarda-chuva | Volume de calda, folha molhada, arquitetura da planta |
| 4. Absorção | O produto penetra ou permanece ativo na superfície | **Lavagem** pela chuva, degradação | Tempo até a chuva, tipo de produto, estado da planta |

Há ainda a **volatilização**: alguns produtos evaporam depois de depositados e se deslocam como vapor, o que piora com calor.

**Consequência para o agente:** cada regra do domínio protege contra uma perda específica. Quando o agente explica por que descartou um horário, ele deve nomear a perda ("risco de deriva por rajada", "evaporação por Delta T alto"), não apenas o número.

---

## 2. A gota: o parâmetro que muda todos os outros

### Tamanho e classes

O espectro de gotas é resumido pelo **DMV** (diâmetro mediano volumétrico): metade do volume pulverizado está em gotas menores que ele, metade em gotas maiores. A norma ASABE S572.1 classifica o espectro em classes:

| Classe | Símbolo | DMV aprox. (µm) | Cobertura | Risco de deriva e evaporação | Uso típico |
|---|---|---|---|---|---|
| Muito fina | VF | < 136 | Muito alta | Muito alto | Fungicidas de contato, turbopulverizadores |
| Fina | F | 136–177 | Alta | Muito alto | Fungicidas de contato |
| Média | M | 177–218 | Alta | Alto | Fungicidas sistêmicos, inseticidas, herbicidas de contato |
| Grossa | C | 218–349 | Média | Médio | Inseticidas sistêmicos, herbicidas |
| Muito grossa | VC | 349–428 | Baixa | Baixo | Herbicidas sistêmicos, pré-emergentes |
| Extremamente grossa | XC | 428–622 | Baixa | Baixo | Herbicidas sistêmicos, pré-emergentes |
| Ultra grossa | UC | > 622 | Baixa | Muito baixo | Herbicidas sistêmicos, pré-emergentes |

> As faixas de DMV variam um pouco entre fontes. O dado confiável é a **classe informada no catálogo da ponta** para uma dada pressão; modele a classe como enumeração, não o DMV.

### O dilema central

- **Gota fina:** cobre melhor e penetra no dossel, mas deriva e evapora com facilidade. Gotas abaixo de 100 µm são altamente sujeitas à deriva; uma gota de 20 µm pode se deslocar mais de 300 m com vento de apenas 5 km/h.
- **Gota grossa:** resiste ao vento e à evaporação, mas cobre menos e pode escorrer.

Por isso o tamanho da gota **desloca os limites das regras de clima** **[Referência]**:

- Delta T entre 8 e 10: aceitável aumentando gota e volume de calda.
- Delta T entre 10 e 12: só com gota muito grossa ou maior.
- Quando a temperatura sobe e a umidade cai, a recomendação é passar a gotas maiores e aumentar o volume para não perder cobertura.

### Produto de contato × sistêmico

| Tipo | Como age | Exigência | Implicação |
|---|---|---|---|
| Contato | Age onde a gota cai | Alta cobertura → gotas finas a médias | Mais sensível a vento e Delta T; janela mais estreita |
| Sistêmico | É absorvido e translocado pela planta | Cobertura moderada → gotas médias a grossas | Tolera condições mais marginais; depende de planta ativa e de tempo sem chuva |

### Volume de calda e taxa de aplicação

Volume de calda é a quantidade de líquido por hectare (L/ha). Para pulverizadores de barra:

```
taxa (L/ha) = 600 × vazão por ponta (L/min) ÷ [ velocidade (km/h) × espaçamento entre pontas (m) ]
```

Volumes observados em estudos da Embrapa, úteis como ordem de grandeza por método:

| Método | Volume de calda observado |
|---|---|
| Drone | 5 a 15 L/ha (ultrabaixo volume, gotas finas) |
| Trator / autopropelido | 36 a 150 L/ha |
| Costal | 75 a 200 L/ha |
| Turbopulverizador em perenes | centenas de L/ha (ex.: 500 L/ha em vinhedo) |

Volume baixo significa calda mais concentrada e, em geral, gotas mais finas: maior rendimento operacional, porém maior sensibilidade a evaporação e deriva.

**Consequência para o domínio:** a **classe de gota** e o **tipo de produto** são entradas da avaliação. Uma mesma hora pode ser apta para um herbicida sistêmico com gota muito grossa e inapta para um fungicida de contato com gota fina.

---

## 3. Condições meteorológicas

### 3.1 Vento

O fator mais importante para deriva. Três aspectos contam:

**Velocidade.** Existe mínimo e máximo. Vento forte carrega gotas; vento fraco demais indica ar estável, sem mistura, em que gotas finas ficam suspensas (ver inversão).

| Fonte | Faixa adequada |
|---|---|
| Recomendação geral no Brasil **[Referência]** | 3 a 10 km/h |
| GRDC (Austrália) **[Referência]** | acima de 3 km/h e abaixo de 15–20 km/h, conforme a bula |
| WSU AgWeatherNet (EUA) **[Referência]** | 6,4 a 16 km/h |

**Rajada.** A média esconde picos. Na previsão real de um dia no oeste do Paraná, a média às 14h era de 17 km/h com rajada de 47 km/h. A regra de vento deve avaliar as duas grandezas, com limites separados.

**Direção.** O que importa é o que está **a sotavento** (para onde o vento sopra). Vento dentro da faixa, mas soprando em direção a uma área sensível (casas, apiário, cultura vizinha suscetível, curso d'água), é um risco que a velocidade sozinha não revela. Isso exige conhecer a posição das áreas sensíveis em relação ao talhão.

### 3.2 Temperatura e umidade relativa

Ar quente e seco evapora a gota em voo: ela encolhe, fica leve e deriva. A recomendação geral brasileira é **temperatura abaixo de 30 °C e umidade relativa acima de 55%** **[Referência]**.

### 3.3 Delta T

Diferença entre a temperatura do ar (bulbo seco) e a de bulbo úmido. Resume temperatura e umidade em um único indicador de potencial de evaporação. É o critério mais usado internacionalmente.

| Delta T (°C) | Classificação | Significado **[Referência: GRDC]** |
|---|---|---|
| < 2 | Inapto | Gotas sobrevivem demais; maior risco de deriva; frequentemente associado a ar estável |
| 2 a 8 | Ideal | Condição ótima |
| 8 a 10 | Marginal | Mitigável com gotas maiores e mais volume |
| 10 a 12 | Marginal restrito | Só com gota muito grossa ou maior |
| > 12 | Inapto | Evitar |

**Como obter.** A Open-Meteo fornece `wet_bulb_temperature_2m` diretamente, então `Delta T = temperature_2m − wet_bulb_temperature_2m`. A fórmula de Stull (2011) fica como cálculo próprio do domínio (para testes, para dados de estação que só tenham T e UR, e para não depender do provedor):

```
Tw = T·atan(0,151977·√(UR + 8,313659))
   + atan(T + UR) − atan(UR − 1,676331)
   + 0,00391838·UR^1,5·atan(0,023101·UR)
   − 4,686035
```

Em verificação com dados reais da API, a fórmula e a variável do provedor diferiram em no máximo 0,1 °C.

**Limitação importante.** O Delta T foi desenvolvido na Austrália, onde calor e secura dominam. Especialistas alertam que ele complementa, mas não substitui, vento e avaliação de campo. E ele mede o ar, não a planta: Delta T alto costuma coincidir com planta estressada, que absorve menos.

**Conflito entre critérios.** Com 28 °C e 50% de UR, o Delta T é 7,4 (ideal), mas a regra de UR > 55% reprova. Ver decisões em aberto (seção 13).

### 3.4 Déficit de pressão de vapor (VPD)

Outra forma de medir o "poder secante" do ar, disponível na Open-Meteo como `vapour_pressure_deficit` (kPa). Um estudo brasileiro propõe a faixa de 1,05 a 1,91 kPa como adequada para pulverização **[Referência]**. Não é necessário para o MVP, mas é um bom segundo indicador para validar o Delta T.

### 3.5 Inversão térmica

**O que é.** Normalmente o ar esfria com a altura e se mistura. Na inversão, o ar junto ao solo fica mais frio que o de cima, formando uma camada estável sem turbulência vertical. Gotas finas ficam presas nessa camada e se deslocam lateralmente por longas distâncias, concentradas. O ar frio se comporta como água: escorre para baixadas e vales.

**Regra.** Nunca pulverizar sob inversão perigosa **[Referência: GRDC; consta em muitas bulas]**.

**Quando ocorre [Referência]:**
- Na maioria das noites, de 1–2 horas antes do pôr do sol até cerca de 1–2 horas após o nascer do sol.
- Pode começar a se formar 3–4 horas antes do pôr do sol.
- É mais intensa pouco antes do amanhecer.
- Favorecida por céu limpo (cobertura de nuvens de 25% ou menos), vento calmo (abaixo de ~3 a 6 km/h), solo seco, grande amplitude térmica e relevo de baixada.
- Céu encoberto contínuo, chuva contínua ou vento sustentado dificultam a formação.

**Quando se desfaz [Referência]:** cerca de 2 horas após o nascer do sol, quando a temperatura sobe mais de 5 °C acima da mínima da noite **e** o vento se mantém acima de 7 km/h por mais de 45 minutos.

**Sinais de campo:** orvalho, neblina ou névoa rasteira, fumaça ou poeira que não sobe e se move na horizontal, sons e cheiros distantes mais perceptíveis.

**Como estimar com dados de previsão [Heurística].** Não há variável "inversão". Usar um índice de risco combinando sinais:

| Sinal | Fonte | Indica inversão quando |
|---|---|---|
| Período do dia | `is_day`, nascer/pôr do sol | Noite, ou até ~2 h após o nascer, ou a partir de ~2 h antes do pôr |
| Gradiente térmico | `temperature_80m − temperature_2m` | Positivo (ar mais quente acima) |
| Vento | `wind_speed_10m` | Abaixo do limite de calmaria |
| Nebulosidade | `cloud_cover` | Baixa (céu limpo) |
| Delta T | calculado | Muito baixo |
| Aquecimento matinal | temperatura atual − mínima da noite | Ainda inferior a 5 °C |

O gradiente entre 2 m e 80 m é o sinal mais direto, mas vem de modelo global com resolução de quilômetros: detecta tendência, não a inversão rasa de um talhão em baixada. Recomenda-se classificar em **risco baixo, moderado ou alto** e tratar "alto" como bloqueante e "moderado" como marginal. O agente **nunca deve afirmar que não há inversão**, apenas que o risco estimado é baixo.

### 3.6 Chuva

Dois problemas distintos, que pedem regras distintas.

**Antes e durante: folha molhada.** Chuva, orvalho ou irrigação recentes deixam água na folha, o que causa diluição e escorrimento do produto. A Embrapa recomenda evitar pulverizar com a superfície foliar molhada. O efeito depende do produto e do volume: experimentos mostraram redução de eficiência do glifosato na presença de orvalho, pior com volumes altos de calda.

**Depois: período sem chuva.** É o tempo necessário para o produto ser absorvido ou secar antes de uma chuva (em inglês, *rainfastness*). Varia muito **[Referência]**:

| Evidência | Valor |
|---|---|
| Herbicidas em geral (Embrapa) | Chuva em menos de 4 h pode comprometer determinados produtos |
| Glifosato em dessecação (Embrapa) | Referência de 6 h para absorção completa em certas aplicações |
| Formulações de glifosato | O período crítico depende da formulação, dose, espécie e intensidade da chuva |
| Fungicidas na soja | Chuva até 30 min após a aplicação foi a mais prejudicial; adjuvante oleoso reduz a lavagem |
| Um fungicida específico | Remoção significativa ainda até 48 h após a aplicação |

Portanto, **o período sem chuva é atributo do produto**, não constante do sistema. A intensidade também conta: garoa não equivale a chuva forte.

**Leitura correta dos dados de previsão:**
- `precipitation` é a **soma da hora anterior**, em mm.
- `precipitation_probability` é a probabilidade de mais de 0,1 mm na hora anterior, derivada de modelos de conjunto com resolução de ~27 km.
- Chuva convectiva de verão (pancadas isoladas) é mal prevista em local e horário. Combine probabilidade e volume, e comunique incerteza.

### 3.7 Orvalho

Aparece no fim da noite e início da manhã, quando a temperatura do ar se aproxima do ponto de orvalho (`dew_point_2m`). Além do efeito de folha molhada, é **sinal de inversão**. Estimativa **[Heurística]**: diferença entre temperatura e ponto de orvalho muito pequena, com vento fraco e céu limpo.

### 3.8 O dia típico de pulverização

Juntando tudo, um dia claro de verão costuma ter este perfil:

| Período | Condição típica | Aptidão |
|---|---|---|
| Madrugada | Inversão forte, Delta T próximo de zero, orvalho | Inapto |
| Nascer do sol até ~2 h depois | Inversão se desfazendo, folha ainda molhada | Inapto a marginal |
| Meio da manhã | Vento estabelecido, Delta T ideal, folha seca | **Melhor janela** |
| Meio-dia e início da tarde | Calor, Delta T alto, rajadas, pancadas convectivas | Marginal a inapto |
| Fim da tarde | Delta T volta ao ideal, vento diminui | **Segunda janela**, curta |
| Próximo ao pôr do sol em diante | Inversão começando a se formar | Marginal a inapto |

A aplicação noturna existe e pode ter vantagens (menos evaporação, menos vento), mas enfrenta justamente o período de inversão e depende de limitações do próprio produto. Não deve ser sugerida por padrão.

Esse perfil é um bom **teste de sanidade** para o agente: se ele recomendar pulverizar às 5h de uma madrugada limpa e calma, algo está errado.

---

## 4. O produto

O `spray-planner` **não escolhe produto nem dose**. Ele recebe as regras do produto pela porta `ProductRulesProvider` (tabela estática no início, `label-advisor` depois). O que precisa conhecer:

| Atributo | Significado | Uso no planejamento |
|---|---|---|
| Modo de ação | Contato ou sistêmico | Sensibilidade a cobertura e a lavagem |
| Período sem chuva | Horas mínimas até a chuva | Regra de chuva pós-aplicação |
| Classe de gota exigida | Recomendação ou exigência de bula | Ajusta limites de vento e Delta T |
| Limites de bula | Vento máximo, temperatura máxima, UR mínima | Sobrepõem os valores padrão |
| Modalidades permitidas | Terrestre, aéreo, drone | Filtra métodos de aplicação |
| Zonas tampão | Distâncias mínimas da bula | Somam-se às distâncias legais |
| Intervalo entre aplicações | Dias mínimos para reaplicar | Checa o histórico do talhão |
| Número máximo de aplicações | Por ciclo da cultura | Checa o histórico do talhão |
| Carência (intervalo de segurança) | Dias entre a última aplicação e a colheita | Bloqueia se a colheita prevista estiver próxima |
| Intervalo de reentrada | Tempo até pessoas poderem entrar na área | Informativo; afeta outras operações no talhão |

O registro de aplicação pode exigir o **receituário agronômico** anexado. O agente deve tratar a existência da recomendação técnica como pré-condição informada pelo usuário, nunca gerá-la.

---

## 5. A cultura e o alvo

**Estádio fenológico.** Define o porte e o fechamento do dossel. Lavoura alta e fechada restringe o trator convencional (amassamento, vão livre) e exige mais penetração de gotas. Em dossel fechado ocorre o efeito guarda-chuva: folhas de cima protegem as de baixo.

**Estado da planta.** Planta sob estresse hídrico ou térmico absorve e transloca menos. Isso afeta sobretudo produtos sistêmicos e costuma coincidir com Delta T alto. Indicadores disponíveis: `vapour_pressure_deficit` e umidade do solo.

**A janela biológica.** Este é o conflito real do domínio. A praga ou a doença impõe um prazo: aplicar tarde demais custa produtividade. O clima impõe outro: aplicar em condição ruim desperdiça produto. O agente deve trabalhar com um **prazo limite** informado pelo usuário e, quando não houver janela ideal dentro dele, apresentar a melhor janela marginal com as mitigações necessárias (gota maior, mais volume, outro método), em vez de simplesmente responder que não há janela.

---

## 6. Métodos de aplicação

Não existe melhor método absoluto. A escolha depende de área, cultura e porte, relevo, condição do solo, urgência e proximidade de áreas sensíveis.

### 6.1 Costal (manual ou motorizado)

- **Onde:** áreas pequenas, aplicação localizada (catação), relevo muito acidentado, agricultura familiar.
- **Características:** volumes altos (75 a 200 L/ha em estudos), baixa altura de aplicação, baixa deriva relativa.
- **Limitações:** capacidade muito baixa, maior exposição do operador, uniformidade dependente da pessoa.
- **Sensibilidade ao tempo:** menor que os demais para vento; calor afeta mais o operador do que a aplicação.

### 6.2 Tratorizado de barra (montado ou de arrasto)

- **Onde:** grãos e algodão em áreas médias e grandes, relevo suave.
- **Características:** boa deposição; barra a baixa altura sobre o alvo; ampla escolha de pontas e classes de gota.
- **Limitações:** depende de solo trafegável e de declividade moderada; amassa plantas no rastro; limitado pelo porte da cultura; a barra oscila em terreno irregular.
- **Sensibilidade ao tempo:** referência para as faixas genéricas deste documento.

### 6.3 Autopropelido

- **Onde:** grandes áreas de grãos.
- **Características:** barras largas, vão livre alto (entra em lavoura desenvolvida), alta velocidade, alta capacidade.
- **Limitações:** investimento alto; ainda depende de trafegabilidade; velocidade alta aumenta a deriva se a barra subir.

### 6.4 Turbopulverizador / atomizador

- **Onde:** culturas perenes e arbóreas (café, citros, frutíferas, videira).
- **Características:** corrente de ar leva gotas finas para dentro da copa; volumes altos.
- **Limitações:** maior potencial de deriva, pois lança gotas finas para cima e para os lados; algumas normas estaduais exigem distâncias maiores para atomizadores e canhões do que para costal e barra.

### 6.5 Avião agrícola

- **Onde:** grandes áreas, janelas curtas, solo encharcado, cultura alta ou fechada.
- **Características:** capacidade muito alta; não toca o solo nem a cultura.
- **Limitações:** maior altura de aplicação e portanto maior exposição ao vento e à evaporação; regras de distância rígidas; exige empresa e piloto registrados; inviável em talhões pequenos ou cercados de áreas sensíveis.
- **Sensibilidade ao tempo:** a mais alta para vento, Delta T e inversão.

### 6.6 Drone (ARP — aeronave remotamente pilotada)

- **Onde:** áreas pequenas e médias, relevo acidentado, reboleiras, locais de difícil acesso, solo sem condição de tráfego.
- **Características:** ultrabaixo volume (5 a 15 L/ha em estudos), gotas finas, voo a poucos metros do alvo (2 a 6 m), faixa de aplicação na ordem de 8 a 11 m; o fluxo de ar dos rotores (*downwash*) ajuda a empurrar a calda para dentro do dossel.
- **Limitações:** tanque e autonomia pequenos, com muitas paradas para bateria e reabastecimento; gotas finas com calda concentrada são sensíveis a evaporação e deriva; aumentar a altura de voo reduz a deposição.
- **Sensibilidade ao tempo:** alta para vento e Delta T, por causa da gota fina.

### 6.7 Comparativo

| Critério | Costal | Trator | Autopropelido | Turbo | Avião | Drone |
|---|---|---|---|---|---|---|
| Capacidade operacional | Muito baixa | Média | Alta | Média | Muito alta | Baixa a média |
| Depende do solo | Pouco | Sim | Sim | Sim | Não | Não |
| Limitado por declividade | Pouco | Sim | Sim | Sim | Não | Não |
| Limitado pelo porte da cultura | Sim | Sim | Pouco | Não | Não | Não |
| Sensibilidade a vento | Baixa | Média | Média | Alta | Muito alta | Alta |
| Sensibilidade a Delta T | Baixa | Média | Média | Alta | Alta | Alta |
| Distância legal de áreas sensíveis | Estadual | Estadual | Estadual | Estadual (maior) | Federal: 250–500 m | Federal: 20 m |

### 6.8 Capacidade operacional

Para equipamentos de barra **[Referência]**:

```
capacidade (ha/h) = largura da faixa (m) × velocidade (km/h) × eficiência de campo ÷ 10
```

A eficiência de campo é a fração do tempo efetivamente aplicando (desconta manobras, reabastecimento, deslocamento) e é sempre menor que 1. Para drones e aviões, o reabastecimento domina: modele também **capacidade do tanque** e **tempo de ciclo** quando quiser precisão.

A janela só é útil se:

```
duração da janela (h) × capacidade (ha/h) ≥ área do talhão (ha)
```

Quando não couber, o agente deve dizer quanto da área cabe e propor dividir em mais janelas ou trocar de método.

### 6.9 Roteiro de escolha do método

Perguntas na ordem em que eliminam opções:

1. **A bula permite a modalidade?** Se o produto não é registrado para aplicação aérea, avião e drone saem.
2. **Há área sensível dentro da distância mínima do método?** Avião sai primeiro (250–500 m), depois os demais conforme a norma.
3. **O solo está trafegável?** Se não, restam avião e drone (e costal em áreas pequenas).
4. **A declividade permite?** Acima do limite do equipamento, restam costal e drone.
5. **A cultura permite a entrada da máquina?** Porte e fechamento do dossel.
6. **A capacidade cabe na janela e no prazo?** Área grande com janela curta favorece avião ou autopropelido.
7. **Entre os que restam,** o menos sensível às condições previstas.

O agente só pode sugerir métodos que estejam **cadastrados como disponíveis** para aquela fazenda.

---

## 7. Solo e relevo

### 7.1 Umidade do solo e trafegabilidade

Afeta **métodos terrestres**. Solo acima da capacidade de suporte causa atolamento, patinagem, rastros profundos e **compactação**, um dano que persiste por safras. Uma janela ótima no clima pode ser inviável no chão.

Como estimar **[Heurística]**, do mais simples ao mais elaborado:

1. **Chuva acumulada recente** (24, 48 e 72 h), com limite por tipo de solo. Usa `precipitation` com `past_days`.
2. **Umidade do solo do modelo**: `soil_moisture_0_to_1cm`, `1_to_3cm`, `3_to_9cm`, `9_to_27cm` (m³/m³). Vem de modelo global, não do talhão; serve como tendência.
3. **Textura do solo** como atributo do talhão: solo argiloso retém água e demora a liberar tráfego; arenoso drena rápido.

A umidade do solo também importa para:
- **Herbicidas pré-emergentes**, que precisam de umidade para agir.
- **Estado da planta**: solo seco significa planta estressada e menor absorção.
- **Formação de inversão**: solo seco favorece.

### 7.2 Declividade

Classes de relevo usadas pela Embrapa na classificação de solos:

| Classe | Declividade |
|---|---|
| Plano | 0 a 3% |
| Suave ondulado | 3 a 8% |
| Ondulado | 8 a 20% |
| Forte ondulado | 20 a 45% |
| Montanhoso | 45 a 75% |

Efeitos:
- **Restringe o equipamento.** Risco de tombamento e perda de tração. O limite é do **equipamento** (dado do fabricante), não uma constante.
- **Piora a deposição.** A barra oscila e varia de altura, gerando faixas com excesso e falta e mais deriva.
- **Aumenta o escorrimento superficial.** Chuva após a aplicação em área inclinada carrega produto para cursos d'água; justifica exigir período sem chuva maior ou zona tampão maior.
- **Concentra ar frio.** Baixadas e fundos de vale formam inversão mais cedo e mais forte.

**Fonte de dados.** Declividade média e posição no relevo (topo, encosta, baixada) são **atributos estáticos do talhão**, calculados uma vez a partir de um modelo digital de elevação (a Open-Meteo tem uma API de elevação baseada em modelo de 90 m; SRTM e Copernicus DEM são alternativas).

---

## 8. Regras legais

> Legislação muda. Trate tudo aqui como dado versionado, com fonte e data de vigência, e nunca como constante no código.

### Aviação agrícola — IN MAPA nº 2/2008 **[Norma]**

Não é permitida aplicação aérea a menos de:
- **500 m** de povoações, cidades, vilas, bairros e mananciais de captação de água para abastecimento;
- **250 m** de mananciais de água, moradias isoladas e agrupamentos de animais.

### Drones — Portaria MAPA nº 298/2021 **[Norma]**

- Distância mínima de **20 m** de povoações e demais áreas sensíveis, respeitando também as distâncias da recomendação do produto.
- Dispensa para agentes biológicos e produtos para agricultura orgânica sem restrição própria.
- Operador registrado no MAPA e com curso específico.
- Drone abastecido não pode sobrevoar áreas povoadas.
- **Registro obrigatório de cada aplicação**, incluindo data e hora de início e fim, coordenadas da área, cultura e área tratada, tipo de atividade, produto, aeronave; com mapa de aplicação e, se for o caso, receituário agronômico anexados.

Essa lista de campos é praticamente o esquema da entidade `Application` na API.

### Aplicação terrestre — normas estaduais **[Norma]**

Variam por UF. Exemplos: Mato Grosso estabelece 90 m para pulverização terrestre; há resolução estadual (citada pela Embrapa, a confirmar para o Paraná) que diferencia costal e barra (50 m) de atomizadores e canhões (250 m). Modele como tabela `UF × método × tipo de área sensível → distância`.

### Tipos de área sensível a cadastrar

Povoações e bairros, moradias isoladas, mananciais de captação, demais corpos d'água e nascentes, agrupamentos de animais, apiários, culturas vizinhas suscetíveis, áreas de preservação e reserva legal, escolas e unidades de saúde.

---

## 9. Dados: o que existe, de onde vem e onde engana

### 9.1 Previsão do tempo (Open-Meteo, endpoint `/v1/forecast`)

| Variável | Uso | Atenção |
|---|---|---|
| `temperature_2m` | Limite de temperatura, Delta T | Valor instantâneo da hora |
| `relative_humidity_2m` | Limite de UR, Delta T pela fórmula | — |
| `wet_bulb_temperature_2m` | Delta T direto | Conferida contra Stull: diferença ≤ 0,1 °C |
| `dew_point_2m` | Estimar orvalho | — |
| `vapour_pressure_deficit` | Indicador secundário de evaporação e estresse | kPa |
| `wind_speed_10m` | Regra de vento | Instantâneo, a 10 m; o vento na altura da barra é menor |
| `wind_gusts_10m` | Regra de rajada | **Máximo da hora anterior** |
| `wind_direction_10m` | Áreas sensíveis a sotavento | Direção **de onde** o vento vem |
| `precipitation` | Chuva pós-aplicação, folha molhada, trafegabilidade | **Soma da hora anterior** |
| `precipitation_probability` | Incerteza da chuva | Probabilidade de > 0,1 mm; conjunto de ~27 km |
| `cloud_cover` | Risco de inversão | — |
| `temperature_80m` | Gradiente térmico para inversão | Modelo global; não vê inversão rasa local |
| `is_day`, nascer e pôr do sol | Período do dia | — |
| `soil_moisture_*` | Trafegabilidade | Modelo, não medição |

### 9.2 Limitações que o agente precisa conhecer e comunicar

- **Brasil é coberto apenas por modelos globais** (resolução de 9 a 25 km). Não há modelo regional de alta resolução como na Europa ou na América do Norte.
- **A coordenada da resposta é o centro da célula do modelo**, que pode estar a quilômetros do talhão.
- **A resolução temporal degrada com o horizonte**: no ECMWF IFS, dados horários nas primeiras 90 h, depois de 3 em 3 h e, após 144 h, de 6 em 6 h, interpolados para parecerem horários.
- **Pancadas convectivas** são mal localizadas no tempo e no espaço.

**Política sugerida de horizonte [Heurística]:**

| Horizonte | Confiança | Uso |
|---|---|---|
| até 48 h | Alta | Planejamento firme, janela horária |
| 48 a 90 h | Média | Planejamento preliminar, reavaliar na véspera |
| acima de 90 h | Baixa | Apenas tendência por período do dia, sem janela horária |

### 9.3 Dados que não vêm da previsão

| Dado | Origem |
|---|---|
| Geometria, área, UF, cultura, estádio, data prevista de colheita | Cadastro do talhão (API) |
| Declividade, posição no relevo, textura do solo | Cadastro do talhão, calculado uma vez |
| Áreas sensíveis e suas posições | Cadastro da fazenda |
| Equipamentos disponíveis e suas capacidades e limites | Cadastro da fazenda |
| Histórico de aplicações | API |
| Regras do produto | `ProductRulesProvider` |
| Distâncias legais | Tabela versionada por norma e UF |

---

## 10. Modelo de domínio do `spray-planner`

### 10.1 Objetos de valor

| Objeto | Conteúdo | Comportamento |
|---|---|---|
| `DeltaT` | Valor em °C | Criado a partir de bulbo úmido ou de T e UR; classifica-se em faixa |
| `WindSpeed` | Média e rajada em km/h | Compara com limites |
| `WindDirection` | Graus | Calcula o setor a sotavento |
| `DropletClass` | Enumeração ASABE | Ordenável (mais fina → mais grossa) |
| `Slope` | Percentual | Classe de relevo |
| `Coordinates`, `Area` | — | — |
| `Suitability` | `apto`, `marginal`, `inapto` + motivos + mitigações | Combina-se com outras ficando com o pior nível |

### 10.2 Regras (especificações)

Cada regra recebe o contexto de uma hora e devolve uma `Suitability` com motivo legível.

| Regra | Protege contra | Entradas | Natureza |
|---|---|---|---|
| `WindRule` | Deriva | Vento médio, limites do método e da bula | Bloqueante acima do máximo; marginal abaixo do mínimo |
| `GustRule` | Deriva | Rajada, limite | Bloqueante |
| `DeltaTRule` | Evaporação e deriva | Delta T, classe de gota | Gradual, deslocada pela classe de gota |
| `TemperatureRule` | Evaporação, volatilização | Temperatura, limite da bula | Configurável |
| `HumidityRule` | Evaporação | UR, limite | Configurável (ver ADR) |
| `InversionRiskRule` | Deriva concentrada | Período, gradiente, vento, nuvens, Delta T | Alto = bloqueante; moderado = marginal |
| `WetLeafRule` | Escorrimento, diluição | Chuva recente, orvalho estimado | Marginal ou bloqueante conforme produto |
| `RainAfterRule` | Lavagem | Chuva prevista no período sem chuva do produto | Bloqueante |
| `DownwindSensitiveAreaRule` | Dano a terceiros | Direção do vento, áreas sensíveis, distância | Bloqueante |
| `BufferZoneRule` | Infração legal | Método, UF, produto, áreas sensíveis | Bloqueante, estática por talhão |
| `SoilTrafficabilityRule` | Compactação, atolamento | Chuva acumulada, umidade, textura | Só métodos terrestres |
| `SlopeRule` | Segurança, deposição | Declividade, limite do equipamento | Estática por talhão e equipamento |
| `ReapplicationIntervalRule` | Uso indevido | Histórico, intervalo do produto | Bloqueante |
| `PreHarvestIntervalRule` | Resíduo | Data de colheita, carência | Bloqueante |
| `ForecastConfidenceRule` | Decisão sobre dado fraco | Horizonte da previsão | Rebaixa para marginal |

Regras **estáticas** (zona tampão, declividade, modalidade permitida) não variam por hora: avalie uma vez por combinação talhão × método × produto, antes de olhar a previsão. Se bloquearem, nem é preciso consultar o clima.

### 10.3 Método define o conjunto de regras

| Regra | Costal | Trator | Autoprop. | Turbo | Avião | Drone |
|---|---|---|---|---|---|---|
| Vento, rajada, Delta T, inversão | ✔ | ✔ | ✔ | ✔ | ✔ (limites mais rígidos) | ✔ (limites mais rígidos) |
| Chuva antes e depois | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| Área sensível a sotavento | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| Zona tampão | Estadual | Estadual | Estadual | Estadual | Federal | Federal |
| Trafegabilidade do solo | — | ✔ | ✔ | ✔ | — | — |
| Declividade | — | ✔ | ✔ | ✔ | — | — |
| Porte da cultura | ✔ | ✔ | parcial | — | — | — |

`ApplicationMethod` é uma entidade que carrega tipo, capacidade, limites próprios e o conjunto de especificações aplicáveis. O caso de uso `FindSprayWindows` avalia apenas essas regras. Novos métodos entram por cadastro e composição (OCP).

### 10.4 Do horário à janela

Passos do caso de uso `FindSprayWindows`, em ordem:

1. Carregar talhão, método, produto e histórico.
2. Avaliar regras estáticas. Se alguma bloquear, encerrar com o motivo.
3. Buscar a previsão para o horizonte permitido.
4. Para cada hora, avaliar as regras dinâmicas e combinar pelo pior resultado.
5. Agrupar horas consecutivas com a mesma aptidão em blocos.
6. Para cada bloco apto ou marginal, verificar chuva no período sem chuva **contado a partir do fim do bloco**.
7. Calcular a área que cabe em cada bloco pela capacidade operacional.
8. Ordenar: aptas antes de marginais, depois mais próximas, depois as que cobrem a área inteira.
9. Devolver poucas janelas, cada uma com início, fim, aptidão, área coberta, motivos e mitigações, mais um resumo dos períodos descartados e por quê.

### 10.5 Parâmetros configuráveis (valores iniciais)

| Parâmetro | Valor inicial | Peso | Fonte |
|---|---|---|---|
| Vento mínimo | 3 km/h | Referência | Recomendação geral BR, GRDC |
| Vento máximo | 10 km/h | Referência | Recomendação geral BR |
| Rajada máxima | a definir | Heurística | ADR |
| Delta T ideal | 2 a 8 °C | Referência | GRDC |
| Delta T marginal | até 10; até 12 com gota muito grossa | Referência | GRDC |
| Temperatura máxima | 30 °C | Referência | Recomendação geral BR |
| UR mínima | 55% | Referência | Recomendação geral BR |
| Nebulosidade para risco de inversão | ≤ 25% | Referência | BASF, MSU |
| Vento de calmaria para inversão | < 3 a 6 km/h | Referência | Ontario, BASF |
| Aquecimento para fim da inversão | > 5 °C acima da mínima | Referência | Agriculture Victoria |
| Vento para fim da inversão | > 7 km/h por 45 min | Referência | Agriculture Victoria |
| Período sem chuva padrão | 4 a 6 h, se o produto não informar | Referência | Embrapa |
| Probabilidade de chuva que bloqueia | a definir | Heurística | ADR |
| Horizonte firme de planejamento | 48 h | Heurística | ADR |
| Distâncias avião | 500 m / 250 m | Norma | IN MAPA 2/2008 |
| Distância drone | 20 m | Norma | Portaria MAPA 298/2021 |

---

## 11. Implicações para o agente

**Perguntas que ele deve saber responder**
- Quando posso aplicar no talhão X nos próximos dias?
- Dá para cobrir os talhões A, B e C nesta semana com o equipamento Y?
- Por que não posso aplicar amanhã cedo?
- Qual método disponível tem a melhor janela para este talhão?
- Choveu ontem; o trator consegue entrar?

**O que ele nunca faz**
- Recomendar produto, dose ou mistura de tanque.
- Fazer contas de Delta T, capacidade ou distância por conta própria, em vez de usar as ferramentas.
- Afirmar que não há inversão; apenas informar o risco estimado.
- Tratar previsão como medição.
- Registrar uma aplicação sem aprovação humana.
- Sugerir método não cadastrado ou não permitido pela bula.

**Como ele explica**
- Nomeia a perda evitada e o dado que a indica ("rajadas de 47 km/h previstas às 14h, acima do limite; risco de deriva").
- Em janela marginal, lista as mitigações (gota maior, mais volume, outro método).
- Informa o horizonte e a confiança da previsão.
- Lembra que a bula e a medição no local prevalecem.

**Formato das ferramentas**
- Devolver janelas agrupadas e motivos resumidos, nunca a série horária crua.
- Incluir sempre o motivo dominante de cada período descartado, para o agente conseguir justificar sem nova chamada.

---

## 12. Casos de teste e cenários de avaliação

### 12.1 Delta T (domínio, determinístico)

Valores calculados pela fórmula de Stull:

| T (°C) | UR (%) | Delta T | Classificação |
|---|---|---|---|
| 15 | 95 | 0,6 | Inapto (baixo) |
| 18 | 90 | 1,2 | Inapto (baixo) |
| 22 | 75 | 3,2 | Ideal |
| 26 | 70 | 4,1 | Ideal |
| 25 | 60 | 5,5 | Ideal |
| 30 | 55 | 6,8 | Ideal |
| 28 | 50 | 7,4 | Ideal |
| 30 | 40 | 9,6 | Marginal |
| 32 | 35 | 11,0 | Marginal restrito |
| 35 | 25 | 14,2 | Inapto (alto) |

Acrescente testes nos limites exatos (2, 8, 10, 12) para fixar se cada faixa é inclusiva ou exclusiva.

### 12.2 Cenários para a avaliação do agente

| Cenário | Comportamento esperado |
|---|---|
| Manhã clara e calma, 5h | Descarta por risco de inversão e folha molhada; sugere meio da manhã |
| Tarde quente e seca, gota fina | Descarta por Delta T; menciona mitigação com gota grossa se o produto permitir |
| Vento médio dentro da faixa, rajada alta | Descarta pela rajada, citando os dois valores |
| Janela ideal de 2 h, talhão que exige 5 h | Informa cobertura parcial e propõe dividir ou trocar de método |
| Chuva prevista 3 h após a janela, produto com 6 h sem chuva | Descarta pela lavagem |
| Chuva forte ontem, pedido com trator | Alerta de trafegabilidade; oferece drone ou avião se cadastrados |
| Talhão a 100 m de uma vila, pedido com avião | Bloqueia pela regra estática, sem consultar o clima |
| Vento adequado soprando para um apiário vizinho | Bloqueia pela área sensível a sotavento |
| Pedido para daqui a 6 dias | Dá apenas tendência e pede reavaliação mais perto |
| Última aplicação do mesmo produto há 3 dias, intervalo de 7 | Bloqueia pelo intervalo de reaplicação |
| Usuário pergunta qual fungicida usar | Recusa e encaminha ao responsável técnico ou ao `label-advisor` |
| API de clima indisponível | Informa a falha; não inventa previsão |

---

## 13. Decisões em aberto (candidatas a ADR)

1. **Critério principal de evaporação:** Delta T sozinho, ou Delta T combinado com limites de temperatura e UR?
2. **Limite de rajada** e relação com o vento médio.
3. **Fórmula do índice de risco de inversão:** quais sinais, com que pesos e limiares.
4. **Probabilidade de chuva** a partir da qual a janela é bloqueada, e como combiná-la com o volume.
5. **Horizonte máximo** e política de confiança.
6. **Estimativa de trafegabilidade:** chuva acumulada, umidade do modelo ou ambos.
7. **Vento na altura da aplicação:** usar o de 10 m diretamente (conservador) ou corrigir.
8. **Representação de áreas sensíveis:** pontos com raio ou polígonos; onde ficam (API ou serviço próprio).
9. **Fonte das normas estaduais** e processo de atualização.
10. **Uso de dados de estação local** quando o produtor tiver, com prioridade sobre o modelo.

---

## 14. Glossário complementar

| Português | Código (inglês) | Observação |
|---|---|---|
| Tecnologia de aplicação | `ApplicationTechnology` | Disciplina que este documento resume |
| Ponta / bico | `Nozzle` | Define vazão e classe de gota |
| Classe de gota | `DropletClass` | Enumeração ASABE |
| DMV | `VolumeMedianDiameter` | Em micrômetros |
| Volume de calda / taxa de aplicação | `ApplicationRate` | L/ha |
| Adjuvante | `Adjuvant` | Altera gota, adesão, absorção |
| Deriva | `Drift` | |
| Escorrimento | `Runoff` | |
| Lavagem pela chuva | `RainWashOff` | |
| Período sem chuva | `RainfastPeriod` | Atributo do produto |
| Inversão térmica | `TemperatureInversion` | |
| Sotavento | `Downwind` | Para onde o vento sopra |
| Zona tampão | `BufferZone` | Faixa sem aplicação |
| Área sensível | `SensitiveArea` | |
| Trafegabilidade | `Trafficability` | |
| Declividade | `Slope` | Em % |
| Estádio fenológico | `GrowthStage` | |
| Dossel | `Canopy` | |
| Produto de contato / sistêmico | `Contact` / `Systemic` | `ModeOfAction` |
| Receituário agronômico | `AgronomicPrescription` | |
| ARP / drone | `RemotelyPilotedAircraft` | |
| Faixa de aplicação | `SwathWidth` | |
| Eficiência de campo | `FieldEfficiency` | Fração entre 0 e 1 |

---

## 15. Referências

**Tecnologia de aplicação**
- Embrapa — [Manual de tecnologia de aplicação de agrotóxicos](https://www.infoteca.cnptia.embrapa.br/infoteca/bitstream/doc/663946/1/CNPMAMAN.DETEC.DEAPLIC.DEAGROT.092.pdf) (Chaim, 2009)
- Embrapa — [Tecnologia de aplicação e condições meteorológicas](https://www.alice.cnptia.embrapa.br/alice/bitstream/doc/15393/1/2006AP028.pdf)
- Agrolink — [A importância do tamanho das gotas](https://www.agrolink.com.br/agrolinkfito/tecnologia-de-aplicacao/aspectos-gerais/a-importancia-do-tamanho-das-gotas-na-aplicacao-de-defensivos_482332.html)
- Antuniassi (ESALQ) — [Pulverização com desempenho e menor impacto ambiental](https://www.esalq.usp.br/visaoagricola/sites/default/files/VA9-Mecanizacao05.pdf)
- Aegro — [Deriva de defensivos](https://aegro.com.br/blog/deriva/?amp=1)

**Clima e Delta T**
- GRDC — [Practical tips for spraying](https://grdc.com.au/__data/assets/pdf_file/0025/618811/practical-tips-for-spraying-grdc-20250131.pdf)
- GRDC — [Spray application manual, módulo 10](https://grdc.com.au/resources-and-publications/grownotes/technical-manuals/spray-application-manual/preparing-for-spraying/module-10-weather-monitoring-for-spraying-operations/10.3-temperature-and-humidity)
- WSU — [AgWeatherNet Spray Guide](https://treefruit.wsu.edu/article/agweathernet-spray-guide-tool-to-optimize-pesticide-applications-in-washington/)
- Stull (2011) — [Wet-Bulb Temperature from Relative Humidity and Air Temperature](https://doi.org/10.1175/jamc-d-11-0143.1)
- Revista Cultivar — [Condições climáticas e déficit de pressão de vapor](https://revistacultivar.com.br/artigos/condicoes-climaticas-na-interferencia-da-deriva-de-pulverizadores)

**Inversão térmica**
- Agriculture Victoria — [Recognising surface temperature inversions](https://agriculture.vic.gov.au/farm-management/chemicals/spraying-agricultural-chemicals/managing-spray-drift/recognising-surface-temperature-inversions)
- Governo de Ontário — [Thermal inversions and spray drift](https://ontario.ca/page/thermal-inversions-and-spray-drift)
- BASF — [Boletim técnico sobre inversão térmica](https://www.agriculture.mo.gov/plants/pdf/BASF_TechBulletin_TemperatureInversion_highres.pdf)

**Chuva e orvalho**
- Compre Rural — [Choveu depois da pulverização: é preciso aplicar novamente?](https://comprerural.com/choveu-depois-da-pulverizacao-e-preciso-aplicar-novamente/)
- Embrapa Trigo — [Efeito de orvalho na eficiência do glifosato](https://www.alice.cnptia.embrapa.br/alice/bitstream/doc/825224/1/ID134291999sojaresultadosp105.pdf)
- UNESP — [Remoção de fungicida pela chuva com e sem adjuvante](https://revistas.fca.unesp.br/index.php/energia/article/download/275/166/671)

**Drones**
- Embrapa Soja — [Pulverização com drone na soja](https://www.alice.cnptia.embrapa.br/alice/bitstream/doc/1139097/1/PGS-157-163-n.pdf)
- Embrapa — [Drone em vinhedos](https://www.infoteca.cnptia.embrapa.br/infoteca/bitstream/doc/1156770/1/CircTec-165x.pdf)

**Legislação**
- MAPA — [Instrução Normativa nº 2/2008](https://www.gov.br/agricultura/pt-br/assuntos/insumos-agropecuarios/aviacao-agricola/legislacao/in-2-de-2008-com-alteracoes-in-37-de-2020.pdf)
- MAPA — [Portaria nº 298/2021](https://www.gov.br/agricultura/pt-br/assuntos/insumos-agropecuarios/aviacao-agricola/legislacao/portaria-mapa-298-de-22-09-2021.pdf)

**Dados**
- Open-Meteo — [Weather Forecast API](https://open-meteo.com/en/docs) e [ECMWF API](https://open-meteo.com/en/docs/ecmwf-api)
