package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

var (
	port       = flag.Int("port", 8099, "port to listen on")
	billingURL = flag.String("billing-url", "http://localhost:8090/webhooks/nicepay", "billing webhook endpoint")
)

type b2bTokenResp struct {
	ResponseCode    string `json:"responseCode"`
	ResponseMessage string `json:"responseMessage"`
	AccessToken     string `json:"accessToken"`
	TokenType       string `json:"tokenType"`
	ExpiresIn       int    `json:"expiresIn"`
}

func main() {
	flag.Parse()

	// Generate RSA keypair for mock signing
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("failed to generate rsa key: %v", err)
	}
	pubASN1, _ := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	pubB64 := base64.StdEncoding.EncodeToString(pubASN1)

	log.Printf("Mock NICEPay Server started on :%d", *port)
	log.Printf("Mock Public Key (NICEPAY_PUBLIC_KEY):\n%s", pubB64)

	http.HandleFunc("/nicepay/v1.0/access-token/b2b", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(b2bTokenResp{
			ResponseCode:    "2002700",
			ResponseMessage: "Successful",
			AccessToken:     "mock-b2b-token-12345",
			TokenType:       "Bearer",
			ExpiresIn:       900,
		})
	})

	http.HandleFunc("/nicepay/api/v1.0/transfer-va/create-va", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"responseCode": "2002700",
			"responseMessage": "Successful",
			"virtualAccountData": {
				"virtualAccountNo": "88081234567890",
				"additionalInfo": {
					"tXidVA": "MOCK-VA-TX-999",
					"vacctValidDt": "20261012",
					"vacctValidTm": "235959"
				}
			}
		}`))
	})

	http.HandleFunc("/nicepay/api/v1.0/qr/qr-mpm-generate", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"responseCode": "2005100",
			"responseMessage": "Successful",
			"referenceNo": "MOCK-QR-REF-999",
			"qrContent": "00020101021226680016ID.CO.NICEPAY.WWW01189360099900000000015204541153033605401000005802ID"
		}`))
	})

	http.HandleFunc("/nicepay/api/v1.0/debit/payment-host-to-host", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"responseCode": "2005400",
			"responseMessage": "Successful",
			"referenceNo": "MOCK-EWAL-REF-999",
			"webRedirectUrl": "https://dev.nicepay.co.id/mock-pay/ewallet?ref=999"
		}`))
	})

	statusHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"responseCode": "2000000",
			"responseMessage": "Successful",
			"latestTransactionStatus": "00",
			"virtualAccountData": {
				"additionalInfo": {
					"latestTransactionStatus": "00"
				}
			}
		}`))
	}
	http.HandleFunc("/nicepay/api/v1.0/transfer-va/status", statusHandler)
	http.HandleFunc("/nicepay/api/v1.0/qr/qr-mpm-query", statusHandler)
	http.HandleFunc("/nicepay/api/v1.0/debit/status", statusHandler)

	http.HandleFunc("/simulate-webhook", func(w http.ResponseWriter, r *http.Request) {
		partnerRef := r.URL.Query().Get("ref")
		if partnerRef == "" {
			partnerRef = "MOCK-REF-1"
		}
		clientID := r.URL.Query().Get("client_id")
		if clientID == "" {
			clientID = "IONPAYTEST"
		}

		payload := map[string]any{
			"originalReferenceNo":        partnerRef,
			"originalPartnerReferenceNo": partnerRef,
			"latestTransactionStatus":    "00",
			"amount": map[string]string{
				"value":    "100000.00",
				"currency": "IDR",
			},
			"additionalInfo": map[string]string{
				"tXidVA": partnerRef,
			},
		}
		raw, _ := json.Marshal(payload)
		ts := time.Now().Format("2006-01-02T15:04:05-07:00")
		digest := sha256.Sum256([]byte(clientID + "|" + ts))
		sig, _ := rsa.SignPKCS1v15(rand.Reader, privKey, crypto.SHA256, digest[:])
		sigB64 := base64.StdEncoding.EncodeToString(sig)

		req, _ := http.NewRequest(http.MethodPost, *billingURL, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-TIMESTAMP", ts)
		req.Header.Set("X-SIGNATURE", sigB64)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to send webhook: %v", err), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		fmt.Fprintf(w, "Webhook dispatched with status %d\n", resp.StatusCode)
	})

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}
