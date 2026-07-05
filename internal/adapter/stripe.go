package adapter

import (
	"context"
	"fmt"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/paymentintent"
	"github.com/stripe/stripe-go/v82/refund"
	"github.com/stripe/stripe-go/v82/webhook"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/pkg/tracing"
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

var stripeTracer = tracing.Tracer("psp.stripe")

func NewStripeAdapter(secretKey, webhookSecret string) *StripeAdapter {
	stripe.Key = secretKey // NOTE: sets package-level global — see struct doc
	return &StripeAdapter{
		webhookSecret: webhookSecret,
	}
}

func (s *StripeAdapter) Name() string { return "stripe" }

func (s *StripeAdapter) CreatePayment(ctx context.Context, req PSPPaymentRequest) (*PSPPaymentResponse, error) {
	ctx, span := stripeTracer.Start(ctx, "psp.stripe.create_payment",
		traceAttrs(attribute.Int64("psp.amount", req.Amount), attribute.String("psp.currency", req.Currency)),
	)
	defer span.End()

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
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("stripe create payment: %w", err)
	}

	span.SetAttributes(attribute.String("psp.payment_id", pi.ID), attribute.String("psp.status", string(pi.Status)))

	return &PSPPaymentResponse{
		PSPPaymentID: pi.ID,
		Status:       string(pi.Status),
		RawResponse: map[string]interface{}{
			"client_secret":  pi.ClientSecret,
			"capture_method": string(pi.CaptureMethod),
		},
	}, nil
}

func (s *StripeAdapter) CapturePayment(ctx context.Context, pspPaymentID string, amount int64, currency string) (*PSPCaptureResponse, error) {
	_, span := stripeTracer.Start(ctx, "psp.stripe.capture",
		traceAttrs(attribute.String("psp.payment_id", pspPaymentID), attribute.Int64("psp.amount", amount)),
	)
	defer span.End()

	params := &stripe.PaymentIntentCaptureParams{}
	if amount > 0 {
		params.AmountToCapture = stripe.Int64(amount)
	}

	pi, err := paymentintent.Capture(pspPaymentID, params)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("stripe capture: %w", err)
	}

	span.SetAttributes(attribute.String("psp.status", string(pi.Status)))

	return &PSPCaptureResponse{
		PSPPaymentID: pi.ID,
		Status:       string(pi.Status),
	}, nil
}

func (s *StripeAdapter) RefundPayment(ctx context.Context, pspPaymentID string, amount int64) (*PSPRefundResponse, error) {
	_, span := stripeTracer.Start(ctx, "psp.stripe.refund",
		traceAttrs(attribute.String("psp.payment_id", pspPaymentID), attribute.Int64("psp.refund_amount", amount)),
	)
	defer span.End()

	params := &stripe.RefundParams{
		PaymentIntent: stripe.String(pspPaymentID),
	}
	if amount > 0 {
		params.Amount = stripe.Int64(amount)
	}

	ref, err := refund.New(params)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("stripe refund: %w", err)
	}

	span.SetAttributes(attribute.String("psp.refund_id", ref.ID), attribute.String("psp.status", string(ref.Status)))

	return &PSPRefundResponse{
		PSPRefundID:  ref.ID,
		PSPPaymentID: pspPaymentID,
		Amount:       ref.Amount,
		Status:       string(ref.Status),
	}, nil
}

func (s *StripeAdapter) CancelPayment(ctx context.Context, pspPaymentID string) error {
	_, span := stripeTracer.Start(ctx, "psp.stripe.cancel",
		traceAttrs(attribute.String("psp.payment_id", pspPaymentID)),
	)
	defer span.End()

	params := &stripe.PaymentIntentCancelParams{}
	_, err := paymentintent.Cancel(pspPaymentID, params)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("stripe cancel: %w", err)
	}
	return nil
}

func (s *StripeAdapter) GetPaymentStatus(ctx context.Context, pspPaymentID string) (*PSPStatusResponse, error) {
	_, span := stripeTracer.Start(ctx, "psp.stripe.get_status",
		traceAttrs(attribute.String("psp.payment_id", pspPaymentID)),
	)
	defer span.End()

	pi, err := paymentintent.Get(pspPaymentID, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
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
