// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Based on https://github.com/DataDog/dd-trace-go/blob/8fb554ff7cf694267f9077ae35e27ce4689ed8b6/contrib/gin-gonic/gin/gintrace_test.go

package otelgin

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	b3prop "go.opentelemetry.io/contrib/propagators/b3"
)

func init() {
	gin.SetMode(gin.ReleaseMode) // silence annoying log msgs
}

func TestGetSpanNotInstrumented(t *testing.T) {
	router := gin.New()
	router.GET("/ping", func(c *gin.Context) {
		// Assert we don't have a span on the context.
		span := trace.SpanFromContext(c.Request.Context())
		ok := !span.SpanContext().IsValid()
		assert.True(t, ok)
		_, _ = c.Writer.Write([]byte("ok"))
	})
	r := httptest.NewRequest("GET", "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	response := w.Result()
	assert.Equal(t, http.StatusOK, response.StatusCode)
}

func TestPropagationWithGlobalPropagators(t *testing.T) {
	provider := noop.NewTracerProvider()
	otel.SetTextMapPropagator(b3prop.New())

	r := httptest.NewRequest("GET", "/user/123", nil)
	w := httptest.NewRecorder()

	ctx := context.Background()
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{0x01},
		SpanID:  trace.SpanID{0x01},
	})
	ctx = trace.ContextWithRemoteSpanContext(ctx, sc)
	ctx, _ = provider.Tracer(ScopeName).Start(ctx, "test")
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(r.Header))

	router := gin.New()
	router.Use(Middleware("foobar", WithTracerProvider(provider)))
	router.GET("/user/:id", func(c *gin.Context) {
		span := trace.SpanFromContext(c.Request.Context())
		assert.Equal(t, sc.TraceID(), span.SpanContext().TraceID())
		assert.Equal(t, sc.SpanID(), span.SpanContext().SpanID())
	})

	router.ServeHTTP(w, r)
}

func TestPropagationWithCustomPropagators(t *testing.T) {
	provider := noop.NewTracerProvider()
	b3 := b3prop.New()

	r := httptest.NewRequest("GET", "/user/123", nil)
	w := httptest.NewRecorder()

	ctx := context.Background()
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{0x01},
		SpanID:  trace.SpanID{0x01},
	})
	ctx = trace.ContextWithRemoteSpanContext(ctx, sc)
	ctx, _ = provider.Tracer(ScopeName).Start(ctx, "test")
	b3.Inject(ctx, propagation.HeaderCarrier(r.Header))

	router := gin.New()
	router.Use(Middleware("foobar", WithTracerProvider(provider), WithPropagators(b3)))
	router.GET("/user/:id", func(c *gin.Context) {
		span := trace.SpanFromContext(c.Request.Context())
		assert.Equal(t, sc.TraceID(), span.SpanContext().TraceID())
		assert.Equal(t, sc.SpanID(), span.SpanContext().SpanID())
	})

	router.ServeHTTP(w, r)
}

// TestCalcReqSize tests the calcReqSize function.
func TestCalcReqSize(t *testing.T) {
	// Create a sample request with a body and headers
	body := []byte("sample body")
	req, err := http.NewRequest("POST", "/test", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")

	// Create a Gin context with the request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Call the function to calculate the request size
	size := calcReqSize(c)

	// Calculate the expected size (body + headers + extra bytes for header formatting)
	expectedSize := len(body) + len("Content-Type") + len("application/json") + len("Authorization") + len("Bearer token") + 4 // 4 extra bytes for ": " and "\r\n"

	// Check if the calculated size matches the expected size
	if size != expectedSize {
		t.Errorf("Expected request size %d, got %d", expectedSize, size)
	}
}

// TestCalcReqSizeWithBodyRead tests the calcReqSize function and ensures the request body can still be read afterward.
func TestCalcReqSizeWithBodyRead(t *testing.T) {
	// Create a sample request with a body and headers
	body := []byte("sample body")
	req, err := http.NewRequest("POST", "/test", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")

	// Create a Gin context with the request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Call the function to calculate the request size
	size := calcReqSize(c)

	// Calculate the expected size (body + headers + extra bytes for header formatting)
	expectedSize := len(body) + len("Content-Type") + len("application/json") + len("Authorization") + len("Bearer token") + 4 // 4 extra bytes for ": " and "\r\n"

	// Check if the calculated size matches the expected size
	if size != expectedSize {
		t.Errorf("Expected request size %d, got %d", expectedSize, size)
	}

	// Read the request body again
	newBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	// Check if the body is unchanged
	if !bytes.Equal(newBody, body) {
		t.Errorf("Expected request body %q, got %q", body, newBody)
	}
}
