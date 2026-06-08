package adapter

import (
	"context"
	"fmt"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/paymentintent"
	"github.com/stripe/stripe-go/v82/refund"
	"github.com/stripe/stripe-go/v82/webhook"
)

// StripeAdapter wraps the Stripe SDK behind the PSPAdapter interface.
// Uses test/sandbox keys — never handles raw card data.
//
// WARNING: stripe.Key is a process-level global. Only one StripeAdapter with one
// secret key should exist per process. If multiple Stripe accounts are needed,
// use stripe.BackendConfiguration with per-request API keys instead.
type StripeAdapter struct {
	webhookSecret string
}

func NewStripeAdapter(secretKey, webhookSecret string) *StripeAdapter {
	stripe.Key = secretKey // NOTE: sets package-level global — see struct doc
	return &StripeAdapter{
		webhookSecret: webhookSecret,
	}
}

func (s *StripeAdapter) Name() string { return "stripe" }

func (s *StripeAdapter) CreatePayment(ctx context.Context, req PSPPaymentRequest) (*PSPPaymentResponse, error) {
	params := &stripe.PaymentIntentParams{
		Amount:        stripe.Int64(req.Amount),
		Currency:      stripe.String(req.Currency),
		CaptureMethod: stripe.String("manual"),
		Description:   stripe.String(req.Description),
	}

	if req.CustomerEmail != "" {
		params.ReceiptEmail = stripe.String(req.CustomerEmail)
	}

	for k, v := range req.Metadata {
		params.AddMetadata(k, v)
	}
	params.AddMetadata("order_id", req.OrderID)

	pi, err := paymentintent.New(params)
	if err != nil {
		return nil, fmt.Errorf("stripe create payment: %w", err)
	}

	return &PSPPaymentResponse{
		PSPPaymentID: pi.ID,
		Status:       string(pi.Status),
		RawResponse: map[string]interface{}{
			"client_secret":  pi.ClientSecret,
			"capture_method": string(pi.CaptureMethod),
		},
	}, nil
}

func (s *StripeAdapter) CapturePayment(ctx context.Context, pspPaymentID string, amount int64) (*PSPCaptureResponse, error) {
	params := &stripe.PaymentIntentCaptureParams{}
	if amount > 0 {
		params.AmountToCapture = stripe.Int64(amount)
	}

	pi, err := paymentintent.Capture(pspPaymentID, params)
	if err != nil {
		return nil, fmt.Errorf("stripe capture: %w", err)
	}

	return &PSPCaptureResponse{
		PSPPaymentID: pi.ID,
		Status:       string(pi.Status),
	}, nil
}

func (s *StripeAdapter) RefundPayment(ctx context.Context, pspPaymentID string, amount int64) (*PSPRefundResponse, error) {
	params := &stripe.RefundParams{
		PaymentIntent: stripe.String(pspPaymentID),
	}
	if amount > 0 {
		params.Amount = stripe.Int64(amount)
	}

	ref, err := refund.New(params)
	if err != nil {
		return nil, fmt.Errorf("stripe refund: %w", err)
	}

	return &PSPRefundResponse{
		PSPRefundID:  ref.ID,
		PSPPaymentID: pspPaymentID,
		Amount:       ref.Amount,
		Status:       string(ref.Status),
	}, nil
}

func (s *StripeAdapter) CancelPayment(ctx context.Context, pspPaymentID string) error {
	params := &stripe.PaymentIntentCancelParams{}
	_, err := paymentintent.Cancel(pspPaymentID, params)
	if err != nil {
		return fmt.Errorf("stripe cancel: %w", err)
	}
	return nil
}

func (s *StripeAdapter) GetPaymentStatus(ctx context.Context, pspPaymentID string) (*PSPStatusResponse, error) {
	pi, err := paymentintent.Get(pspPaymentID, nil)
	if err != nil {
		return nil, fmt.Errorf("stripe get status: %w", err)
	}

	return &PSPStatusResponse{
		PSPPaymentID: pi.ID,
		Status:       string(pi.Status),
		Amount:       pi.Amount,
		Currency:     string(pi.Currency),
	}, nil
}

func (s *StripeAdapter) VerifyWebhookSignature(payload []byte, signature string) error {
	_, err := webhook.ConstructEvent(payload, signature, s.webhookSecret)
	if err != nil {
		return fmt.Errorf("stripe: invalid webhook signature: %w", err)
	}
	return nil
}
