package tracing

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/exp/slices"
)

var newExporters = map[string]func(context.Context) (sdktrace.SpanExporter, error){
	"stdout": func(_ context.Context) (sdktrace.SpanExporter, error) { return stdouttrace.New() },
	"otlp":   NewOTLPExporterFromEnv,
	// The Jaeger exporter is DEPRECATED. It is provided only to help
	// migrate existing installations from Jaeger protocol to OTLP.
	// https://opentelemetry.io/docs/reference/specification/sdk-environment-variables/#jaeger-exporter
	"jaeger": NewJaegerExporterFromEnv,
}

func NewOTLPExporterFromEnv(ctx context.Context) (sdktrace.SpanExporter, error) {
	protocol := "http/protobuf"
	if p := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL")); p != "" {
		protocol = p
	} else if p = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")); p != "" {
		protocol = p
	}

	var otlpExporter *otlptrace.Exporter
	var err error
	switch protocol {
	case "grpc":
		otlpExporter, err = otlptracegrpc.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP gRPC trace exporter: %w", err)
		}
	case "http/protobuf":
		otlpExporter, err = otlptracehttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP HTTP trace exporter: %w", err)
		}
	default:
		return nil, fmt.Errorf("unimplemented OTLP protocol %s", protocol)
	}
	return otlpExporter, nil
}

func NewJaegerExporterFromEnv(ctx context.Context) (sdktrace.SpanExporter, error) {
	if v := os.Getenv("OTEL_EXPORTER_JAEGER_ENDPOINT"); v != "" {
		return jaeger.New(jaeger.WithCollectorEndpoint())
	}

	return jaeger.New(jaeger.WithAgentEndpoint())
}

func NewProcessorsFromEnv(ctx context.Context) ([]sdktrace.SpanProcessor, error) {
	var enabled []string
	if s := strings.TrimSpace(os.Getenv("OTEL_TRACES_EXPORTER")); s != "" {
		enabled = strings.Split(s, ",")
	}

	// https://opentelemetry.io/docs/reference/specification/sdk-environment-variables/#exporter-selection
	// Default exporter should be "otlp"; however to preserve compatibiltiy
	// we will default to "none".
	if slices.Contains(enabled, "none") {
		// Short-circuit: If "none" is present, ignore everything else.
		enabled = nil
	}

	var spanProcessors []sdktrace.SpanProcessor

	for _, name := range enabled {
		if new, ok := newExporters[name]; ok {
			if exporter, err := new(ctx); err == nil {
				spanProcessors = append(spanProcessors, sdktrace.NewBatchSpanProcessor(exporter))
			} else {
				return nil, fmt.Errorf("failed to create exporter \"%s\": %w", name, err)
			}
		} else {
			return nil, fmt.Errorf("unknown trace exporter: \"%s\"", name)
		}
	}

	return spanProcessors, nil
}
