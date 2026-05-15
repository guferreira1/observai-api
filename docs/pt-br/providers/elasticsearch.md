# Elasticsearch / OpenSearch

Adapter de coleta de logs em modo somente leitura.

O mesmo adapter cobre os tipos `elasticsearch` e `opensearch`.

## Configuração

```yaml
observability:
  providers:
    - type: elasticsearch  # ou opensearch
      name: logs-prod
      url: https://logs.example.com
      timeout: 10s
      signals: [logs]
      options:
        index: app-logs-*
        error_pattern: "(?i).*(error|exception|panic).*"
        service_field: service.name
        message_field: message
        timestamp_field: "@timestamp"
        username: observai-read
        password_ref: env:OBSERVAI_ELASTIC_PASSWORD
        # ou
        api_key: env:OBSERVAI_ELASTIC_API_KEY
```

## Opções

| Chave | Padrão | Significado |
| --- | --- | --- |
| `index` | `_all` | Índice ou padrão de índices de busca. |
| `error_pattern` | `(?i)(error|exception|panic)` | Regex para linhas suspeitas. |
| `service_field` | `service.name` | Campo usado para mapear service name. |
| `message_field` | `message` | Campo de texto do log. |
| `timestamp_field` | `@timestamp` | Campo de janela temporal. |
| `username` | (vazio) | Usuário de autenticação básica. |
| `password` / `password_ref` | (vazio) | Senha ou referência segura. |
| `api_key` | (vazio) | Referência de autenticação por API key. |

`api_key` possui precedência sobre autenticação básica.

