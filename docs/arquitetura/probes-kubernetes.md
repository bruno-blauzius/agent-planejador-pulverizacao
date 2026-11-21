# Probes de Kubernetes da API

A `services/api` expõe três endpoints de saúde, cada um com uma pergunta diferente. Decisão registrada no [ADR 0008](../adr/0008-api-go-stdlib-primeiro.md).

| Endpoint | Probe | Pergunta | Fica down quando |
|---|---|---|---|
| `GET /health/startup` | `startupProbe` | A inicialização terminou? | Enquanto a API ainda está inicializando |
| `GET /health/live` | `livenessProbe` | O processo está vivo? | Nunca consulta dependências; só falha se o processo não responde |
| `GET /health/ready` | `readinessProbe` | Posso receber tráfego? | Ao iniciar, no shutdown (`BeginShutdown`) e se uma dependência falhar |

## Por que liveness não checa dependências

Quando o liveness falha, o Kubernetes **reinicia o contêiner**. Se ele consultasse o Postgres ou um serviço externo e essa dependência caísse, todos os pods falhariam juntos e seriam reiniciados em loop, sem resolver nada e piorando a recuperação (cold start, conexões perdidas). Reiniciar não conserta dependência externa.

Dependência fora do ar é caso de **readiness**: o pod sai do balanceamento, continua vivo e volta sozinho quando a dependência se recupera. Liveness responde apenas "o processo travou?".

## Startup e shutdown

- O `startupProbe` suspende liveness e readiness até passar, evitando que uma inicialização lenta seja morta pelo liveness.
- No SIGTERM, a API chama `BeginShutdown` (readiness passa a falhar) e depois `http.Server.Shutdown`, que espera as requisições em andamento por até `API_SHUTDOWN_TIMEOUT` (padrão 15s).
- O `preStop` com `sleep` curto dá tempo para o Kubernetes propagar a remoção do pod dos endpoints antes de a API começar a encerrar.
- `terminationGracePeriodSeconds` deve cobrir `preStop` + `API_SHUTDOWN_TIMEOUT` com folga: 5s + 15s + margem = 30s.

## Exemplo de Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  replicas: 2
  selector:
    matchLabels:
      app: api
  template:
    metadata:
      labels:
        app: api
    spec:
      terminationGracePeriodSeconds: 30   # preStop (5s) + API_SHUTDOWN_TIMEOUT (15s) + margem
      containers:
        - name: api
          image: api:latest
          ports:
            - containerPort: 8080
          env:
            - name: API_PORT
              value: "8080"
            - name: API_SHUTDOWN_TIMEOUT
              value: "15s"
            - name: API_PROBE_TIMEOUT
              value: "2s"
            - name: API_DOCS_ENABLED
              value: "false"
          lifecycle:
            preStop:
              sleep:
                seconds: 5   # ação nativa (Kubernetes 1.30+); a imagem distroless não tem binário sleep
          startupProbe:
            httpGet:
              path: /health/startup
              port: 8080
            periodSeconds: 2
            failureThreshold: 30          # até 60s para inicializar
          livenessProbe:
            httpGet:
              path: /health/live
              port: 8080
            periodSeconds: 10
            timeoutSeconds: 2
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /health/ready
              port: 8080
            periodSeconds: 5
            timeoutSeconds: 3             # maior que API_PROBE_TIMEOUT (2s)
            failureThreshold: 2
```

> **Nota sobre o `preStop`:** a imagem final é `distroless/static`, que não tem shell nem `sleep`, por isso o exemplo usa a ação nativa `sleep`, disponível no Kubernetes 1.30+. Em clusters mais antigos, ajuste a imagem para incluir um binário `sleep`.
