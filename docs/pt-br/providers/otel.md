# OTel / Tempo

Adapter de traces somente leitura para backends compatíveis com OTLP, incluindo Tempo.

## Configuração

```yaml
observability:
  providers:
    - type: tempo
      name: tempo-prod
      url: http://tempo.svc:3200
      timeout: 15s
      signals: [traces]
```

Aliases aceitos: `otel`, `otlp`, `tempo`.

## Comportamento

- Lê traces em formato `OTLP-JSON` em `/api/traces/{traceID}`.
- Achata spans por resource e extrai:
  - service name de `service.name`
  - relação pai-filho de `parent_span_id`
  - status via código `status.code`.

## Health

Quando disponível, usa `/api/echo`.

