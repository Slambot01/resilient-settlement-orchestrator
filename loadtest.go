package main

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"
	"time"
	"sync/atomic"
	"crypto/rand"
	"encoding/hex"
)

func randomUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func main() {
	totalRequests := 10000
	concurrency := 50

	var successCount int32
	var failCount int32

	fmt.Printf("Starting load test: %d requests, %d concurrency...\n", totalRequests, concurrency)
	
	start := time.Now()

	var wg sync.WaitGroup
	requestsPerWorker := totalRequests / concurrency

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client := &http.Client{Timeout: 5 * time.Second}

			for j := 0; j < requestsPerWorker; j++ {
				payload := []byte(`{"merchant_id":"merchant_123","order_id":"order_` + randomUUID() + `","amount":1500,"currency":"USD","payment_method":"card","return_url":"https://example.com"}`)
				
				req, _ := http.NewRequest("POST", "http://localhost:8080/v1/payments", bytes.NewBuffer(payload))
				req.Header.Set("Authorization", "Bearer dev-admin-key")
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", randomUUID())

				resp, err := client.Do(req)
				if err != nil {
					atomic.AddInt32(&failCount, 1)
					continue
				}
				
				if resp.StatusCode == 201 || resp.StatusCode == 200 {
					atomic.AddInt32(&successCount, 1)
				} else {
					atomic.AddInt32(&failCount, 1)
				}
				resp.Body.Close()
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)
	
	reqPerSec := float64(totalRequests) / duration.Seconds()

	fmt.Println("\n--- Benchmark Results ---")
	fmt.Printf("Total Requests: %d\n", totalRequests)
	fmt.Printf("Success: %d\n", successCount)
	fmt.Printf("Failed: %d\n", failCount)
	fmt.Printf("Duration: %.2f seconds\n", duration.Seconds())
	fmt.Printf("Throughput: %.2f req/sec\n", reqPerSec)
}
