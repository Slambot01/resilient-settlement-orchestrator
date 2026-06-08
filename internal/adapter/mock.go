package adapter

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MockConfig controls simulated PSP behavior.
type MockConfig struct {
	SuccessRate    float64       // 0.0–1.0, probability of success
	LatencyMin     time.Duration // minimum simulated delay
	LatencyMax     time.Duration // maximum simulated delay
	WebhookSecret  string
}

func DefaultMockConfig() MockConfig {
	return MockConfig{
		SuccessRate: 0.95,
		LatencyMin:  50 * time.Millisecond,
		LatencyMax:  200 * time.Millisecond,
		WebhookSecret: "mock_webhook_secret_key",
	}
}

type MockPSP struct {
	cfg MockConfig
	mu  sync.Mutex
	rng *rand.Rand
}

func NewMockPSP(cfg MockConfig) *MockPSP {
	return &MockPSP{
		cfg: cfg,
		rng: rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixNano()>>32))),
	}
}

func (m *MockPSP) Name() string { return "mock" }

func (m *MockPSP) CreatePayment(ctx context.Context, req PSPPaymentRequest) (*PSPPaymentResponse, error) {
	m.simulateLatency()

	if !m.shouldSucceed() {
		return nil, fmt.Errorf("mock psp: simulated payment failure for order %s", req.OrderID)
	}

	return &PSPPaymentResponse{
		PSPPaymentID: "mock_pay_" + uuid.NewString()[:8],
		Status:       "authorized",
		RawResponse: map[string]interface{}{
			"mock":   true,
			"amount": req.Amount,
		},
	}, nil
}

func (m *MockPSP) CapturePayment(ctx context.Context, pspPaymentID string, amount int64, currency string) (*PSPCaptureResponse, error) {
	m.simulateLatency()

	if !m.shouldSucceed() {
		return nil, fmt.Errorf("mock psp: simulated capture failure for %s", pspPaymentID)
	}

	return &PSPCaptureResponse{
		PSPPaymentID: pspPaymentID,
		Status:       "captured",
	}, nil
}

func (m *MockPSP) RefundPayment(ctx context.Context, pspPaymentID string, amount int64) (*PSPRefundResponse, error) {
	m.simulateLatency()

	if !m.shouldSucceed() {
		return nil, fmt.Errorf("mock psp: simulated refund failure for %s", pspPaymentID)
	}

	return &PSPRefundResponse{
		PSPRefundID:  "mock_ref_" + uuid.NewString()[:8],
		PSPPaymentID: pspPaymentID,
		Amount:       amount,
		Status:       "refunded",
	}, nil
}

func (m *MockPSP) CancelPayment(ctx context.Context, pspPaymentID string) error {
	m.simulateLatency()

	if !m.shouldSucceed() {
		return fmt.Errorf("mock psp: simulated cancel failure for %s", pspPaymentID)
	}
	return nil
}

func (m *MockPSP) GetPaymentStatus(ctx context.Context, pspPaymentID string) (*PSPStatusResponse, error) {
	m.simulateLatency()

	return &PSPStatusResponse{
		PSPPaymentID: pspPaymentID,
		Status:       "captured",
		Amount:       0, // not tracked in mock
		Currency:     "INR",
	}, nil
}

func (m *MockPSP) VerifyWebhookSignature(payload []byte, signature string) error {
	mac := hmac.New(sha256.New, []byte(m.cfg.WebhookSecret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return fmt.Errorf("mock psp: invalid webhook signature")
	}
	return nil
}

func (m *MockPSP) simulateLatency() {
	spread := m.cfg.LatencyMax - m.cfg.LatencyMin
	if spread <= 0 {
		return
	}
	m.mu.Lock()
	delay := m.cfg.LatencyMin + time.Duration(m.rng.Int64N(int64(spread)))
	m.mu.Unlock()
	time.Sleep(delay)
}

func (m *MockPSP) shouldSucceed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rng.Float64() < m.cfg.SuccessRate
}
