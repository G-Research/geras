package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/G-Research/geras/pkg/store"
	opentsdb "github.com/G-Research/opentsdb-goclient/client"
	"github.com/G-Research/opentsdb-goclient/config"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/improbable-eng/thanos/pkg/store/storepb"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
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

func main() {
	// define and parse command line flags
	grpcListenAddr := flag.String("grpc-listen", "localhost:19000", "service will expose the store api on this address")
	logFormat := flag.String("log.format", "logfmt", "Log format. One of [logfmt, json]")
	logLevel := flag.String("log.level", "error", "Log filtering level. One of [debug, info, warn, error]")
	openTSDBAddress := flag.String("opentsdb-address", "", "host:port")
	refreshInterval := flag.Duration("metrics-refresh-interval", time.Minute*15,
		"Time between metric name refreshes. Use negative duration to disable refreshes.")
	flag.Parse()
	if *openTSDBAddress == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}
	// initialize logger
	logger, err := NewConfiguredLogger(*logFormat, *logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not initialize logger: %s", err)
		os.Exit(1)
	}
	// initialize openTSDB api client
	client, err := opentsdb.NewClientContext(
		config.OpenTSDBConfig{
			OpentsdbHost: *openTSDBAddress,
		})
	if err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}
	// create openTSDBStore and expose its api on a grpc server
	srv := store.NewOpenTSDBStore(logger, client, *refreshInterval)
	grpcSrv := grpc.NewServer()
	storepb.RegisterStoreServer(grpcSrv, srv)
	l, err := net.Listen("tcp", *grpcListenAddr)
	if err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}
	grpcSrv.Serve(l)
}
