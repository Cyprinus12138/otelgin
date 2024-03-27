# Go OpenTelemetry Gonic-gin Tracer & Metrics Instrumentation

[![ci](https://github.com/Cyprinus12138/otelgin/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/Cyprinus12138/otelgin/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Cyprinus12138/otelgin)](https://goreportcard.com/report/github.com/Cyprinus12138/otelgin)
[![Documentation](https://godoc.org/github.com/Cyprinus12138/otelgin?status.svg)](https://pkg.go.dev/mod/github.com/Cyprinus12138/otelgin)

It is an OpenTelemetry (OTel) metric instrumentation for Golang gRPC servers and clients based on [gRPC Stats](https://pkg.go.dev/google.golang.org/grpc/stats).

## Install

```bash
$ go get github.com/Cyprinus12138/otelgin
```

## Usage

Metrics are reported based on [Semantic Conventions for HTTP Metrics](https://opentelemetry.io/docs/specs/semconv/http/http-metrics/#http-server) :

1. `http.server.request.duration`
2. `http.server.request.body.size`
3. `http.server.response.body.size`
4. `http.server.active_requests`

### Plugin as a middleware

[Example Server](https://github.com/Cyprinus12138/otelgin/blob/main/example/server.go)

```go
	r := gin.New()
    r.Use(otelgin.Middleware("my-server"))
```
