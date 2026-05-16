package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	asynqadapter "github.com/guferreira1/observai-api/internal/adapters/outbound/asynq"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/dynamic"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/factory"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/ollama"
	prometheusadapter "github.com/guferreira1/observai-api/internal/adapters/outbound/prometheus"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/prompts"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/policy"
	coreports "github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/core/usecase"
	"github.com/guferreira1/observai-api/internal/platform/config"
	"github.com/guferreira1/observai-api/internal/platform/crypto"
	"github.com/guferreira1/observai-api/internal/platform/observability"
)

// providers groups the runtime adapters used by the analysis and chat use cases.
//
// The composition root populates this struct through the outbound factory,
// which dispatches construction by provider type. Typed client handles are
// preserved so health probes can ping the underlying backends without
// re-establishing connections.
type providers struct {
	collector         coreports.SignalCollector
	generator         coreports.AnalysisGenerator
	responder         coreports.ChatResponder
	traces            coreports.TraceProvider
	traceProviderName string
	prometheus        *prometheusadapter.Client
	ollama            *ollama.Client
	llmProvider       string
	llmModel          string
	observabilityList []observabilityProvider
}

type observabilityProvider struct {
	name    string
	signals []string
}

// newProviders resolves observability and LLM adapters via the outbound factory.
//
// The dispatcher maps inside the factory enforce one builder per provider
// type. When the operator declared no provider, the null adapters take
// over and the request surface reports provider-not-configured per call.
func newProviders(cfg config.Config, log *slog.Logger, observer observability.ProviderObserver, credentials coreports.CredentialStore) (providers, error) {
	dependencies := factory.Dependencies{
		Logger:       log,
		Observer:     observer,
		PromptLoader: prompts.NewFileLoader(cfg.Prompts.Dir),
		Credentials:  credentials,
	}

	observabilityResult, err := factory.BuildObservability(cfg, dependencies)
	if err != nil {
		return providers{}, err
	}

	llmResult, err := factory.BuildLLM(cfg, dependencies)
	if err != nil {
		return providers{}, err
	}

	traceResult, err := factory.BuildTraceProvider(cfg, dependencies)
	if err != nil {
		return providers{}, err
	}

	return providers{
		collector:         observabilityResult.Collector,
		generator:         llmResult.Generator,
		responder:         llmResult.Responder,
		traces:            traceResult.Provider,
		traceProviderName: traceResult.Name,
		prometheus:        observabilityResult.Clients.Prometheus,
		ollama:            llmResult.Clients.Ollama,
		llmProvider:       llmResult.Provider,
		llmModel:          llmResult.Model,
		observabilityList: toObservabilityProviders(observabilityResult.Capabilities),
	}, nil
}

// providerInventoryFromConfig adapts the current configuration into a
// snapshot for the Setup use case. Kept for the local/dev fallback when no
// DB-backed repository is wired.
func providerInventoryFromConfig(cfg config.Config) coreports.ProviderInventory {
	return coreports.ProviderInventoryFunc(func() coreports.ProviderInventorySnapshot {
		return coreports.ProviderInventorySnapshot{
			ObservabilityProviders: len(cfg.Observability.Providers),
			LLMProviders:           len(cfg.LLM.Providers),
		}
	})
}

// providerInventoryFromStores prefers the DB-backed provider/LLM
// configurations when available, falling back to the YAML/env-derived
// counts otherwise. The snapshot is computed at call time so the setup
// status endpoint reflects the live state without restart.
func providerInventoryFromStores(store analysisStore, cfg config.Config) coreports.ProviderInventory {
	if store.providerConfigs == nil || store.llmConfigs == nil {
		return providerInventoryFromConfig(cfg)
	}
	return coreports.ProviderInventoryFunc(func() coreports.ProviderInventorySnapshot {
		ctx, cancel := context.WithTimeout(context.Background(), 2*1e9)
		defer cancel()
		providerCount, err := store.providerConfigs.Count(ctx)
		if err != nil {
			providerCount = int64(len(cfg.Observability.Providers))
		}
		llmCount, err := store.llmConfigs.Count(ctx)
		if err != nil {
			llmCount = int64(len(cfg.LLM.Providers))
		}
		return coreports.ProviderInventorySnapshot{
			ObservabilityProviders: int(providerCount),
			LLMProviders:           int(llmCount),
		}
	})
}

// adapterRegistries holds dynamic atomic-pointer wrappers around the
// observability collector, LLM generator/responder and trace provider so
// the provider/LLM CRUD hooks can swap the live set without restarting
// the API.
type adapterRegistries struct {
	collector *dynamic.Collector
	generator *dynamic.Generator
	responder *dynamic.Responder
	traces    *dynamic.TraceProvider
}

func newAdapterRegistries(initial providers) adapterRegistries {
	return adapterRegistries{
		collector: dynamic.NewCollector(initial.collector),
		generator: dynamic.NewGenerator(initial.generator),
		responder: dynamic.NewResponder(initial.responder),
		traces:    dynamic.NewTraceProvider(initial.traces),
	}
}

// mergeDBProvidersIntoConfig translates DB-active provider configurations
// into the YAML-config shape consumed by the factory builders, then
// returns a Config copy with the merged providers. DB configs replace the
// legacy YAML list when at least one DB row is active.
//
// Plaintext credentials are decrypted via the supplied decrypter and
// piped through the per-type option keys recognized by each adapter's
// builder (Dynatrace api_token, Datadog credentials, New Relic api_key,
// Elasticsearch/OpenSearch username/password or api_key).
func mergeDBProvidersIntoConfig(cfg config.Config, dbConfigs []domain.ProviderConfig, decrypter func(domain.ProviderConfig) (string, error)) config.Config {
	if len(dbConfigs) == 0 {
		return cfg
	}
	merged := cfg
	merged.Observability.Providers = make([]config.ObservabilityProviderConfig, 0, len(dbConfigs))
	for _, dbc := range dbConfigs {
		options := cloneStringMap(dbc.Options)
		credentials, _ := decrypter(dbc)
		applyProviderCredentials(options, dbc.Type, credentials)
		merged.Observability.Providers = append(merged.Observability.Providers, config.ObservabilityProviderConfig{
			Type:    string(dbc.Type),
			Name:    dbc.Name,
			URL:     dbc.URL,
			Timeout: dbc.Timeout,
			Signals: dbc.Signals,
			Options: options,
		})
	}
	return merged
}

// mergeDBLLMIntoConfig replaces cfg.LLM.Providers with a single entry built
// from the active DB-configured LLM provider. The plaintext API key is
// injected through a literal: reference so the factory's credential
// dispatcher resolves it transparently.
func mergeDBLLMIntoConfig(cfg config.Config, active *domain.LLMConfig, plaintextAPIKey string) config.Config {
	if active == nil {
		return cfg
	}
	merged := cfg
	provider := config.LLMProviderConfig{
		Type:    string(active.Type),
		Name:    active.Name,
		BaseURL: active.BaseURL,
		Model:   active.Model,
		Timeout: active.Timeout,
		Options: active.Options,
	}
	if plaintextAPIKey != "" {
		provider.APIKeyEnv = "literal:" + plaintextAPIKey
	}
	merged.LLM.Providers = []config.LLMProviderConfig{provider}
	merged.LLM.Active = provider.Name
	return merged
}

func initialRuntimeProviderConfig(ctx context.Context, cfg config.Config, log *slog.Logger, providerConfigs *usecase.ProviderConfig, llmConfigs *usecase.LLMConfig) (config.Config, bool) {
	runtimeConfig, observabilityLoaded := initialObservabilityProviderConfig(ctx, cfg, log, providerConfigs)
	runtimeConfig, llmLoaded := initialLLMProviderConfig(ctx, runtimeConfig, log, llmConfigs)
	return runtimeConfig, observabilityLoaded || llmLoaded
}

func initialObservabilityProviderConfig(ctx context.Context, cfg config.Config, log *slog.Logger, providerConfigs *usecase.ProviderConfig) (config.Config, bool) {
	if providerConfigs == nil {
		return cfg, false
	}
	activeConfigs, err := providerConfigs.ListActiveConfigs(ctx)
	if err != nil {
		log.Warn("load active provider configurations failed", "error", err)
		return cfg, false
	}
	if len(activeConfigs) == 0 {
		return cfg, false
	}
	return mergeDBProvidersIntoConfig(cfg, activeConfigs, providerConfigs.DecryptCredentials), true
}

func initialLLMProviderConfig(ctx context.Context, cfg config.Config, log *slog.Logger, llmConfigs *usecase.LLMConfig) (config.Config, bool) {
	if llmConfigs == nil {
		return cfg, false
	}
	active, err := llmConfigs.FindActiveConfig(ctx)
	if errors.Is(err, domain.ErrLLMConfigNotFound) {
		return cfg, false
	}
	if err != nil {
		log.Warn("load active llm provider configuration failed", "error", err)
		return cfg, false
	}
	plaintextAPIKey, err := llmConfigs.DecryptAPIKey(active)
	if err != nil {
		log.Warn("decrypt active llm provider api key failed", "error", err)
		return cfg, false
	}
	return mergeDBLLMIntoConfig(cfg, &active, plaintextAPIKey), true
}

func applyProviderCredentials(options map[string]string, providerType domain.ObservabilityProviderType, credentials string) {
	if options == nil || credentials == "" {
		return
	}
	switch providerType {
	case domain.ProviderTypeDynatrace:
		options["api_token"] = credentials
	case domain.ProviderTypeDatadog:
		options["credentials"] = credentials
	case domain.ProviderTypeNewRelic:
		options["api_key"] = credentials
	case domain.ProviderTypeElasticsearch, domain.ProviderTypeOpenSearch:
		if strings.Contains(credentials, ":") {
			parts := strings.SplitN(credentials, ":", 2)
			options["username"] = parts[0]
			options["password"] = parts[1]
			break
		}
		options["api_key"] = credentials
	}
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

// factoryDependencies builds the shared dependency bag for the outbound
// factory. Extracted so the startup wiring and the reload hook share the
// exact same dependency graph.
func factoryDependencies(cfg config.Config, log *slog.Logger, observer observability.ProviderObserver, credentialStore coreports.CredentialStore) factory.Dependencies {
	return factory.Dependencies{
		Logger:       log,
		Observer:     observer,
		PromptLoader: prompts.NewFileLoader(cfg.Prompts.Dir),
		Credentials:  credentialStore,
	}
}

// newAdapterReloader returns a ReloadHook that, on each invocation,
// reads the active provider/LLM configurations from the database,
// translates them into the YAML-config shape, runs the factory builders
// and atomically swaps the collector/generator/responder/traces inside
// the supplied registries.
//
// Failures are logged at warn level and never propagated so a bad
// provider configuration cannot wedge the API; the previous adapter set
// stays in use until the operator fixes the configuration.
func newAdapterReloader(cfg config.Config, log *slog.Logger, deps factory.Dependencies, registries adapterRegistries, capabilities *capabilitiesStore, providerConfigs *usecase.ProviderConfig, llmConfigs *usecase.LLMConfig) usecase.ReloadHook {
	return func(ctx context.Context) {
		if providerConfigs != nil {
			dbConfigs, err := providerConfigs.ListActiveConfigs(ctx)
			if err != nil {
				log.Warn("reload: list active providers failed", "error", err)
			} else {
				merged := mergeDBProvidersIntoConfig(cfg, dbConfigs, providerConfigs.DecryptCredentials)
				if result, err := factory.BuildObservability(merged, deps); err != nil {
					log.Warn("reload: build observability failed", "error", err)
				} else {
					registries.collector.Set(result.Collector)
					if capabilities != nil {
						snapshot := capabilities.Get()
						snapshot.Observability = toCapabilityProviders(toObservabilityProviders(result.Capabilities))
						capabilities.Set(snapshot)
					}
					if trace, err := factory.BuildTraceProvider(merged, deps); err != nil {
						log.Warn("reload: build trace provider failed", "error", err)
					} else if trace.Provider != nil {
						registries.traces.Set(trace.Provider)
					}
				}
			}
		}
		if llmConfigs != nil {
			active, err := llmConfigs.FindActiveConfig(ctx)
			if err == nil {
				plaintext, _ := llmConfigs.DecryptAPIKey(active)
				merged := mergeDBLLMIntoConfig(cfg, &active, plaintext)
				if result, err := factory.BuildLLM(merged, deps); err != nil {
					log.Warn("reload: build llm failed", "error", err)
				} else {
					if result.Generator != nil {
						registries.generator.Set(result.Generator)
					}
					if result.Responder != nil {
						registries.responder.Set(result.Responder)
					}
					if capabilities != nil {
						snapshot := capabilities.Get()
						snapshot.LLM.Provider = result.Provider
						snapshot.LLM.Model = result.Model
						capabilities.Set(snapshot)
					}
				}
			}
		}
	}
}

// newSchedulerIfEnabled wires the Asynq-backed scheduler when the operator
// has opted in via OBSERVAI_SCHEDULER_ENABLED and a Redis URL is
// configured. Otherwise it logs the reason and returns (nil, nil) so the
// composition root can continue without the scheduler.
func newSchedulerIfEnabled(cfg config.Config, log *slog.Logger, retention *usecase.AnalysisRetention, webhooks *usecase.WebhookSubscriptions) (*asynqadapter.Scheduler, error) {
	if !cfg.Scheduler.Enabled {
		return nil, nil
	}
	redisURL := strings.TrimSpace(cfg.RedisURL)
	if redisURL == "" {
		log.Warn("scheduler enabled but OBSERVAI_REDIS_URL is empty; scheduler disabled")
		return nil, nil
	}

	var retentionHandler asynqadapter.RetentionPurgeHandler
	if retention != nil && (cfg.Scheduler.RetentionDays > 0 || cfg.Scheduler.RetentionQuantity > 0) {
		days := cfg.Scheduler.RetentionDays
		quantity := cfg.Scheduler.RetentionQuantity
		retentionHandler = func(ctx context.Context) error {
			if days > 0 {
				age := time.Duration(days) * 24 * time.Hour
				deleted, err := retention.Purge(ctx, age)
				if err != nil {
					return err
				}
				log.Info("retention purge executed", "deleted", deleted, "age_days", days)
			}
			if quantity > 0 {
				deleted, err := retention.PurgeByQuantity(ctx, quantity)
				if err != nil {
					return err
				}
				log.Info("retention purge-by-quantity executed", "deleted", deleted, "keep", quantity)
			}
			return nil
		}
	}

	var webhookHandler asynqadapter.WebhookRetrySweepHandler
	if webhooks != nil {
		lookup := cfg.Scheduler.WebhookSweepLookup
		webhookHandler = func(ctx context.Context) error {
			cutoff := time.Now().UTC()
			dispatched, err := webhooks.DispatchPending(ctx, cutoff, 100)
			if err != nil {
				return err
			}
			if dispatched > 0 {
				log.Info("webhook retry sweep dispatched", "count", dispatched, "lookup", lookup)
			}
			return nil
		}
	}

	if retentionHandler == nil && webhookHandler == nil {
		log.Warn("scheduler enabled but no handlers wired; scheduler disabled")
		return nil, nil
	}

	return asynqadapter.NewScheduler(asynqadapter.SchedulerOptions{
		RedisURL:            redisURL,
		RetentionCron:       cfg.Scheduler.RetentionCron,
		WebhookRetryCron:    cfg.Scheduler.WebhookRetryCron,
		RetentionHandler:    retentionHandler,
		WebhookRetryHandler: webhookHandler,
		Logger:              log,
		Concurrency:         cfg.Scheduler.Concurrency,
	})
}

// buildRedactionChain translates OBSERVAI_REDACTION_RULES into a configured
// ChainRedactor. An empty configuration falls back to the default chain.
func buildRedactionChain(cfg config.Config) policy.ChainRedactor {
	cleaned := strings.TrimSpace(cfg.Redaction.Rules)
	if cleaned == "" {
		return policy.NewDefaultRedactor()
	}
	rules := make([]policy.RedactorRule, 0)
	for _, raw := range strings.Split(cleaned, ",") {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		rules = append(rules, policy.RedactorRule(name))
	}
	return policy.NewRedactorFromConfig(policy.RedactorConfig{Enabled: rules})
}

// buildEncryptionCipher returns the cipher used by the provider/LLM
// configuration use cases. In local mode without a configured key the
// function generates a volatile key for the session and warns; production
// startup already enforces the presence of the key via config.Validate.
func buildEncryptionCipher(cfg config.Config, log *slog.Logger) (coreports.Cipher, error) {
	key, err := cfg.LoadEncryptionKey()
	if err != nil {
		if !errors.Is(err, config.ErrEncryptionKeyMissing) {
			return nil, err
		}
		volatile, err := crypto.GenerateKey()
		if err != nil {
			return nil, err
		}
		log.Warn("encryption key missing; generated volatile in-memory key for local session")
		key = volatile
	}
	return crypto.NewAESGCMCipher(key)
}

func toObservabilityProviders(capabilities []factory.ProviderCapability) []observabilityProvider {
	if len(capabilities) == 0 {
		return []observabilityProvider{{name: "none", signals: []string{}}}
	}
	out := make([]observabilityProvider, 0, len(capabilities))
	for _, capability := range capabilities {
		out = append(out, observabilityProvider{name: capability.Name, signals: capability.Signals})
	}
	return out
}
