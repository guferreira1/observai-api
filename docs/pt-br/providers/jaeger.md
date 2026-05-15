# Jaeger

Adapter de traces em modo somente leitura via API HTTP do Jaeger.

## Configuração

```yaml
observability:
  providers:
    - type: jaeger
      name: jaeger-prod
      url: http://jaeger-query.svc:16686
      timeout: 15s
      signals: [traces]
```

## Comportamento

- Utiliza `GET /api/traces/{traceID}`.
- O `traceID` vem de `AnalysisResult.TraceID` ou do `analysisID` como fallback.
- Converte spans para `domain.Span` normalizado (duração em ms, relacionamentos pai/filho, status).

## Health

`/api/services` é usado em readiness.

