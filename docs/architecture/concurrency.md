# Concorrência na ObservAI API

## Visão geral

Cada requisição HTTP entra na ObservAI API em sua própria goroutine, despachada pelo `net/http` da biblioteca padrão. O estado dessa requisição vive no `*http.Request` e no `context.Context`, ambos por-requisição. Nenhum buffer global é compartilhado entre requisições.

Esse modelo é a razão pela qual diferentes usuários não recebem respostas trocadas: o caminho da requisição usa apenas locais de pilha, valores imutáveis e dependências que já são seguras para uso concorrente.

## Pontos de estado compartilhado

| Recurso | Tipo | Por que é seguro |
|---|---|---|
| `ollama.Client` | `*http.Client` | A `net/http` documenta que `Client` e `Transport` são seguros para uso concorrente. |
| `fake.AnalysisRepository` | mapa + `sync.RWMutex` | Todas as leituras/escritas passam pelo lock. |
| `fake.AnalysisContextCache` | mapa + `sync.RWMutex` | Idem. |
| `redis.AnalysisContextCache` | `*redis.Client` | O cliente go-redis é seguro para uso concorrente; cada chave Redis é indexada por `analysisID`, não cruza usuários. |
| `postgres.AnalysisRepository` | `*pgxpool.Pool` | `pgxpool` mantém um pool seguro para uso concorrente; cada `Save`/`Find` adquire uma conexão isolada. |
| `Router` | imutável após `NewRouter` | Os handlers são funções puras sobre o estado da requisição. |

## Onde a ordem importa

A única restrição de ordem introduzida pela API é o **FIFO de chat por análise**. Várias perguntas concorrentes sobre a mesma `analysisID` precisam atingir o LLM em ordem para preservar coerência conversacional. Essa serialização vive no `usecase.Chat.Ask`, envolvendo a chamada ao `ChatResponder` com `ports.AnalysisLocker.Acquire(analysisID)`.

Chats de análises diferentes continuam paralelos. Há dois adapters do locker:

- `fake.AnalysisLocker` — `sync.Map[string]*sync.Mutex`, suficiente em single-instance e em testes.
- `redis.AnalysisLocker` — `SET NX PX` + script Lua de release, válido para múltiplas instâncias.

## Fila de análise

A análise (`POST /v1/analyses`) é assíncrona. O handler:

1. valida o request;
2. cria um `AnalysisJob` em estado `pending` no `AnalysisJobRepository`;
3. enfileira o `jobID` via `JobEnqueuer`;
4. responde **202 Accepted** com `jobID` e `Location: /v1/jobs/{jobID}`.

Um worker consome a fila, recarrega o job pelo repositório, executa `executeAnalyze` e atualiza o estado para `running`/`completed`/`failed`. O cliente acompanha o progresso via `GET /v1/jobs/{jobID}`.

A fila desacopla o tempo de resposta da latência do LLM e introduz backpressure natural: a concorrência de workers é configurável (`OBSERVAI_QUEUE_CONCURRENCY`), evitando que um pico de requisições estoure memória ou o limite de tokens do provedor.

## Anti-padrões que evitamos

- Buffers globais que acumulam tokens entre requisições.
- Singletons que mantêm estado por usuário.
- Construção de clientes LLM por requisição (o cliente é reusado; só os parâmetros mudam).
- Goroutines não-bounded disparadas por requisição.
