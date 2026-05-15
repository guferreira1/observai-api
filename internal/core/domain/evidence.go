package domain

import (
	"sort"
	"strings"
	"time"
)

// LogEvidence is an Evidence whose Signal is SignalLogs. The alias gives
// callers a domain-specific name that does not require touching the
// jsonb-stored Evidence shape.
type LogEvidence Evidence

// MetricEvidence is an Evidence whose Signal is SignalMetrics.
type MetricEvidence Evidence

// TraceEvidence is an Evidence whose Signal is SignalTraces.
type TraceEvidence Evidence

// APMEvidence is an Evidence whose Signal is SignalAPM.
type APMEvidence Evidence

// DependencyEvidence describes a service-to-service edge derived from
// traces, APM data or signal correlation.
type DependencyEvidence Evidence

// Anomaly describes a deviation detected over normalized evidence (rate
// spike, latency drift, error explosion). Anomalies are stored alongside
// Evidence and consumed by analysis hypotheses.
type Anomaly struct {
	Name        string
	Service     string
	Signal      SignalType
	Description string
	Severity    Severity
	Confidence  float64
	Observed    time.Time
	Evidence    []string
	Attributes  map[string]string
}

// NewLogEvidence wraps the supplied Evidence enforcing the Signal class.
func NewLogEvidence(evidence Evidence) LogEvidence {
	evidence.Signal = SignalLogs
	return LogEvidence(evidence)
}

// NewMetricEvidence wraps the supplied Evidence enforcing the Signal class.
func NewMetricEvidence(evidence Evidence) MetricEvidence {
	evidence.Signal = SignalMetrics
	return MetricEvidence(evidence)
}

// NewTraceEvidence wraps the supplied Evidence enforcing the Signal class.
func NewTraceEvidence(evidence Evidence) TraceEvidence {
	evidence.Signal = SignalTraces
	return TraceEvidence(evidence)
}

// NewAPMEvidence wraps the supplied Evidence enforcing the Signal class.
func NewAPMEvidence(evidence Evidence) APMEvidence {
	evidence.Signal = SignalAPM
	return APMEvidence(evidence)
}

// CorrelationKey returns a deterministic key that can be used to group
// evidence pieces that observe the same incident. Preference order
// (highest to lowest specificity):
//
//   - traceId (when known)
//   - service + spanId
//   - service + observed minute bucket
//   - service alone
//
// Adapters that already emit a traceId/spanId via Attributes participate
// automatically; others can populate the keys manually.
func (evidence Evidence) CorrelationKey() string {
	traceID := strings.TrimSpace(evidence.Attributes["traceId"])
	spanID := strings.TrimSpace(evidence.Attributes["spanId"])
	service := strings.TrimSpace(evidence.Service)
	switch {
	case traceID != "":
		return "trace:" + traceID
	case service != "" && spanID != "":
		return "span:" + service + ":" + spanID
	case service != "" && !evidence.Observed.IsZero():
		return "minute:" + service + ":" + evidence.Observed.UTC().Format("2006-01-02T15:04")
	case service != "":
		return "service:" + service
	}
	return ""
}

// CorrelatedEvidenceGroup buckets evidence by correlation key.
type CorrelatedEvidenceGroup struct {
	Key      string
	Evidence []Evidence
}

// GroupEvidenceByCorrelation returns groups of evidence sharing a
// correlation key. Evidence without a key is emitted in a single group
// keyed by "uncorrelated".
func GroupEvidenceByCorrelation(evidence []Evidence) []CorrelatedEvidenceGroup {
	if len(evidence) == 0 {
		return nil
	}
	buckets := make(map[string][]Evidence, len(evidence))
	for _, item := range evidence {
		key := item.CorrelationKey()
		if key == "" {
			key = "uncorrelated"
		}
		buckets[key] = append(buckets[key], item)
	}
	groups := make([]CorrelatedEvidenceGroup, 0, len(buckets))
	for key, items := range buckets {
		groups = append(groups, CorrelatedEvidenceGroup{Key: key, Evidence: items})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if len(groups[i].Evidence) != len(groups[j].Evidence) {
			return len(groups[i].Evidence) > len(groups[j].Evidence)
		}
		return groups[i].Key < groups[j].Key
	})
	return groups
}
