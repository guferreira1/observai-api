# New Relic

Adapter de observabilidade somente leitura usando GraphQL da New Relic.

## Autenticação

- Cabeçalho `API-Key: <user-api-key>`.
- Chave precisa de permissão `Read NRQL queries`.
- `account_id` é obrigatório em opções.

## Configuração

```yaml
observability:
  providers:
    - type: newrelic
      name: nr-prod
      url: https://api.newrelic.com
      timeout: 15s
      signals: [metrics]
      options:
        api_key_ref: env:OBSERVAI_NEWRELIC_KEY
        account_id: "1234567"
```

Endpoint da UE:

`https://api.eu.newrelic.com`

## Templates

- `transaction_duration_p95`: `SELECT percentile(duration, 95) FROM Transaction WHERE service.name = '{service}' SINCE 15 minutes ago` (ms)
- `error_rate`: `SELECT count(*) FROM TransactionError WHERE service.name = '{service}' SINCE 15 minutes ago`

## Endpoint de teste

`POST /v1/admin/providers/{id}/test` valida execução GraphQL.

