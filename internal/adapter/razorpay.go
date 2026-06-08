package adapter

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	razorpay "github.com/razorpay/razorpay-go"
)

// RazorpayAdapter wraps the Razorpay SDK behind the PSPAdapter interface.
// Razorpay uses an Order → Payment → Capture flow.
type RazorpayAdapter struct {
	client        *razorpay.Client
	webhookSecret string
}

func NewRazorpayAdapter(keyID, keySecret, webhookSecret string) *RazorpayAdapter {
	client := razorpay.NewClient(keyID, keySecret)
	return &RazorpayAdapter{
		client:        client,
		webhookSecret: webhookSecret,
	}
}

func (r *RazorpayAdapter) Name() string { return "razorpay" }

func (r *RazorpayAdapter) CreatePayment(ctx context.Context, req PSPPaymentRequest) (*PSPPaymentResponse, error) {
	// Razorpay requires creating an Order first, then the payment happens client-side.
	data := map[string]interface{}{
		"amount":   req.Amount,
		"currency": req.Currency,
		"receipt":  req.OrderID,
		"notes": map[string]interface{}{
			"order_id":       req.OrderID,
			"customer_email": req.CustomerEmail,
		},
	}

	body, err := r.client.Order.Create(data, nil)
	if err != nil {
		return nil, fmt.Errorf("razorpay create order: %w", err)
	}

	orderID, _ := body["id"].(string)
	status, _ := body["status"].(string)

	return &PSPPaymentResponse{
		PSPPaymentID: orderID,
		Status:       status,
		RawResponse:  body,
	}, nil
}

func (r *RazorpayAdapter) CapturePayment(ctx context.Context, pspPaymentID string, amount int64, currency string) (*PSPCaptureResponse, error) {
	extra := map[string]interface{}{
		"currency": currency, // V-018 fix: use actual payment currency instead of hardcoded INR
	}

	body, err := r.client.Payment.Capture(pspPaymentID, int(amount), extra, nil)
	if err != nil {
		return nil, fmt.Errorf("razorpay capture: %w", err)
	}

	status, _ := body["status"].(string)

	return &PSPCaptureResponse{
		PSPPaymentID: pspPaymentID,
		Status:       status,
	}, nil
}

func (r *RazorpayAdapter) RefundPayment(ctx context.Context, pspPaymentID string, amount int64) (*PSPRefundResponse, error) {
	data := map[string]interface{}{
		"payment_id": pspPaymentID,
	}
	if amount > 0 {
		data["amount"] = amount
	}

	body, err := r.client.Refund.Create(data, nil)
	if err != nil {
		return nil, fmt.Errorf("razorpay refund: %w", err)
	}

	refundID, _ := body["id"].(string)
	refundAmount, _ := body["amount"].(float64)
	status, _ := body["status"].(string)

	return &PSPRefundResponse{
		PSPRefundID:  refundID,
		PSPPaymentID: pspPaymentID,
		Amount:       int64(refundAmount),
		Status:       status,
	}, nil
}

func (r *RazorpayAdapter) CancelPayment(ctx context.Context, pspPaymentID string) error {
	// Razorpay does not have a direct cancel on orders.
	// Authorized payments that are not captured expire automatically.
	return nil
}

func (r *RazorpayAdapter) GetPaymentStatus(ctx context.Context, pspPaymentID string) (*PSPStatusResponse, error) {
	body, err := r.client.Payment.Fetch(pspPaymentID, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("razorpay get status: %w", err)
	}

	status, _ := body["status"].(string)
	amount, _ := body["amount"].(float64)
	currency, _ := body["currency"].(string)

	return &PSPStatusResponse{
		PSPPaymentID: pspPaymentID,
		Status:       status,
		Amount:       int64(amount),
		Currency:     currency,
	}, nil
}

func (r *RazorpayAdapter) VerifyWebhookSignature(payload []byte, signature string) error {
	mac := hmac.New(sha256.New, []byte(r.webhookSecret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return fmt.Errorf("razorpay: invalid webhook signature")
	}
	return nil
}
