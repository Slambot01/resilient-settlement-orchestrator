package models

import (
	"time"
)

type PaymentStatus string

const (
	PaymentStatusCreated          PaymentStatus = "created"
	PaymentStatusPending          PaymentStatus = "pending"
	PaymentStatusAuthorized       PaymentStatus = "authorized"
	PaymentStatusCaptured         PaymentStatus = "captured"
	PaymentStatusFailed           PaymentStatus = "failed"
	PaymentStatusCancelled        PaymentStatus = "cancelled"
	PaymentStatusRefunded         PaymentStatus = "refunded"
	PaymentStatusPartiallyRefunded PaymentStatus = "partially_refunded"
)

func (s PaymentStatus) IsValid() bool {
	switch s {
	case PaymentStatusCreated, PaymentStatusPending, PaymentStatusAuthorized,
		PaymentStatusCaptured, PaymentStatusFailed, PaymentStatusCancelled,
		PaymentStatusRefunded, PaymentStatusPartiallyRefunded:
		return true
	}
	return false
}

// IsTerminal returns true if no further transitions are possible.
func (s PaymentStatus) IsTerminal() bool {
	switch s {
	case PaymentStatusFailed, PaymentStatusCancelled, PaymentStatusRefunded:
		return true
	}
	return false
}

type Payment struct {
	ID         string        `json:"id"`
	MerchantID string        `json:"merchant_id"`
	OrderID    string        `json:"order_id"`
	Amount     int64         `json:"amount"`
	Currency   string        `json:"currency"`
	Status     PaymentStatus `json:"status"`

	PSP          string `json:"psp,omitempty"`
	PSPPaymentID string `json:"psp_payment_id,omitempty"`

	CustomerEmail string `json:"customer_email,omitempty"`
	CustomerPhone string `json:"customer_phone,omitempty"`
	CustomerName  string `json:"customer_name,omitempty"`

	PaymentMethodType    string                 `json:"payment_method_type,omitempty"`
	PaymentMethodDetails map[string]interface{} `json:"payment_method_details,omitempty"`

	Metadata map[string]interface{} `json:"metadata,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy string    `json:"created_by,omitempty"`
}

type StateTransition struct {
	ID          string        `json:"id"`
	PaymentID   string        `json:"payment_id"`
	FromStatus  PaymentStatus `json:"from_status"`
	ToStatus    PaymentStatus `json:"to_status"`
	Reason      string        `json:"reason,omitempty"`
	TriggeredBy string        `json:"triggered_by"`
	CreatedAt   time.Time     `json:"created_at"`
}

// CreatePaymentRequest is the inbound payload for POST /v1/payments.
type CreatePaymentRequest struct {
	MerchantID  string `json:"merchant_id" validate:"required"`
	OrderID     string `json:"order_id" validate:"required"`
	Amount      int64  `json:"amount" validate:"required,gt=0"`
	Currency    string `json:"currency" validate:"required,len=3"`

	CustomerEmail string `json:"customer_email" validate:"omitempty,email"`
	CustomerPhone string `json:"customer_phone" validate:"omitempty"`
	CustomerName  string `json:"customer_name" validate:"omitempty"`

	PaymentMethodType    string                 `json:"payment_method_type" validate:"omitempty"`
	PaymentMethodDetails map[string]interface{} `json:"payment_method_details,omitempty"`

	RoutingPreferences *RoutingPreferences    `json:"routing_preferences,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
}

type RoutingPreferences struct {
	PreferredPSP string `json:"preferred_psp,omitempty"`
}

type RefundRequest struct {
	Amount int64  `json:"amount" validate:"required,gt=0"`
	Reason string `json:"reason" validate:"omitempty"`
}

// PaymentResponse wraps a payment with its state history for API output.
type PaymentResponse struct {
	Payment
	StateTransitions []StateTransition `json:"state_transitions,omitempty"`
}

// PaymentListResponse supports paginated listing.
type PaymentListResponse struct {
	Payments []Payment `json:"payments"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
