package butter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecSwapSendsBearerToken(t *testing.T) {
	Init("test-key")
	defer Init("")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != UrlOfExecSwap {
			t.Fatalf("path = %s, want %s", r.URL.Path, UrlOfExecSwap)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q, want Bearer test-key", got)
		}
		_, _ = w.Write([]byte(`{"errno":0}`))
	}))
	defer srv.Close()

	if _, err := ExecSwap(srv.URL, "a=b"); err != nil {
		t.Fatalf("ExecSwap failed: %v", err)
	}
}

func TestSolCrossInSendsBearerToken(t *testing.T) {
	Init("test-key")
	defer Init("")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != UrlOfSolCrossIn {
			t.Fatalf("path = %s, want %s", r.URL.Path, UrlOfSolCrossIn)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q, want Bearer test-key", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["orderId"] != "0xabc" {
			t.Fatalf("orderId = %v, want 0xabc", body["orderId"])
		}
		_, _ = w.Write([]byte(`{"errno":0,"statusCode":0,"data":{"routeWithTxParams":[{"txParam":[{"data":"0x"}]}]}}`))
	}))
	defer srv.Close()

	if _, err := SolCrossIn(srv.URL, "a=b", map[string]interface{}{"orderId": "0xabc"}); err != nil {
		t.Fatalf("SolCrossIn failed: %v", err)
	}
}

func TestSolCrossInReportsRefundOnlyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errno":0,"data":{"routeWithTxParams":[],
			"rescueFundsTxParams":[{"step":"receiveOpenRefund","isRefund":true,"data":"00"}]}}`))
	}))
	defer srv.Close()

	_, err := SolCrossIn(srv.URL, "a=b", map[string]interface{}{})
	if err == nil {
		t.Fatal("SolCrossIn error = nil, want refund-only error")
	}
	if !strings.Contains(err.Error(), "only refund") {
		t.Fatalf("SolCrossIn error = %q, want it to mention only refund", err.Error())
	}
}

func TestSolCrossInReportsRouteBuildError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errno":0,"data":{"routeWithTxParams":[
			{"route":{},"error":{"message":"quote expired"}}]}}`))
	}))
	defer srv.Close()

	_, err := SolCrossIn(srv.URL, "a=b", map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "quote expired") {
		t.Fatalf("SolCrossIn error = %v, want route build error", err)
	}
}
