module github.com/G-Research/geras

require (
	github.com/G-Research/opentsdb-goclient v0.0.0-20191028155047-1a0d357f6ca7
	github.com/go-kit/kit v0.9.0
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.1.0
	github.com/prometheus/common v0.6.0
	github.com/prometheus/prometheus v1.8.2-0.20190913102521-8ab628b35467 // v1.8.2 is misleading as Prometheus does not have v2 module.
	github.com/thanos-io/thanos v0.7.0
	golang.org/x/net v0.0.0-20190724013045-ca1201d0de80
	google.golang.org/grpc v1.22.1
)
