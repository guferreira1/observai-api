# Loki

Adapter de logs em modo somente leitura usando `/loki/api/v1/query_range`.

## Configuração

```yaml
observability:
  providers:
    - type: loki
      name: loki-prod
      url: http://loki.svc:3100
      timeout: 10s
      signals: [logs]
```

## Comportamento

- Monta query range por serviço afetado.
- O template padrão conta eventos com `(?i)(error|exception|panic)`.
- O step padrão é 60s com limite interno.

## Health

Readiness pode consultar `/ready`.

