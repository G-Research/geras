module github.com/G-Research/geras

go 1.15

require (
	github.com/G-Research/opentsdb-goclient v0.0.0-20191219203319-f9f2aa5b2624
	github.com/go-kit/kit v0.9.0
	github.com/grpc-ecosystem/go-grpc-prometheus v0.0.0-20181025070259-68e3a13e4117
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.1.0
	github.com/prometheus/common v0.6.0
	github.com/prometheus/prometheus v1.8.2-0.20190913102521-8ab628b35467 // v1.8.2 is misleading as Prometheus does not have v2 module.
	github.com/thanos-io/thanos v0.7.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.16.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.16.0 // indirect
	go.opentelemetry.io/contrib/propagators v0.16.0 // indirect
	go.opentelemetry.io/otel v0.16.0
	go.opentelemetry.io/otel/exporters/trace/jaeger v0.16.0 // indirect
	golang.org/x/net v0.0.0-20201031054903-ff519b6c9102
	google.golang.org/grpc v1.34.0
)
