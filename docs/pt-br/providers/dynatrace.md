# Dynatrace

Adapter observabilidade somente leitura para Dynatrace.

## Autenticação

Usa `Authorization: Api-Token <token>`.

Gere um token com, no mínimo:

- `metrics.read`
- `entities.read`

## Configuração

```yaml
observability:
  providers:
    - type: dynatrace
      name: dt-prod
      url: https://abc12345.live.dynatrace.com
      timeout: 15s
      signals: [metrics]
      options:
        api_token_ref: env:OBSERVAI_DT_TOKEN
```

## Templates

- `service_response_time_avg`: `builtin:service.response.time:filter(eq("dt.entity.service.name","{service}")):avg` (ms)
- `service_error_rate`: `builtin:service.errors.total.rate:filter(eq("dt.entity.service.name","{service}"))`

Valores de `{service}` são escapados antes de substituição.

## Teste de conexão

`POST /v1/admin/providers/{id}/test` valida com `GET /api/v2/clusterversion`.

