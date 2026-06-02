package service

import (
	"fmt"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/models"
)

// validTransitions defines every legal status change.
// Any transition not listed here is rejected.
var validTransitions = map[models.PaymentStatus][]models.PaymentStatus{
	models.PaymentStatusCreated:    {models.PaymentStatusPending, models.PaymentStatusAuthorized, models.PaymentStatusFailed},
	models.PaymentStatusPending:    {models.PaymentStatusAuthorized, models.PaymentStatusCaptured, models.PaymentStatusFailed},
	models.PaymentStatusAuthorized: {models.PaymentStatusCaptured, models.PaymentStatusCancelled, models.PaymentStatusFailed},
	models.PaymentStatusCaptured:   {models.PaymentStatusRefunded, models.PaymentStatusPartiallyRefunded},
	models.PaymentStatusPartiallyRefunded: {models.PaymentStatusRefunded, models.PaymentStatusPartiallyRefunded},
}

// ValidateTransition checks whether moving from `from` to `to` is permitted.
func ValidateTransition(from, to models.PaymentStatus) error {
	allowed, exists := validTransitions[from]
	if !exists {
		return fmt.Errorf("no transitions allowed from terminal state %q", from)
	}

	for _, s := range allowed {
		if s == to {
			return nil
		}
	}

	return fmt.Errorf("invalid transition: %s → %s", from, to)
}

// CanTransition is the boolean variant of ValidateTransition.
func CanTransition(from, to models.PaymentStatus) bool {
	return ValidateTransition(from, to) == nil
}

// AllowedTransitions returns the set of states reachable from `current`.
func AllowedTransitions(current models.PaymentStatus) []models.PaymentStatus {
	return validTransitions[current]
}
