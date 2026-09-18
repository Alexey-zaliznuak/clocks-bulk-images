package imanator

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateOrder404IsTemplateNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"statusCode":404,"path":"/api/image-generation-orders","message":"Not found"}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL, "key").CreateOrder(context.Background(), "missing-tpl", nil)
	var missing *TemplateNotFoundError
	if !errors.As(err, &missing) || missing.TemplateID != "missing-tpl" {
		t.Fatalf("got %v, want TemplateNotFoundError", err)
	}
}
