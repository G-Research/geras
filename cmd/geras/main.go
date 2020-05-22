package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"github.com/thanos-io/thanos/pkg/store/storepb"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"

	"github.com/G-Research/geras/pkg/store"
	"github.com/G-Research/opentsdb-goclient/config"

	_ "net/http/pprof"

	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	opentsdb "github.com/G-Research/opentsdb-goclient/client"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
)

func NewConfiguredLogger(format string, logLevel string) (log.Logger, error) {
	var logger log.Logger
	switch format {
	case "logfmt":
		logger = log.NewLogfmtLogger(os.Stdout)
	case "json":
		logger = log.NewJSONLogger(os.Stdout)
	default:
		return nil, errors.Errorf("%s is not a valid log format", format)
	}

	var filterOption level.Option
	switch logLevel {
	case "debug":
		filterOption = level.AllowDebug()
	case "info":
		filterOption = level.AllowInfo()
	case "warn":
		filterOption = level.AllowWarn()
	case "error":
		filterOption = level.AllowError()
	default:
		return nil, errors.Errorf("%s is not a valid log level", logLevel)
	}
	logger = log.With(logger, "ts", log.DefaultTimestampUTC, "caller", log.Caller(5))
	logger = level.NewFilter(logger, filterOption)
	return logger, nil
}

type TracedTransport struct {
	originalTransport http.RoundTripper
	dumpHTTPBody      bool
}

func (t TracedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if tr, ok := trace.FromContext(req.Context()); ok {
		dumpReq, _ := httputil.DumpRequestOut(req, t.dumpHTTPBody)
		tr.LazyPrintf("TSDB Request: %v", string(dumpReq))
	}

	res, err := t.originalTransport.RoundTrip(req)

	if tr, ok := trace.FromContext(req.Context()); ok {
		if err != nil {
			tr.LazyPrintf("Error: %v", err)
		} else {
			dumpRes, _ := httputil.DumpResponse(res, t.dumpHTTPBody)
			tr.LazyPrintf("TSDB Response: %v", string(dumpRes))
		}
	}

	return res, err
}

type multipleStringFlags []string

func (i *multipleStringFlags) String() string {
	return fmt.Sprintf(strings.Join(*i, ", "))
}

func (i *multipleStringFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func main() {
	// define and parse command line flags
	grpcListenAddr := flag.String("grpc-listen", "localhost:19000", "Service will expose the Store API on this address")
	httpListenAddr := flag.String("http-listen", "localhost:19001", "Where to serve HTTP debugging endpoints (like /metrics)")
	traceEnabled := flag.Bool("trace-enabled", true, "Enable tracing of requests, which is shown at /debug/requests")
	traceDumpBody := flag.Bool("trace-dumpbody", false, "Include TSDB request and response bodies in traces (can be expensive)")
	logFormat := flag.String("log.format", "logfmt", "Log format. One of [logfmt, json]")
	logLevel := flag.String("log.level", "error", "Log filtering level. One of [debug, info, warn, error]")
	healthcheckMetric := flag.String("healthcheck-metric", "tsd.rpc.received", "A metric to query as a readiness health check.")
	openTSDBAddress := flag.String("opentsdb-address", "", "<host>:<port>")
	refreshInterval := flag.Duration("metrics-refresh-interval", time.Minute*15,
		"Time between metric name refreshes. Use negative duration to disable refreshes.")
	allowedMetricNamesRE := flag.String("metrics-allowed-regexp", ".*", "Regexp of metrics to allow")
	blockedMetricNamesRE := flag.String("metrics-blocked-regexp", "", "Regexp of metrics to block (empty disables blocking)")
	enableMetricSuggestions := flag.Bool("metrics-suggestions", true, "Enable metric suggestions (can be expensive)")
	enableMetricNameRewriting := flag.Bool("metrics-name-response-rewriting", false, "Rewrite '.' to ':' in all responses (Prometheus remote_read won't accept '.', while Thanos will)")
	var labels multipleStringFlags
	flag.Var(&labels, "label", "Label to expose on the Store API, of the form '<key>=<value>'. May be repeated.")
	flag.Parse()

	if *openTSDBAddress == "" {
		fmt.Fprintln(os.Stderr, version.Print("geras"))
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
		os.Exit(1)
	}
	allowedMetricNames, err := regexp.Compile(*allowedMetricNamesRE)
	if err != nil {
		fmt.Fprintf(os.Stderr, "metrics-allowed-regexp compile failed: %v", err)
		os.Exit(1)
	}
	var blockedMetricNames *regexp.Regexp
	if len(*blockedMetricNamesRE) > 0 {
		blockedMetricNames, err = regexp.Compile(*blockedMetricNamesRE)
		if err != nil {
			fmt.Fprintf(os.Stderr, "metrics-blocked-regexp compile failed: %v", err)
			os.Exit(1)
		}
	}

	// initialize logger
	logger, err := NewConfiguredLogger(*logFormat, *logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not initialize logger: %s", err)
		os.Exit(1)
	}
	// initialize tracing
	var transport http.RoundTripper = opentsdb.DefaultTransport
	if *traceEnabled {
		transport = TracedTransport{
			originalTransport: transport,
			dumpHTTPBody:      *traceDumpBody,
		}
		trace.AuthRequest = func(req *http.Request) (any, sensitive bool) {
			return true, true
		}
		grpc.EnableTracing = true
	}
	// initialize openTSDB api client
	client, err := opentsdb.NewClientContext(
		config.OpenTSDBConfig{
			OpentsdbHost: *openTSDBAddress,
			Transport:    transport,
		})
	if err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}
	var storeLabels []storepb.Label
	for _, label := range labels {
		splitLabel := strings.Split(label, "=")
		if len(splitLabel) != 2 {
			level.Error(logger).Log("message", fmt.Sprintf("Labels should be of the form '<key>=<value>', actually found '%s'", label))
			os.Exit(1)
		}
		storeLabel := storepb.Label{
			Name:  splitLabel[0],
			Value: splitLabel[1],
		}
		storeLabels = append(storeLabels, storeLabel)
	}

	http.Handle("/metrics", promhttp.Handler())
	prometheus.DefaultRegisterer.MustRegister(version.NewCollector("geras"))

	// create openTSDBStore and expose its api on a grpc server
	srv, err := store.NewOpenTSDBStore(logger, client, prometheus.DefaultRegisterer, *refreshInterval, storeLabels, allowedMetricNames, blockedMetricNames, *enableMetricSuggestions, *enableMetricNameRewriting, *healthcheckMetric)
	if err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}
	grpcSrv := grpc.NewServer(
		grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
		grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor),
	)

	storepb.RegisterStoreServer(grpcSrv, srv)
	healthpb.RegisterHealthServer(grpcSrv, srv)
	// After all your registrations, make sure all of the Prometheus metrics are initialized.
	grpc_prometheus.Register(grpcSrv)

	l, err := net.Listen("tcp", *grpcListenAddr)
	if err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}
	if len(*httpListenAddr) > 0 {
		go http.ListenAndServe(*httpListenAddr, nil)
		level.Debug(logger).Log("Debug listening on ", *httpListenAddr)
	}
	level.Debug(logger).Log("GRPC listening on ", *grpcListenAddr)
	grpcSrv.Serve(l)
}
