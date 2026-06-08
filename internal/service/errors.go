package service

import "errors"

// Sentinel errors for the service layer.
// Use errors.Is() in handlers to distinguish error types without fragile string matching.
var (
	ErrPaymentNotFound  = errors.New("payment not found")
	ErrAccountNotFound  = errors.New("account not found")
	ErrInvalidState     = errors.New("invalid payment state for this operation")
	ErrNotAuthorized    = errors.New("payment not in authorized state")
)
