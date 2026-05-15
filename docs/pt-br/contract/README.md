# Contrato da API

O ObservAI expõe um contrato OpenAPI oficial como fonte principal de integração.

- Arquivo da especificação: `internal/adapters/inbound/http/openapi.yaml`
- Endpoint servido: `GET /v1/openapi.yaml`

O contrato é mantido alinhado com a camada HTTP em Go e validado por testes de integração
que checam cobertura de rotas e comportamento de envelope.

## Envelope

Toda resposta de sucesso usa `WrapperDtoResponde`:

```json
{
  "data": { "...": "payload do endpoint" },
  "metadata": {
    "requestId": "uuid-v4",
    "processingTimeMs": 12,
    "provider": {
      "mode": "prod",
      "observability": ["prometheus", "loki"],
      "llm": "ollama"
    },
    "warnings": [],
    "pagination": { "limit": 20, "offset": 0, "next": "/v1/analyses?limit=20&offset=20" }
  }
}
```

Erros usam o mesmo envelope e retornam código em `data.code`.

### Campos de erro úteis para clientes

- `code`: identificador estável do erro.
- `message`: resumo curto para UI.
- `details`: detalhes opcionais por validação de campo.

## Resumo de autenticação

- **Sessão de navegador**
  - `POST /v1/auth/login` define:
    - `oai_session` (HttpOnly),
    - `oai_refresh` (HttpOnly, path `/v1/auth`),
    - `oai_csrf` (acessível por JS).
  - Envie `X-CSRF-Token: <csrf>` em requests não-GET.
- **API Key**
  - Use `Authorization: Bearer <token>`.
- **Keys estáticas ou persistidas**
  - Chaves podem vir de ambiente ou de endpoints de administração.

## Recursos principais

- `POST /v1/analyses` -> envio assíncrono de análise (`202 Accepted`) com `jobID`.
- `GET /v1/jobs/{jobID}` -> estado/progresso do job.
- `GET /v1/analyses/{analysisID}` -> resultado final da análise.
- `POST /v1/analyses/{analysisID}/chat` -> pergunta contextual na análise.
- `GET /v1/analyses/{analysisID}/chat` -> histórico de chat.
- `POST /v1/admin/...` -> provedores, LLMs, usuários, chaves, webhooks, auditoria e setup.

## Paginação e filtros

Os endpoints de lista aceitam:

- `limit` / `offset`
- `from` / `to` (RFC3339)
- `severity` (`low|medium|high|critical`)
- `service`, `signal`, `provider`, `q`, `sort`, `order`.

## Streaming

`POST /v1/analyses/{analysisID}/chat` retorna JSON padrão por padrão.

Para SSE:

```http
Accept: text/event-stream
```

Eventos emitidos:

- `token`
- `evidence_cited`
- `done`
- `error`

## Fluxo assíncrono e webhooks

- `POST /v1/analyses` cria job e envia à fila.
- `/v1/jobs/{jobID}` retorna fase e progresso.
- Webhooks externos podem ser notificados na conclusão/falha/cancelamento.

## Endpoints operacionais e metadados

- `GET /health` e `GET /healthz`: sinais de liveness.
- `GET /readyz`: readiness de dependências.
- `GET /v1/capabilities`: resumo ativo de provedor e modo.
- `GET /metrics`: exposição Prometheus.

## Manter docs e código alinhados

- DTOs em runtime: `internal/adapters/inbound/http/dto.go`.
- Especificação OpenAPI: `internal/adapters/inbound/http/openapi.yaml`.
- `internal/adapters/inbound/http/openapi_test.go` valida consistência entre rota e spec.
