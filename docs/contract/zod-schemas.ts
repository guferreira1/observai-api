/**
 * ObservAI API — Zod schemas (contract source for the frontend).
 *
 * This file is the authoritative TypeScript+Zod representation of every
 * DTO the frontend consumes. The Go backend OpenAPI document under
 * `internal/adapters/inbound/http/openapi.yaml` is the same contract
 * expressed in OpenAPI 3.1; both files MUST stay in sync.
 *
 * Generation strategy:
 *   1. The Go DTOs in `internal/adapters/inbound/http/dto.go` are the
 *      runtime source of truth.
 *   2. This file mirrors them, with optional fields explicitly marked.
 *   3. The frontend imports these schemas (or copies them verbatim) and
 *      uses `safeParse` at the network boundary so any drift triggers a
 *      typed error rather than silently corrupting state.
 *
 * Conventions:
 *   - Timestamps are ISO-8601 strings (RFC 3339).
 *   - Severity / Confidence / SignalType / Status enums match the Go
 *     domain constants verbatim.
 *   - The `WrapperDtoResponde` envelope wraps every successful response;
 *     errors share the same envelope with an `ErrorResponse` `data`.
 */

import { z } from "zod";

// --- Primitives -----------------------------------------------------------

export const Iso8601 = z.string().datetime({ offset: true });

export const SeveritySchema = z.enum([
  "info",
  "low",
  "medium",
  "high",
  "critical",
]);

export const ConfidenceSchema = z.enum(["low", "medium", "high"]);

export const SignalTypeSchema = z.enum(["logs", "metrics", "traces", "apm"]);

export const RoleSchema = z.enum(["admin", "operator", "viewer"]);

export const JobStatusSchema = z.enum([
  "pending",
  "running",
  "completed",
  "failed",
  "canceled",
]);

// --- Envelope -------------------------------------------------------------

export const PaginationSchema = z.object({
  limit: z.number().int().nonnegative(),
  offset: z.number().int().nonnegative(),
  total: z.number().int().nonnegative(),
  next: z.string().optional(),
});

export const ProviderSummarySchema = z.object({
  mode: z.string(),
  llm: z.string(),
  observability: z.array(z.string()),
});

export const ResponseMetadataSchema = z.object({
  requestId: z.string(),
  processingTimeMs: z.number().int().nonnegative(),
  provider: ProviderSummarySchema,
  pagination: PaginationSchema.nullable().optional(),
});

export const WrapperDtoResponde = <DataSchema extends z.ZodTypeAny>(
  data: DataSchema,
) =>
  z.object({
    data,
    metadata: ResponseMetadataSchema,
  });

export const ErrorResponseSchema = z.object({
  code: z.string(),
  message: z.string(),
  details: z
    .array(
      z.object({
        field: z.string(),
        rule: z.string().optional(),
        message: z.string().optional(),
      }),
    )
    .optional(),
});

// --- Auth + Profile -------------------------------------------------------

export const UserResponseSchema = z.object({
  id: z.string(),
  email: z.string().email(),
  role: RoleSchema,
  isActive: z.boolean(),
  createdAt: Iso8601,
  updatedAt: Iso8601,
  lastLoginAt: Iso8601.nullable().optional(),
});

export const SessionResponseSchema = z.object({
  user: UserResponseSchema,
  csrfToken: z.string(),
  expiresAt: Iso8601,
});

// --- Setup ----------------------------------------------------------------

export const SetupStatusResponseSchema = z.object({
  adminExists: z.boolean(),
  llmConfigured: z.boolean(),
  observabilityConfigured: z.boolean(),
  setupCompleted: z.boolean(),
  state: z.enum(["pending", "completed", "degraded"]),
});

// --- API keys -------------------------------------------------------------

export const APIKeyScopeSchema = z.enum([
  "analysis:read",
  "analysis:write",
  "chat:write",
  "admin:read",
  "admin:write",
]);

export const APIKeyResponseSchema = z.object({
  id: z.string(),
  name: z.string(),
  description: z.string().optional(),
  scopes: z.array(APIKeyScopeSchema),
  masked: z.string(),
  createdAt: Iso8601,
  expiresAt: Iso8601.nullable().optional(),
  lastUsedAt: Iso8601.nullable().optional(),
  revokedAt: Iso8601.nullable().optional(),
});

export const IssuedAPIKeyResponseSchema = APIKeyResponseSchema.extend({
  secret: z.string(),
});

// --- Provider / LLM configs ----------------------------------------------

export const ProviderConfigResponseSchema = z.object({
  id: z.string(),
  type: z.string(),
  name: z.string(),
  url: z.string().url(),
  timeoutMs: z.number().int().positive(),
  signals: z.array(z.string()),
  options: z.record(z.string()).optional(),
  credentialsMasked: z.string().optional(),
  hasCredentials: z.boolean(),
  isActive: z.boolean(),
  createdAt: Iso8601,
  updatedAt: Iso8601,
});

export const LLMConfigResponseSchema = z.object({
  id: z.string(),
  type: z.string(),
  name: z.string(),
  baseUrl: z.string().url(),
  model: z.string(),
  timeoutMs: z.number().int().positive(),
  options: z.record(z.string()).optional(),
  apiKeyMasked: z.string().optional(),
  hasApiKey: z.boolean(),
  isActive: z.boolean(),
  createdAt: Iso8601,
  updatedAt: Iso8601,
});

export const TestConnectionResponseSchema = z.object({
  reached: z.boolean(),
  latencyMs: z.number().int().nonnegative(),
  error: z.string().optional(),
});

// --- Analyses + chat ------------------------------------------------------

export const EvidenceSchema = z.object({
  id: z.string(),
  signal: SignalTypeSchema,
  service: z.string(),
  source: z.string(),
  name: z.string(),
  summary: z.string(),
  observed: Iso8601,
  score: z.number(),
  confidence: z.number().optional(),
  unit: z.string().optional(),
  reference: z.string().optional(),
  provider: z.string().optional(),
  query: z.string().optional(),
  attributes: z.record(z.string()).optional(),
  redactedFields: z.array(z.string()).optional(),
});

export const RootCauseHypothesisSchema = z.object({
  cause: z.string(),
  evidence: z.array(z.string()),
  confidence: ConfidenceSchema,
});

export const RecommendationSchema = z.object({
  action: z.string(),
  rationale: z.string(),
  priority: z.number().int(),
});

export const AnalysisResponseSchema = z.object({
  id: z.string(),
  summary: z.string(),
  severity: SeveritySchema,
  confidence: ConfidenceSchema,
  affectedServices: z.array(z.string()),
  traceId: z.string().optional(),
  createdAt: Iso8601,
  evidence: z.array(EvidenceSchema),
  possibleRootCauses: z.array(RootCauseHypothesisSchema),
  recommendedActions: z.array(RecommendationSchema),
});

export const AnalysisJobAcceptedSchema = z.object({
  jobId: z.string(),
  status: JobStatusSchema,
  statusUrl: z.string(),
});

export const AnalysisJobStatusSchema = z.object({
  jobId: z.string(),
  status: JobStatusSchema,
  phase: z.string().optional(),
  progressPercent: z.number().int().optional(),
  analysisId: z.string().optional(),
  error: z.string().optional(),
  acceptedAt: Iso8601,
  startedAt: Iso8601.nullable().optional(),
  finishedAt: Iso8601.nullable().optional(),
});

export const ChatResponseSchema = z.object({
  analysisId: z.string(),
  answer: z.string(),
  evidence: z.array(z.string()),
  citations: z
    .array(
      z.object({
        evidenceId: z.string(),
        quote: z.string().optional(),
      }),
    )
    .optional(),
});

// --- Webhooks -------------------------------------------------------------

export const WebhookResponseSchema = z.object({
  id: z.string(),
  name: z.string(),
  url: z.string().url(),
  event: z.string(),
  secret: z.string().optional(),
  createdAt: Iso8601,
  disabledAt: Iso8601.nullable().optional(),
});

export const WebhookDeliverySchema = z.object({
  id: z.string(),
  subscriptionId: z.string(),
  event: z.string(),
  status: z.enum(["pending", "delivered", "failed"]),
  attempt: z.number().int().nonnegative(),
  lastError: z.string().optional(),
  responseStatus: z.number().int().optional(),
  nextAttemptAt: Iso8601.nullable().optional(),
  deliveredAt: Iso8601.nullable().optional(),
  createdAt: Iso8601,
  updatedAt: Iso8601,
  payload: z.unknown().optional(),
});

// --- Audit ----------------------------------------------------------------

export const AuditEntrySchema = z.object({
  id: z.number().int(),
  requestId: z.string(),
  apiKeyId: z.string(),
  actor: z.string(),
  method: z.string(),
  path: z.string(),
  status: z.number().int(),
  durationMs: z.number().int(),
  remote: z.string(),
  action: z.string().optional(),
  resourceType: z.string().optional(),
  resourceId: z.string().optional(),
  metadata: z.record(z.string()).optional(),
  createdAt: Iso8601,
});

// --- Type exports for the frontend ---------------------------------------

export type Severity = z.infer<typeof SeveritySchema>;
export type Confidence = z.infer<typeof ConfidenceSchema>;
export type Role = z.infer<typeof RoleSchema>;
export type SignalType = z.infer<typeof SignalTypeSchema>;
export type JobStatus = z.infer<typeof JobStatusSchema>;
export type APIKeyScope = z.infer<typeof APIKeyScopeSchema>;

export type UserResponse = z.infer<typeof UserResponseSchema>;
export type SessionResponse = z.infer<typeof SessionResponseSchema>;
export type SetupStatusResponse = z.infer<typeof SetupStatusResponseSchema>;
export type APIKeyResponse = z.infer<typeof APIKeyResponseSchema>;
export type IssuedAPIKeyResponse = z.infer<typeof IssuedAPIKeyResponseSchema>;
export type ProviderConfigResponse = z.infer<typeof ProviderConfigResponseSchema>;
export type LLMConfigResponse = z.infer<typeof LLMConfigResponseSchema>;
export type TestConnectionResponse = z.infer<typeof TestConnectionResponseSchema>;
export type Evidence = z.infer<typeof EvidenceSchema>;
export type AnalysisResponse = z.infer<typeof AnalysisResponseSchema>;
export type AnalysisJobAccepted = z.infer<typeof AnalysisJobAcceptedSchema>;
export type AnalysisJobStatus = z.infer<typeof AnalysisJobStatusSchema>;
export type ChatResponse = z.infer<typeof ChatResponseSchema>;
export type WebhookResponse = z.infer<typeof WebhookResponseSchema>;
export type WebhookDelivery = z.infer<typeof WebhookDeliverySchema>;
export type AuditEntry = z.infer<typeof AuditEntrySchema>;
export type ErrorResponse = z.infer<typeof ErrorResponseSchema>;
