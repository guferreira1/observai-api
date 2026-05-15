# Arquitetura

O ObservAI API é um backend em Go com arquitetura hexagonal que adiciona uma camada
de análise com IA sobre sistemas de observabilidade e armazenamento operacional.

Todo detalhe de provedor e de framework fica nos adapters.  
O domínio central não importa pacotes de adapters, apenas bibliotecas padrão e
pacotes internos de domínio.

## Princípios de projeto

- **Domínio agnóstico a provedor**
  - Modelos únicos para sinais, análises e recomendações.
- **Direção de dependência correta**
  - Adaptadores de entrada/saída dependem do core; o core não depende dos adapters.
- **Configuração e recarga dinâmica**
  - Provedores configurados via admin podem ser alterados sem reiniciar a aplicação.
- **Processamento assíncrono**
  - Job de análise desacopla o tempo de resposta HTTP do processamento de LLM.
- **Saídas por políticas**
  - Severidade e recomendações estão centralizadas no domínio.

## Camadas

```text
           ┌──────────────────────────────┐
           │      Adapters (Inbound)      │  Handlers HTTP, DTOs, middlewares
           └──────────────┬───────────────┘
                          │
           ┌──────────────▼───────────────┐
           │  Casos de Uso (Core)         │  analysis, chat, trace, retention...
           └──────────────┬───────────────┘
                          │
           ┌──────────────▼───────────────┐
           │   Domínio + Portas            │  sinais, políticas, contratos
           └──────────────▲───────────────┘
                          │
           ┌──────────────┴───────────────┐
           │   Adapters (Outbound)         │  coletores, geradores, repositórios, filas
           └──────────────────────────────┘

Cross-cutting:
internal/platform/{config,health,logger,telemetry,retry,server,crypto,observability}
```

## Mapa de pacotes

- `cmd/observai-api`: composição do processo.
- `internal/core/domain`: modelos normalizados do domínio (análise, evidência, chat, auditoria, auth).
- `internal/core/ports`: contratos que o core consome.
- `internal/core/usecase`: orquestração (analysis, chat, auth, webhooks, setup, etc.).
- `internal/core/policy`: políticas de severidade, recomendações e redaction.
- `internal/adapters/inbound/http`: transporte HTTP, middleware, OpenAPI, DTOs.
- `internal/adapters/outbound`:
  - `factory`, `dynamic`, `composite`, `credentials`: composição e troca dinâmica de adapters.
  - `prometheus`, `loki`, `elasticsearch`, `jaeger`, `otel`, `datadog`, `dynatrace`, `newrelic`.
  - `ollama`, `openai`, `anthropic`: provedores LLM.
  - `postgres`, `redis`, `inmemory`, `asynq`: persistência e filas.
  - `webhooks`, `uuid`, `prompts`, `providertest`.
- `internal/platform`: bootstrap de servidor, configuração, tracing, health, telemetria, crypto.
- `agents`: prompts versionados de runtime para LLM.

## Fluxo de requisição: nova análise

1. `POST /v1/analyses` é validado e persistido como job de análise (`pending`).
2. Um worker de background retira o job da fila e executa `analysis.RunAnalysisJob`.
3. `usecase.Analysis.executeAnalyze`:
   - valida o request
   - coleta evidências via `SignalCollector`
   - normaliza e filtra evidências
   - aplica política de redaction
   - chama `AnalysisGenerator` (LLM)
   - aplica políticas de severidade/recomendação
   - persiste resultado e atualiza cache/contexto.
4. Notificador opcional dispara webhooks (`success`, `failure`, `canceled`).
5. Cliente consome:
   - `GET /v1/jobs/{jobID}` para progresso;
   - `GET /v1/analyses/{analysisID}` para resultado final.

## Fluxo de chat

1. Frontend chama `POST /v1/analyses/{id}/chat`.
2. `chat.Ask` valida escopo da pergunta (somente no contexto da análise).
3. Carrega contexto em cache ou repositório.
4. Chama `ChatResponder` (LLM) e persiste o histórico.
5. Há lock por analysis ID para manter perguntas concorrentes da mesma análise
   em ordem.

## Fila e workers

`analysis.WithAsyncBackend()` recebe:

- `ports.AnalysisJobRepository` para estado do job.
- `ports.JobEnqueuer` para enfileiramento.

A implementação da fila vem da configuração:

- `legacy` (fila em memória ou worker Redis conforme dependências)
- `asynq` quando o backend Redis + `asynq` estiver ativo.

Variáveis principais:

- `OBSERVAI_QUEUE_BACKEND`
- `OBSERVAI_QUEUE_CONCURRENCY`
- `OBSERVAI_QUEUE_DEQUEUE_TIMEOUT`
- `OBSERVAI_CHAT_LOCK_TTL`
- `OBSERVAI_CHAT_LOCK_WAIT`

## Carregamento e recarga de provedores

No startup, os providers e LLMs são construídos a partir da configuração.

Quando há configurações administrativas em banco, hooks de reload trocam os adapters
em runtime de forma atômica, sem reiniciar o processo.

Efeito prático:

- Mudanças de configuração entram em vigor automaticamente.
- Em erro de build da nova configuração, a API mantém o adapter anterior.
- `capabilities` e health refletem o estado ativo real.

## Contrato de resposta

Todas as respostas de sucesso usam:

```json
{
  "data": { "...": "payload específico" },
  "metadata": {
    "requestId": "uuid",
    "processingTimeMs": 0,
    "provider": {
      "mode": "prod",
      "observability": ["prometheus", "loki"],
      "llm": "ollama"
    },
    "warnings": []
  }
}
```

Ver `contract/README.md` para schema e códigos de erro exatos.

## Contratos e operação

- `GET /health`: resposta de liveness leve.
- `GET /healthz`: liveness para Kubernetes.
- `GET /readyz`: readiness com probes de dependências.
- `GET /metrics`: métricas Prometheus.
- `GET /v1/openapi.yaml`: contrato OpenAPI 3.1 embutido.
- `GET /v1/capabilities`: capacidades não sensíveis ativas no runtime.

## Segurança e transporte

- Autenticação suportada por:
  - API Key via bearer (`Authorization: Bearer`);
  - sessão de usuário com JWT cookies (`oai_session`, `oai_refresh`, `oai_csrf`).
- Role/scope de autorização aplicado por middleware.
- CSRF apenas para requests de mudança de estado em fluxo de browser.
- Credenciais de provedores vêm de referências seguras, sem vazamento por DTO.

## Operação

- Métricas de adapters em `observability` com tags por provedor/operação.
- Retry com backoff exponencial + jitter para chamadas de saída.
- Probe de readiness inclui dependências críticas (DB, Redis, provedores).
- OpenTelemetry pode ser habilitado via `OBSERVAI_OTEL_EXPORTER_OTLP_ENDPOINT`.

## Testes e confiabilidade

- Testes unitários e de integração cobrem políticas e adapters principais.
- A suíte inclui contrato HTTP e caminho assíncrono de análise.
- Use cases podem ser testados com implementações fake/in-memory para garantir
  comportamento determinístico sem dependências externas.

