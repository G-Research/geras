package tracing

import (
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/otel/propagation"
	"golang.org/x/exp/slices"
)

var newPropagators = map[string]func() propagation.TextMapPropagator{
	"baggage":      func() propagation.TextMapPropagator { return propagation.Baggage{} },
	"tracecontext": func() propagation.TextMapPropagator { return propagation.TraceContext{} },
}

func NewPropagatorsFromEnv() (propagation.TextMapPropagator, error) {
	enabled := []string{"tracecontext", "baggage"}

	if v := strings.TrimSpace(os.Getenv("OTEL_PROPAGATORS")); v != "" {
		enabled = strings.Split(v, ",")
	}

	if slices.Contains(enabled, "none") {
		// OpenTelemetry defines an empty environment value as meaning the same as unset:
		// https://opentelemetry.io/docs/reference/specification/sdk-environment-variables/#parsing-empty-value
		// To allow users to turn off the propagators, recognize the special name "none"
		// to turn off propagators, the same way as exporter selection
		// (<https://opentelemetry.io/docs/reference/specification/sdk-environment-variables/#exporter-selection>).
		enabled = nil
	}

	var propagators []propagation.TextMapPropagator

	for _, name := range enabled {
		if new, ok := newPropagators[name]; ok {
			propagators = append(propagators, new())
		} else {
			return nil, fmt.Errorf("unknown propagator: \"%s\"", name)
		}
	}

	if len(propagators) > 1 {
		return propagation.NewCompositeTextMapPropagator(propagators...), nil
	}
	if len(propagators) == 1 {
		return propagators[0], nil
	}
	return nil, nil
}
