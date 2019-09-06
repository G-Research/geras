package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
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
	logFormat := flag.String("log.format", "logfmt", "Log format. One of [logfmt, json]")
	logLevel := flag.String("log.level", "error", "Log filtering level. One of [debug, info, warn, error]")
	openTSDBAddress := flag.String("opentsdb-address", "", "http[s]://<host>:<port>")
	refreshInterval := flag.Duration("metrics-refresh-interval", time.Minute*15,
		"Time between metric name refreshes. Use negative duration to disable refreshes.")
	allowedMetricNamesRE := flag.String("metrics-allowed-regexp", ".*", "Regexp of metrics to allow")
	blockedMetricNamesRE := flag.String("metrics-blocked-regexp", "", "Regexp of metrics to block (empty disables blocking)")
	enableMetricSuggestions := flag.Bool("metrics-suggestions", true, "Enable metric suggestions (can be expensive)")
	var labels multipleStringFlags
	flag.Var(&labels, "label", "Label to expose on the Store API, of the form '<key>=<value>'. May be repeated.")
	flag.Parse()

	if *openTSDBAddress == "" {
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
	// initialize openTSDB api client
	client, err := opentsdb.NewClientContext(
		config.OpenTSDBConfig{
			OpentsdbHost: *openTSDBAddress,
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

	// create openTSDBStore and expose its api on a grpc server
	srv := store.NewOpenTSDBStore(logger, client, *refreshInterval, storeLabels, allowedMetricNames, blockedMetricNames, *enableMetricSuggestions)
	grpcSrv := grpc.NewServer()
	storepb.RegisterStoreServer(grpcSrv, srv)
	l, err := net.Listen("tcp", *grpcListenAddr)
	if err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}
	grpcSrv.Serve(l)
}
