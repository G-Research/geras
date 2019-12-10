module github.com/G-Research/geras

require (
	github.com/G-Research/opentsdb-goclient v0.0.0-20191210204552-9a5d3f5d556d
	github.com/go-kit/kit v0.9.0
	github.com/grpc-ecosystem/go-grpc-prometheus v0.0.0-20181025070259-68e3a13e4117
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.1.0
	github.com/prometheus/common v0.6.0
	github.com/prometheus/prometheus v1.8.2-0.20190913102521-8ab628b35467 // v1.8.2 is misleading as Prometheus does not have v2 module.
	github.com/thanos-io/thanos v0.7.0
	golang.org/x/net v0.0.0-20190724013045-ca1201d0de80
	golang.org/x/tools v0.0.0-20191206204035-259af5ff87bd // indirect
	google.golang.org/grpc v1.22.1
)
