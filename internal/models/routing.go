package models

import "time"

type PSPName string

const (
	PSPStripe   PSPName = "stripe"
	PSPRazorpay PSPName = "razorpay"
	PSPMock     PSPName = "mock"
)

type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

type RoutingRule struct {
	Name        string            `json:"name"`
	Conditions  RoutingConditions `json:"conditions"`
	PrimaryPSP  PSPName           `json:"primary_psp"`
	FallbackPSP PSPName           `json:"fallback_psp,omitempty"`
	Priority    int               `json:"priority"`
}

type RoutingConditions struct {
	MinAmount *int64  `json:"amount_gte,omitempty"`
	MaxAmount *int64  `json:"amount_lte,omitempty"`
	Currency  string  `json:"currency,omitempty"`
	Country   string  `json:"country,omitempty"`
	CountryNe string  `json:"country_ne,omitempty"`
}

type PSPHealth struct {
	PSP           PSPName      `json:"psp"`
	SuccessRate   float64      `json:"success_rate"`
	AvgLatencyMs  int64        `json:"avg_latency_ms"`
	CircuitState  CircuitState `json:"circuit_state"`
	TotalRequests int64        `json:"total_requests"`
	FailedRequests int64       `json:"failed_requests"`
	LastFailureAt *time.Time   `json:"last_failure_at,omitempty"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

// RoutingDecision captures why a particular PSP was selected.
type RoutingDecision struct {
	SelectedPSP PSPName `json:"selected_psp"`
	FallbackPSP PSPName `json:"fallback_psp,omitempty"`
	Reason      string  `json:"reason"`
	RuleName    string  `json:"rule_name"`
}
