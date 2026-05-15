# Documentação do ObservAI API

O ObservAI API é um backend open-source e self-hosted para análise inteligente de
observabilidade.  
Não é um dashboard de visualização: é um motor de análise que conecta em sistemas
de observabilidade, normaliza dados e gera achados técnicos com auxílio de LLM.

O projeto é implementado em **Go** com arquitetura **hexagonal**, deixando o domínio
central isolado de frameworks HTTP, SDKs, banco de dados, filas e provedores de LLM.

## O que esta API entrega

- **Análises assíncronas** sobre logs, métricas, traces e dados de APM.
- **Projeto agnóstico de provedores** para fontes de observabilidade e LLMs.
- **Saídas estruturadas por política de domínio** com severidade, confiança,
  hipóteses, recomendações e evidências referenciadas.
- **Chat contextual** por análise, com validação de escopo e histórico persistido.
- **Camada administrativa** para provedores, chaves, usuários, webhooks e auditoria.
- **Endpoints operacionais** para saúde, disponibilidade, métricas e descoberta de
  capacidades de runtime.
- **Testes determinísticos** com portas explícitas e alternativas em memória.

## Leitura recomendada

- [Português](./)
- [English](../en/README.md)

## Arquitetura em visão geral

```txt
Cliente HTTP -> adaptadores inbound -> casos de uso -> domínio/políticas
                         ^            |
                         |            +-> adaptadores outbound (LLM + provedores + armazenamento + filas)
                         +-> contrato OpenAPI + autenticação + métricas + tracing
```

## Início rápido

1. Prepare a configuração (exemplo mínimo em modo local):

```bash
OBSERVAI_CONFIG_FILE=config/config.example.yaml \
go run ./cmd/observai-api
```

2. Envie uma análise:

```bash
curl -X POST http://localhost:8080/v1/analyses \
  -H "Authorization: Bearer <CHAVE_API_OU_SESSÃO>" \
  -H "Content-Type: application/json" \
  -d '{
    "goal": "Investigar aumento de latência no checkout nas últimas 30 minutos",
    "timeWindow": {"start":"2026-05-15T09:00:00Z","end":"2026-05-15T09:30:00Z"},
    "affectedServices": ["checkout-api"],
    "signals": ["metrics","logs","traces"],
    "context": "Deploy às 08:45 UTC"
  }'
```

3. Consulte o job retornado:

```bash
curl http://localhost:8080/v1/jobs/<jobId>
```

4. Busque o resultado da análise quando concluída:

```bash
curl http://localhost:8080/v1/analyses/<analysisId>
```

5. Faça follow-up no chat dessa análise:

```bash
curl -X POST http://localhost:8080/v1/analyses/<analysisId>/chat \
  -H "Authorization: Bearer <CHAVE_API_OU_SESSÃO>" \
  -H "X-CSRF-Token: <csrf-da-sessão-se-browser>" \
  -H "Content-Type: application/json" \
  -d '{ "question": "Qual a evidência mais forte dessa latência?" }'
```

> O fluxo com sessão de navegador usa os cookies `oai_session`, `oai_refresh` e
> `oai_csrf`. No fluxo por API Key, utilize apenas `Authorization: Bearer`.

## Como os componentes estão organizados

- `cmd/observai-api`: composição da aplicação e carregamento de configuração.
- `internal/core/*`: domínio, portas, políticas e casos de uso.
- `internal/adapters/inbound/http`: contrato HTTP, middlewares, DTOs, OpenAPI.
- `internal/adapters/outbound/*`: integrações e adaptadores de efeito colateral.
- `internal/platform/*`: recursos transversais (config, telemetria, retry, health,
  logging, servidor, crypto, migrations).
- `agents/*`: prompts versionados consumidos pelos adaptadores de LLM.

## Modelo de execução e integração

- Integrações podem vir de arquivo YAML (`OBSERVAI_CONFIG_FILE`) ou variáveis de
  ambiente.
- Provedores observabilidade e LLM são normalizados para um modelo de sinal e
  resultado agnóstico de provedor antes de chegar ao núcleo.
- Toda resposta com sucesso segue contrato `WrapperDtoResponde` (`data`, `metadata`).
- A análise assíncrona desacopla o tempo da requisição HTTP do tempo de processamento LLM.

## Observações de deploy

- Em desenvolvimento local, repositórios em memória podem ser usados quando não há
  Postgres ou Redis.
- Em produção, prefira Postgres e Redis, usando:
  `OBSERVAI_QUEUE_BACKEND`, `OBSERVAI_QUEUE_CONCURRENCY` e as configs de lock.
- Métricas públicas de Prometheus estão em `/metrics`.

## Para consumidores da API

- Use `GET /v1/openapi.yaml` para o contrato canônico.
- Consulte `contract/README.md` para detalhes de envelope e autenticação.
- Consulte os documentos de provedores ao configurar blocos `observability` e `llm`.

## Documentos relacionados

- `architecture.md`
- `contract/README.md`
- `providers/README.md`
- `ROADMAP.md`

