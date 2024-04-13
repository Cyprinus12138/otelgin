// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Based on https://github.com/DataDog/dd-trace-go/blob/8fb554ff7cf694267f9077ae35e27ce4689ed8b6/contrib/gin-gonic/gin/gintrace.go

package otelgin // import "go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/gin-gonic/gin"

	"github.com/Cyprinus12138/otelgin/internal/semconvutil"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	tracerKey = "otel-go-contrib-tracer"
	meterKey  = "otel-go-contrib-meter"
	// ScopeName is the instrumentation scope name.
	ScopeName = "github.com/Cyprinus12138/otelgin"
	role      = "server"
	one       = 1
)

// Middleware returns middleware that will trace incoming requests.
// The service parameter should describe the name of the (virtual)
// server handling the request.
func Middleware(service string, opts ...Option) gin.HandlerFunc {
	var err error
	cfg := config{}
	for _, opt := range opts {
		opt.apply(&cfg)
	}
	if cfg.TracerProvider == nil {
		cfg.TracerProvider = otel.GetTracerProvider()
	}
	if cfg.MeterProvider == nil {
		cfg.MeterProvider = otel.GetMeterProvider()
	}
	tracer := cfg.TracerProvider.Tracer(
		ScopeName,
		oteltrace.WithInstrumentationVersion(Version()),
	)
	meter := cfg.MeterProvider.Meter(
		ScopeName,
		otelmetric.WithInstrumentationVersion(Version()),
	)
	if cfg.Propagators == nil {
		cfg.Propagators = otel.GetTextMapPropagator()
	}

	cfg.reqDuration, err = meter.Float64Histogram("http."+role+".request.duration",
		otelmetric.WithDescription("Measures the duration of inbound RPC."),
		otelmetric.WithUnit("ms"))
	if err != nil {
		otel.Handle(err)
		if cfg.reqDuration == nil {
			cfg.reqDuration = noop.Float64Histogram{}
		}
	}

	cfg.reqSize, err = meter.Int64UpDownCounter("http."+role+".request.body.size",
		otelmetric.WithDescription("Measures size of RPC request messages (uncompressed)."),
		otelmetric.WithUnit("By"))
	if err != nil {
		otel.Handle(err)
		if cfg.reqSize == nil {
			cfg.reqSize = noop.Int64UpDownCounter{}
		}
	}

	cfg.respSize, err = meter.Int64UpDownCounter("http."+role+".response.body.size",
		otelmetric.WithDescription("Measures size of RPC response messages (uncompressed)."),
		otelmetric.WithUnit("By"))
	if err != nil {
		otel.Handle(err)
		if cfg.respSize == nil {
			cfg.respSize = noop.Int64UpDownCounter{}
		}
	}

	cfg.activeReqs, err = meter.Int64UpDownCounter("http."+role+".active_requests",
		otelmetric.WithDescription("Measures the number of messages received per RPC. Should be 1 for all non-streaming RPCs."),
		otelmetric.WithUnit("{count}"))
	if err != nil {
		otel.Handle(err)
		if cfg.activeReqs == nil {
			cfg.activeReqs = noop.Int64UpDownCounter{}
		}
	}

	return func(c *gin.Context) {
		var (
			metricAttrs []attribute.KeyValue
			rAttr       attribute.KeyValue
		)

		for _, f := range cfg.Filters {
			if !f(c.Request) {
				// Serve the request to the next middleware
				// if a filter rejects the request.
				c.Next()
				return
			}
		}
		c.Set(tracerKey, tracer)
		c.Set(meterKey, meter)
		savedCtx := c.Request.Context()
		defer func() {
			c.Request = c.Request.WithContext(savedCtx)
		}()
		ctx := cfg.Propagators.Extract(savedCtx, propagation.HeaderCarrier(c.Request.Header))
		httpTraceAttrs := semconvutil.HTTPServerRequest(service, c.Request)
		opts := []oteltrace.SpanStartOption{
			oteltrace.WithAttributes(httpTraceAttrs...),
			oteltrace.WithSpanKind(oteltrace.SpanKindServer),
		}
		metricAttrs = semconvutil.HTTPServerRequestMetrics(service, c.Request)
		var spanName string
		if cfg.SpanNameFormatter == nil {
			spanName = c.FullPath()
		} else {
			spanName = cfg.SpanNameFormatter(c.Request)
		}
		if spanName == "" {
			spanName = fmt.Sprintf("HTTP %s route not found", c.Request.Method)
		} else {
			rAttr = semconv.HTTPRoute(spanName)
			opts = append(opts, oteltrace.WithAttributes(rAttr))
			metricAttrs = append(metricAttrs, rAttr)
		}
		ctx, span := tracer.Start(ctx, spanName, opts...)
		defer span.End()

		// pass the span through the request context
		c.Request = c.Request.WithContext(ctx)
		// calculate the size of the request.
		reqSize := calcReqSize(c)
		before := time.Now()

		// serve the request to the next middleware
		c.Next()
		// Use floating point division here for higher precision (instead of Millisecond method).
		elapsedTime := float64(time.Since(before)) / float64(time.Millisecond)
		respSize := c.Writer.Size()
		// If nothing written in the response yet, a value of -1 may be returned.
		if respSize < 0 {
			respSize = 0
		}

		status := c.Writer.Status()
		span.SetStatus(semconvutil.HTTPServerStatus(status))
		cfg.reqSize.Add(ctx, int64(reqSize), otelmetric.WithAttributes(metricAttrs...))
		cfg.respSize.Add(ctx, int64(respSize), otelmetric.WithAttributes(metricAttrs...))

		if status > 0 {
			statusAttr := semconv.HTTPStatusCode(status)
			span.SetAttributes(statusAttr)
			metricAttrs = append(metricAttrs, statusAttr)
		}
		if len(c.Errors) > 0 {
			errAttr := attribute.String("gin.errors", c.Errors.String())
			span.SetAttributes(errAttr)
			metricAttrs = append(metricAttrs, errAttr)
		}

		cfg.reqDuration.Record(ctx, elapsedTime, otelmetric.WithAttributes(metricAttrs...))
		cfg.activeReqs.Add(ctx, one, otelmetric.WithAttributes(metricAttrs...))
	}
}

// calcReqSize returns the total size of the request.
// It will calculate the header size by iterate all the header KVs
// and add with body size.
func calcReqSize(c *gin.Context) int {
	// Read the request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to read request body"})
		return 0
	}

	// Restore the request body for further processing
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	// Calculate the size of headers
	headerSize := 0
	for name, values := range c.Request.Header {
		headerSize += len(name) + 2 // Colon and space
		for _, value := range values {
			headerSize += len(value)
		}
	}

	// Calculate the total size of the request (headers + body)
	return headerSize + len(body)
}

// HTML will trace the rendering of the template as a child of the
// span in the given context. This is a replacement for
// gin.Context.HTML function - it invokes the original function after
// setting up the span.
func HTML(c *gin.Context, code int, name string, obj interface{}) {
	var tracer oteltrace.Tracer
	tracerInterface, ok := c.Get(tracerKey)
	if ok {
		tracer, ok = tracerInterface.(oteltrace.Tracer)
	}
	if !ok {
		tracer = otel.GetTracerProvider().Tracer(
			ScopeName,
			oteltrace.WithInstrumentationVersion(Version()),
		)
	}
	savedContext := c.Request.Context()
	defer func() {
		c.Request = c.Request.WithContext(savedContext)
	}()
	opt := oteltrace.WithAttributes(attribute.String("go.template", name))
	_, span := tracer.Start(savedContext, "gin.renderer.html", opt)
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("error rendering template:%s: %s", name, r)
			span.RecordError(err)
			span.SetStatus(codes.Error, "template failure")
			span.End()
			panic(r)
		}
		span.End()
	}()
	c.HTML(code, name, obj)
}
