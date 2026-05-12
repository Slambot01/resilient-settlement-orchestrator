package service

import (
	"context"
	"fmt"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/adapter"
	"github.com/Slambot01/resilient-settlement-orchestrator/internal/models"
)

type PaymentRouter struct {
	adapters map[models.PSPName]adapter.PSPAdapter
	rules    []models.RoutingRule
}

func NewPaymentRouter() *PaymentRouter {
	return &PaymentRouter{
		adapters: make(map[models.PSPName]adapter.PSPAdapter),
		rules:    []models.RoutingRule{},
	}
}

// RegisterAdapter adds a PSP adapter to the router.
func (r *PaymentRouter) RegisterAdapter(name models.PSPName, a adapter.PSPAdapter) {
	r.adapters[name] = a
}

// LoadRules loads the routing configuration. In production, this might come from a DB or ConfigMap.
func (r *PaymentRouter) LoadRules(rules []models.RoutingRule) {
	r.rules = rules
}

// Route selects the best PSP for a given payment based on defined rules.
func (r *PaymentRouter) Route(ctx context.Context, req models.CreatePaymentRequest) (*models.RoutingDecision, adapter.PSPAdapter, error) {
	// 1. Check if the merchant explicitly preferred a PSP
	if req.RoutingPreferences != nil && req.RoutingPreferences.PreferredPSP != "" {
		pref := models.PSPName(req.RoutingPreferences.PreferredPSP)
		if adp, ok := r.adapters[pref]; ok {
			return &models.RoutingDecision{
				SelectedPSP: pref,
				Reason:      "merchant_preference",
				RuleName:    "explicit_override",
			}, adp, nil
		}
	}

	// 2. Evaluate rules in order of priority
	for _, rule := range r.rules {
		if r.matches(rule.Conditions, req) {
			// In a full implementation, we'd check Circuit Breaker state here.
			// If primary is OPEN, we fall back to FallbackPSP.
			// For now, we assume primary is healthy.
			if adp, ok := r.adapters[rule.PrimaryPSP]; ok {
				return &models.RoutingDecision{
					SelectedPSP: rule.PrimaryPSP,
					Reason:      "rule_match",
					RuleName:    rule.Name,
				}, adp, nil
			}
		}
	}

	// 3. Fallback to default (Mock for testing, or fail)
	if adp, ok := r.adapters[models.PSPMock]; ok {
		return &models.RoutingDecision{
			SelectedPSP: models.PSPMock,
			Reason:      "default_fallback",
			RuleName:    "no_rule_matched",
		}, adp, nil
	}

	return nil, nil, fmt.Errorf("no suitable PSP found for payment")
}

// matches evaluates if a payment request matches a set of routing conditions.
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
	// Note: We'd extract Country from req.PaymentMethodDetails, etc.
	// Simplifying for the scaffolding.
	return true
}
