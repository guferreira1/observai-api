# Prometheus

Adapter de métricas em modo somente leitura.

## Configuração

```yaml
observability:
  providers:
    - type: prometheus
      name: prom-prod
      url: http://prometheus.svc:9090
      timeout: 10s
      signals: [metrics]
```

## Compatibilidade legada

Variáveis antigas ainda funcionam:

- `OBSERVAI_PROMETHEUS_URL`
- `OBSERVAI_PROMETHEUS_TIMEOUT`

## Comportamento

- Executa query instantânea em `/api/v1/query`.
- Usa templates por tipo de evidência; não encaminha query bruta do usuário.
- Template padrão checa `up` por serviço afetado.
- Retry de falhas transitórias (rede e `5xx`) com backoff + jitter.

## Health

Readiness pode consultar `/-/healthy`.

