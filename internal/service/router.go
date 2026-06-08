package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/adapter"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/models"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/pkg/circuitbreaker"
)

type PaymentRouter struct {
	mu       sync.RWMutex
	adapters map[models.PSPName]adapter.PSPAdapter
	breakers map[models.PSPName]*circuitbreaker.CircuitBreaker
	rules    []models.RoutingRule
}

func NewPaymentRouter() *PaymentRouter {
	return &PaymentRouter{
		adapters: make(map[models.PSPName]adapter.PSPAdapter),
		breakers: make(map[models.PSPName]*circuitbreaker.CircuitBreaker),
		rules:    []models.RoutingRule{},
	}
}

func (r *PaymentRouter) RegisterAdapter(name models.PSPName, a adapter.PSPAdapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[name] = a
	r.breakers[name] = circuitbreaker.New(string(name), circuitbreaker.DefaultConfig())
}

func (r *PaymentRouter) LoadRules(rules []models.RoutingRule) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rules = rules
}

// GetAdapter returns the PSP adapter by name.
func (r *PaymentRouter) GetAdapter(name models.PSPName) (adapter.PSPAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	adp, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("PSP adapter %q not found", name)
	}
	return adp, nil
}

// GetAdapters returns a string-keyed map of all registered PSP adapters.
// Used by webhook and reconciliation services for PSP lookup.
func (r *PaymentRouter) GetAdapters() map[string]adapter.PSPAdapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]adapter.PSPAdapter, len(r.adapters))
	for name, adp := range r.adapters {
		result[string(name)] = adp
	}
	return result
}

// RecordSuccess signals a successful PSP call to the circuit breaker.
func (r *PaymentRouter) RecordSuccess(psp models.PSPName) {
	r.mu.RLock()
	cb, ok := r.breakers[psp]
	r.mu.RUnlock()
	if ok {
		cb.RecordSuccess()
	}
}

// RecordFailure signals a failed PSP call to the circuit breaker.
func (r *PaymentRouter) RecordFailure(psp models.PSPName) {
	r.mu.RLock()
	cb, ok := r.breakers[psp]
	r.mu.RUnlock()
	if ok {
		cb.RecordFailure()
	}
}

// GetBreakerStats returns circuit breaker stats for all PSPs.
func (r *PaymentRouter) GetBreakerStats() map[string]circuitbreaker.Stats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	stats := make(map[string]circuitbreaker.Stats)
	for name, cb := range r.breakers {
		stats[string(name)] = cb.Stats()
	}
	return stats
}

// Route selects the best PSP, respecting circuit breaker state.
// If the primary PSP's breaker is open, it falls back automatically.
func (r *PaymentRouter) Route(ctx context.Context, req models.CreatePaymentRequest) (*models.RoutingDecision, adapter.PSPAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Explicit merchant preference (bypass rules, but still check breaker)
	if req.RoutingPreferences != nil && req.RoutingPreferences.PreferredPSP != "" {
		pref := models.PSPName(req.RoutingPreferences.PreferredPSP)
		if adp, ok := r.adapters[pref]; ok {
			if r.isAvailable(pref) {
				return &models.RoutingDecision{
					SelectedPSP: pref,
					Reason:      "merchant_preference",
					RuleName:    "explicit_override",
				}, adp, nil
			}
			slog.Warn("preferred PSP circuit is open, falling through to rules",
				slog.String("psp", string(pref)),
			)
		}
	}

	// Evaluate rules — check primary first, then fallback
	for _, rule := range r.rules {
		if !r.matches(rule.Conditions, req) {
			continue
		}

		// Try primary PSP
		if r.isAvailable(rule.PrimaryPSP) {
			if adp, ok := r.adapters[rule.PrimaryPSP]; ok {
				return &models.RoutingDecision{
					SelectedPSP: rule.PrimaryPSP,
					FallbackPSP: rule.FallbackPSP,
					Reason:      "rule_match",
					RuleName:    rule.Name,
				}, adp, nil
			}
		}

		// Primary is down — try fallback
		if rule.FallbackPSP != "" && r.isAvailable(rule.FallbackPSP) {
			if adp, ok := r.adapters[rule.FallbackPSP]; ok {
				slog.Warn("primary PSP circuit open, using fallback",
					slog.String("primary", string(rule.PrimaryPSP)),
					slog.String("fallback", string(rule.FallbackPSP)),
				)
				return &models.RoutingDecision{
					SelectedPSP: rule.FallbackPSP,
					Reason:      "circuit_breaker_fallback",
					RuleName:    rule.Name,
				}, adp, nil
			}
		}
	}

	// Last resort — any available adapter
	for name, adp := range r.adapters {
		if r.isAvailable(name) {
			return &models.RoutingDecision{
				SelectedPSP: name,
				Reason:      "last_resort_fallback",
				RuleName:    "no_rule_matched",
			}, adp, nil
		}
	}

	return nil, nil, fmt.Errorf("all PSPs are unavailable")
}

func (r *PaymentRouter) isAvailable(psp models.PSPName) bool {
	cb, ok := r.breakers[psp]
	if !ok {
		return true
	}
	return cb.Allow()
}

func (r *PaymentRouter) matches(cond models.RoutingConditions, req models.CreatePaymentRequest) bool {
	if cond.MinAmount != nil && req.Amount < *cond.MinAmount {
		return false
	}
	if cond.MaxAmount != nil && req.Amount > *cond.MaxAmount {
		return false
	}
	if cond.Currency != "" && req.Currency != cond.Currency {
		return false
	}
	return true
}
