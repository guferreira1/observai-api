# Datadog

Adapter observability somente leitura para Datadog.  
Implementa coleta de métricas via API de query v1 do Datadog.

## Autenticação

Datadog exige os cabeçalhos:

- `DD-API-KEY`: chave de API da organização
- `DD-APPLICATION-KEY`: chave de aplicação com escopo `metrics_read`

As duas partes são informadas como string única:

`<api_key>:<app_key>` em `options.credentials` quando usado no construtor.

## Configuração

```yaml
observability:
  providers:
    - type: datadog
      name: dd-prod
      url: https://api.datadoghq.com
      timeout: 15s
      signals: [metrics]
      options:
        credentials_ref: env:OBSERVAI_DATADOG_KEYS  # "api_key:app_key"
```

É possível usar a URL da região específica (`api.datadoghq.eu`, etc.).

## Templates

Padrões úteis:

- `service_request_latency_avg`: `avg:trace.servlet.request{service:{service}}` (ms)
- `service_error_rate`: `avg:trace.servlet.request.errors{service:{service}}` (rate)

O valor `{service}` é sanitizado antes da interpolação.

## Endpoint de teste

`POST /v1/admin/providers/{id}/test` valida conectividade.

## Persistência em banco

Quando usando configurações no banco, `credentials` é armazenado com criptografia
em repouso.

