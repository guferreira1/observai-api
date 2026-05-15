# Modelo de concorrência

O ObservAI API foi projetado para ser seguro em concorrência.  
O transporte (`net/http` + chi) e os adapters garantem que um request não compartilhe
estado mutável com outro.

## Isolamento por requisição

Cada requisição HTTP tem seu próprio `context.Context` e logger.
As dependências de infraestrutura são imutáveis ou seguras para uso concorrente:

- pools de banco (`pgxpool.Pool`)
- clientes HTTP (`http.Client` / transport)
- clientes de cache (`redis.Client`)
- repositórios/locks em memória com mutex quando usado em modo local

## Estados compartilhados e segurança

| Recurso | Tipo | Por que é seguro |
| --- | --- | --- |
| `ollama.Client` | wrappers HTTP | `http.Client` e `http.Transport` são seguros em concorrência. |
| `inmemory.AnalysisRepository` | mapa + `sync.RWMutex` | Todo acesso passa por lock. |
| `redis.AnalysisContextCache` | cliente Redis | Pool seguro e chave por `analysisID`, sem cruzamento de usuários. |
| `postgres.AnalysisRepository` | `pgxpool.Pool` | Pool gerencia conexões concorrentes com segurança. |
| Fila e lock de chat | Redis ou memória | Bloqueio por `analysisID`; lock com TTL para serialização por análise. |
| Router | imutável após `NewRouter` | Handlers funcionam apenas sobre estado da requisição. |

## Onde a ordem importa

A regra de ordem real é **FIFO do chat por análise**.

- Análises diferentes rodam paralelamente.
- Mensagens da mesma análise são serializadas via `usecase.Chat` e `AnalysisLocker`.

## Fila e workers

`POST /v1/analyses` cria um job e responde rápido (`202`).

Workers processam com concorrência configurada:

1. Marcar job como `running`.
2. Persistir fase/andamento (`collecting`, `calling_llm`, etc.).
3. Gerar resultado.
4. Marcar finalização (`completed`/`failed`/`canceled`).

### Backend Redis

- `legacy`: fila/worker com estruturas Redis.
- `asynq`: uso de Asynq para fila e scheduling.

O lock em Redis possui TTL e política de espera, o que permite múltiplas instâncias.

## Controles de concorrência em escrita

- Cancelamento de análise usa contexto de cancelamento por job.
- `chat_lock_ttl` e `chat_lock_wait` evitam lock preso e starvation.
- Em memória, mutex local com concorrência de worker limitada continua funcionando.

## Anti-padrões evitados

- Sem estado global com dados do usuário ou trace.
- Sem criar clientes de adapter por requisição.
- Sem goroutines não-limitadas por requisição.
- Sem compartilhar sessão mutável de LLM entre requisições.

