# Pattern strategy rule

## Goal

Keep business rules readable, testable, extensible and easy to replace.

Agents must follow Clean Code, SOLID principles and appropriate Design Patterns when implementing or changing business behavior.

Agents must not write loose procedural code when the behavior represents a domain rule, provider variation, analysis decision, normalization decision, recommendation rule or response shaping rule.

The default approach must be:

- clear responsibility;
- small functions;
- explicit dependencies;
- readable names;
- isolated business rules;
- independently testable behavior;
- no hidden coupling;
- no large conditional blocks.

---

## Non-negotiable principles

All code must follow:

- Single Responsibility Principle;
- Open/Closed Principle;
- Dependency Inversion Principle;
- Interface Segregation Principle;
- Clean Code;
- explicit dependency injection;
- readable naming;
- small cohesive packages;
- behavior isolated behind clear contracts;
- tests focused on behavior.

Agents must not choose the fastest implementation if it creates coupling, unreadable code or future maintenance problems.

---

## Conditional limit

If a block, function or method needs more than two `if`, `else if`, `switch` cases or equivalent conditional branches to choose between behaviors, agents must stop and extract the variation into a pattern.

This rule applies especially when conditionals represent different behaviors, rules, providers, policies or business decisions.

Preferred options:

- Strategy pattern for interchangeable behavior.
- Policy object for decision rules.
- Specification pattern for composable predicates.
- Rule object for isolated analysis or recommendation rules.
- Chain of Responsibility for sequential rule evaluation.
- Factory pattern for controlled construction.
- Small dispatcher map when selecting handlers by stable keys.
- Adapter pattern for provider-specific integrations.
- Template Method-like composition through interfaces when the algorithm has stable steps and replaceable behavior.

Do not accumulate complex behavior inside a single function.

---

## What must be isolated

Extract a strategy, policy, specification, rule object, adapter or dispatcher when conditionals decide:

- provider-specific behavior;
- signal normalization behavior;
- LLM prompt construction behavior;
- LLM response translation behavior;
- analysis rules;
- hypothesis rules;
- recommendation rules;
- severity calculation;
- scoring logic;
- prioritization logic;
- chat scope behavior;
- refusal behavior;
- API response shaping behavior;
- persistence behavior;
- cache behavior;
- retry behavior;
- timeout behavior;
- fallback behavior;
- observability enrichment behavior.

Each extracted rule must have one clear responsibility, receive explicit dependencies and be independently testable.

---

## Acceptable conditionals

Simple guard clauses are allowed for:

- error handling;
- nil checks;
- context cancellation;
- basic input validation;
- early returns that keep the main flow readable.

Acceptable guard clauses must stay shallow.

Guard clauses must not hide business decisions.

Example of acceptable guard clause:

    if request == nil {
        return nil, ErrInvalidRequest
    }

Example of unacceptable business conditional accumulation:

    if provider == "dynatrace" {
        ...
    } else if provider == "datadog" {
        ...
    } else if provider == "newrelic" {
        ...
    }

This must become a provider strategy, adapter registry or dispatcher.

---

## Naming rule

Variables, parameters, functions, structs, interfaces and packages must have readable names.

Agents must not use one-letter names unless the meaning is obvious and idiomatic in a very small scope.

Avoid names like:

- `a`
- `b`
- `c`
- `x`
- `y`
- `z`
- `d`
- `r`
- `s`
- `m`
- `p`
- `v`
- `tmp`
- `data`
- `info`
- `obj`
- `val`
- `res`
- `req`
- `ctxx`

Allowed short names only when they are idiomatic and obvious:

- `ctx` for `context.Context`;
- `id` for identifiers;
- `tx` for database transaction;
- `db` for database handle;
- `tt` in table-driven tests;
- `t` in tests as `*testing.T`;
- `w` and `r` only in HTTP handlers when the function is very small and follows Go convention;
- `i` only for simple loop indexes;
- `ok` for map/type assertion checks;
- `err` for errors.

Even when short names are allowed, prefer readable names when the scope is not trivial.

Bad:

    func Analyze(ctx context.Context, p Provider, d Data) error

Good:

    func Analyze(ctx context.Context, observabilityProvider Provider, signalBatch SignalBatch) error

Bad:

    for _, x := range items {
        result = append(result, normalize(x))
    }

Good:

    for _, rawSignal := range rawSignals {
        normalizedSignals = append(normalizedSignals, normalize(rawSignal))
    }

---

## Function responsibility rule

A function must not mix multiple responsibilities.

A single function must not perform all of these together:

- validate input;
- choose provider;
- normalize provider data;
- execute analysis;
- call LLM;
- persist result;
- shape API response;
- emit logs and metrics;
- handle retries;
- calculate severity.

When a function starts mixing orchestration with business decisions, agents must split it.

Preferred structure:

- handler validates transport input and calls use case;
- use case orchestrates business flow;
- strategy/policy/rule handles business variation;
- adapter handles external provider details;
- presenter/mapper shapes API response;
- repository/cache adapter handles persistence details.

---

## Orchestration rule

Use cases may orchestrate steps, but must not contain provider-specific logic or large business conditionals.

A use case can call interfaces such as:

    SignalCollector
    SignalNormalizer
    AnalysisRule
    RecommendationPolicy
    LLMProvider
    AnalysisRepository

A use case must not directly decide:

    if provider == "dynatrace"
    if provider == "datadog"
    if llmProvider == "openai"
    if llmProvider == "anthropic"

Provider-specific logic belongs in adapters, factories, registries or strategies.

---

## Strategy pattern rule

Use Strategy when multiple implementations perform the same type of behavior differently.

Use it for:

- provider-specific collection;
- provider-specific normalization;
- LLM-specific prompt execution;
- LLM-specific response parsing;
- severity calculation;
- recommendation generation;
- analysis enrichment.

Expected shape:

    type SignalCollector interface {
        Collect(ctx context.Context, request SignalCollectionRequest) (SignalBatch, error)
    }

Each provider must implement the same contract.

The use case must depend on the interface, not on concrete provider implementations.

---

## Policy object rule

Use Policy objects when the code needs to decide whether something is allowed, relevant, severe, prioritized or eligible.

Use it for:

- severity policy;
- recommendation eligibility;
- chat refusal policy;
- provider fallback policy;
- analysis scope policy;
- cache eligibility policy.

Policy names must express domain intent.

Bad:

    CheckPolicy

Good:

    CriticalSeverityPolicy
    ProviderFallbackPolicy
    SensitiveSignalPolicy

---

## Specification pattern rule

Use Specification when the code needs composable predicates.

Use it for:

- filtering signals;
- detecting relevant spans;
- identifying error logs;
- classifying anomalies;
- selecting affected services;
- matching incident evidence.

Specifications must be small and independently testable.

Avoid anonymous inline predicate chains when the predicate has domain meaning.

---

## Rule object rule

Use Rule objects when each rule produces a finding, anomaly, recommendation or hypothesis.

Use it for:

- error spike detection;
- latency anomaly detection;
- database bottleneck detection;
- retry amplification detection;
- N+1 query suspicion;
- memory saturation detection;
- dependency degradation detection.

Expected shape:

    type AnalysisRule interface {
        Evaluate(ctx context.Context, input AnalysisInput) ([]Finding, error)
    }

Rules must be composable.

Rules must not know about HTTP, database, cache or provider SDKs.

---

## Dispatcher map rule

A small dispatcher map is allowed when selecting handlers by stable keys.

Good use cases:

- selecting provider adapter by provider type;
- selecting LLM adapter by configured provider;
- selecting normalizer by signal type;
- selecting parser by response format.

Example:

    collectors := map[ProviderType]SignalCollector{
        ProviderTypeDynatrace: dynatraceCollector,
        ProviderTypeDatadog: datadogCollector,
    }

Dispatcher maps must stay small and explicit.

Do not hide complex business logic inside anonymous functions in maps.

---

## Factory rule

Use factories when object creation depends on configuration, provider type or runtime selection.

Factories must create and wire dependencies.

Factories must not execute business logic.

Factory names must be explicit.

Good:

    SignalCollectorFactory
    LLMProviderFactory
    AnalysisRuleFactory

Bad:

    Factory
    ProviderFactory
    ObjectFactory

---

## Adapter rule

Use Adapter pattern for every external provider.

Adapters must isolate:

- provider SDK types;
- provider authentication;
- provider request format;
- provider response format;
- provider pagination;
- provider rate limits;
- provider error format;
- provider-specific normalization.

The core must never import provider SDK packages.

---

## Interface rule

Interfaces must stay close to the consumer package.

Do not create interfaces only because an implementation exists.

Create an interface when:

- the core needs to depend on behavior;
- tests need to replace external dependencies;
- multiple implementations are expected;
- provider replacement is part of the design.

Interfaces must be small.

Bad:

    type ObservabilityService interface {
        GetLogs(...)
        GetMetrics(...)
        GetTraces(...)
        GetAPM(...)
        Normalize(...)
        Analyze(...)
        Save(...)
        Notify(...)
    }

Good:

    type LogCollector interface {
        CollectLogs(ctx context.Context, request LogCollectionRequest) ([]LogEntry, error)
    }

    type TraceCollector interface {
        CollectTraces(ctx context.Context, request TraceCollectionRequest) ([]Trace, error)
    }

---

## Dependency rule

Business rules must receive dependencies explicitly.

Do not instantiate infrastructure dependencies inside use cases or rules.

Bad:

    func NewAnalyzeUseCase() *AnalyzeUseCase {
        client := dynatrace.NewClient(...)
        return &AnalyzeUseCase{client: client}
    }

Good:

    func NewAnalyzeUseCase(signalCollector SignalCollector, analyzer Analyzer) *AnalyzeUseCase {
        return &AnalyzeUseCase{
            signalCollector: signalCollector,
            analyzer: analyzer,
        }
    }

---

## Readability rule

Code must be readable before it is clever.

Agents must avoid:

- cryptic names;
- deeply nested conditionals;
- long functions;
- large structs with unrelated fields;
- generic names;
- hidden side effects;
- premature abstractions;
- duplicated business rules;
- provider logic inside use cases;
- business logic inside HTTP handlers.

Prefer:

- explicit flow;
- domain language;
- simple composition;
- focused types;
- small files;
- testable behavior.

---

## Test rule

Every extracted strategy, policy, specification or rule object must have focused tests.

Tests must cover:

- expected behavior;
- unsupported or invalid input;
- edge cases;
- error paths when relevant.

Use table-driven tests when it improves clarity.

Test names must describe behavior.

Bad:

    TestRule

Good:

    TestCriticalSeverityPolicyReturnsCriticalWhenErrorRateIsAboveThreshold

---

## Review checklist

Before finishing any task, agents must verify:

- No function mixes orchestration, provider decisions, normalization, LLM behavior, persistence and response shaping.
- No business behavior is hidden behind many inline conditionals.
- More than two behavioral conditionals in the same block are replaced by a Strategy, Policy, Specification, Rule Object, Chain of Responsibility, Factory, Adapter or dispatcher.
- Rule implementations are small, named with domain language and covered by focused tests.
- Interfaces stay close to the consumer package.
- Interfaces do not create unnecessary layers.
- Dependencies are explicit.
- Variable and parameter names are readable.
- One-letter names are used only when idiomatic and obvious in a very small scope.
- Exported Go elements have GoDocs.
- The core does not import adapters, SDKs, HTTP packages, database clients or cache clients.
- Code follows Clean Code, SOLID and appropriate Design Patterns.

---

## Mandatory refactor triggers

Agents must refactor before completing the task when any of the following appears:

- a function grows because new behavior was added through another `if`;
- provider-specific behavior appears inside a use case;
- LLM-specific behavior appears inside a use case;
- normalization logic is duplicated across handlers or adapters;
- the same condition appears in multiple places;
- a function needs more than two behavioral branches;
- a variable name requires mental decoding;
- a parameter name is a single letter without obvious meaning;
- a test needs excessive setup because the unit has too many responsibilities;
- a handler contains business rules;
- a repository contains business rules;
- a rule cannot be tested without starting infrastructure.

When a trigger appears, stop adding behavior and extract the correct abstraction.