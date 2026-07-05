package adapter

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// PSPPaymentRequest is the normalized input to any PSP.
type PSPPaymentRequest struct {
	Amount      int64
	Currency    string
	Description string
	OrderID     string
	CustomerEmail string
	Metadata    map[string]string
}

// PSPPaymentResponse is the normalized output from any PSP.
type PSPPaymentResponse struct {
	PSPPaymentID string
	Status       string // raw PSP status string
	RawResponse  map[string]interface{}
}

type PSPCaptureResponse struct {
	PSPPaymentID string
	Status       string
}

type PSPRefundResponse struct {
	PSPRefundID  string
	PSPPaymentID string
	Amount       int64
	Status       string
}

type PSPStatusResponse struct {
	PSPPaymentID string
	Status       string
	Amount       int64
	Currency     string
}

// PSPAdapter is the contract every payment provider must satisfy.
type PSPAdapter interface {
	Name() string

	CreatePayment(ctx context.Context, req PSPPaymentRequest) (*PSPPaymentResponse, error)
	CapturePayment(ctx context.Context, pspPaymentID string, amount int64, currency string) (*PSPCaptureResponse, error)
	RefundPayment(ctx context.Context, pspPaymentID string, amount int64) (*PSPRefundResponse, error)
	CancelPayment(ctx context.Context, pspPaymentID string) error
	GetPaymentStatus(ctx context.Context, pspPaymentID string) (*PSPStatusResponse, error)

	VerifyWebhookSignature(payload []byte, signature string) error
}

// traceAttrs is a convenience function that wraps attributes into a SpanStartOption.
func traceAttrs(attrs ...attribute.KeyValue) trace.SpanStartOption {
	return trace.WithAttributes(attrs...)
}

