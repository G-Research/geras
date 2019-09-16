package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/tsdb/chunkenc"
	"github.com/thanos-io/thanos/pkg/store/storepb"
	"golang.org/x/net/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	opentsdb "github.com/G-Research/opentsdb-goclient/client"
)

type OpenTSDBStore struct {
	logger                                 log.Logger
	openTSDBClient                         opentsdb.ClientContext
	internalMetrics                        internalMetrics
	metricNames                            []string
	metricsNamesLock                       sync.RWMutex
	metricRefreshInterval                  time.Duration
	allowedMetricNames, blockedMetricNames *regexp.Regexp
	enableMetricSuggestions                bool
	storeLabels                            []storepb.Label
}

func NewOpenTSDBStore(logger log.Logger, client opentsdb.ClientContext, reg prometheus.Registerer, interval time.Duration, storeLabels []storepb.Label, allowedMetricNames, blockedMetricNames *regexp.Regexp, enableMetricSuggestions bool) *OpenTSDBStore {
	store := &OpenTSDBStore{
		logger:                  log.With(logger, "component", "opentsdb"),
		openTSDBClient:          client,
		internalMetrics:         newInternalMetrics(reg),
		metricRefreshInterval:   interval,
		enableMetricSuggestions: enableMetricSuggestions,
		storeLabels:             storeLabels,
		allowedMetricNames:      allowedMetricNames,
		blockedMetricNames:      blockedMetricNames,
	}
	store.updateMetrics(context.Background(), logger)
	return store
}

func (store *OpenTSDBStore) updateMetrics(ctx context.Context, logger log.Logger) {
	events := trace.NewEventLog("store.updateMetrics", "")

	fetch := func() {
		events.Printf("Refresh metrics")
		tr := trace.New("store.updateMetrics", "fetch")
		defer tr.Finish()
		err := store.loadAllMetricNames(trace.NewContext(ctx, tr))
		if err != nil {
			level.Info(store.logger).Log("err", err)
			events.Errorf("error: %v", err)
		} else {
			level.Debug(logger).Log("msg", "metric names have been refreshed")
			events.Printf("Refreshed")
		}
	}
	fetch()

	if store.metricRefreshInterval >= 0 {
		go func() {
			for {
				time.Sleep(store.metricRefreshInterval)
				fetch()
			}
		}()
	}
}

type internalMetrics struct {
	numberOfOpenTSDBMetrics *prometheus.GaugeVec
	openTSDBLatency         *prometheus.HistogramVec
	servedDatapoints        prometheus.Counter
}

func newInternalMetrics(reg prometheus.Registerer) internalMetrics {
	m := internalMetrics{
		openTSDBLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "geras_opentsdb_request_latency_seconds",
			Buckets: []float64{
				0.01, 0.05, 0.1, 0.5, 1, 5, 10, 20, 50,
			}},
			[]string{"endpoint", "status"}),
		servedDatapoints: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "geras_served_datapoints_total",
		}),
		numberOfOpenTSDBMetrics: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "geras_cached_metrics",
		},
			[]string{"type"}),
	}
	if reg != nil {
		reg.MustRegister(m.openTSDBLatency)
		reg.MustRegister(m.servedDatapoints)
		reg.MustRegister(m.numberOfOpenTSDBMetrics)
	}
	return m
}

func (store *OpenTSDBStore) Info(
	ctx context.Context,
	req *storepb.InfoRequest) (*storepb.InfoResponse, error) {
	res := storepb.InfoResponse{
		MinTime: 0,
		MaxTime: math.MaxInt64,
		Labels:  store.storeLabels,
	}
	var err error
	store.timedTSDBOp("version", func() error {
		_, err = store.openTSDBClient.WithContext(ctx).Version()
		return err
	})
	return &res, err
}

func (store *OpenTSDBStore) Series(
	req *storepb.SeriesRequest,
	server storepb.Store_SeriesServer) error {
	ctx := server.Context()
	query, warnings, err := store.composeOpenTSDBQuery(req)
	if err != nil {
		level.Error(store.logger).Log("err", err)
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Series query compose error: %v", err)
		}
		return err
	}
	if len(query.Queries) == 0 {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Series request resulted in no queries")
		}
		return nil
	}
	if len(warnings) > 0 {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Series query compose warnings: %v", warnings)
		}
		for _, warning := range warnings {
			server.Send(storepb.NewWarnSeriesResponse(warning))
		}
	}

	var result *opentsdb.QueryResponse
	store.timedTSDBOp("query", func() error {
		result, err = store.openTSDBClient.WithContext(ctx).Query(query)
		return err
	})
	if err != nil {
		level.Error(store.logger).Log("err", err)
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Series query error: %v", err)
		}
		return err
	}
	for _, respI := range result.QueryRespCnts {
		res, err := convertOpenTSDBResultsToSeriesResponse(respI)
		if err != nil {
			level.Error(store.logger).Log("err", err)
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Series result conversion error: %v", err)
			}
			return err
		}
		if err := server.Send(res); err != nil {
			level.Error(store.logger).Log("err", err)
			return err
		}
		store.internalMetrics.servedDatapoints.Add(float64(res.Result.Size()))
	}
	return nil
}

func (store *OpenTSDBStore) timedTSDBOp(endpoint string, f func() error) {
	start := time.Now()
	err := f()
	taken := float64(time.Since(start) / time.Second)
	typeString := "success"
	if err != nil {
		typeString = "error"
	}
	store.internalMetrics.openTSDBLatency.With(
		prometheus.Labels{
			"status":   typeString,
			"endpoint": endpoint,
		},
	).Observe(taken)
}

func (store *OpenTSDBStore) LabelNames(
	ctx context.Context,
	req *storepb.LabelNamesRequest) (*storepb.LabelNamesResponse, error) {
	labelNames, err := store.suggestAsList(ctx, "tagk")
	if err != nil {
		return nil, err
	}
	return &storepb.LabelNamesResponse{
		Names: labelNames,
	}, nil
}

func (store *OpenTSDBStore) suggestAsList(ctx context.Context, t string) ([]string, error) {
	var result *opentsdb.SuggestResponse
	var err error
	store.timedTSDBOp("suggest_"+t, func() error {
		result, err = store.openTSDBClient.WithContext(ctx).Suggest(
			opentsdb.SuggestParam{
				Type:         t,
				Q:            "",
				MaxResultNum: math.MaxInt32,
			})
		return err
	})
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Suggest %s error: %v", t, err)
		}
		return nil, err
	}
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Suggest %s results: %d items", t, len(result.ResultInfo))
	}
	return result.ResultInfo, nil
}

func (store *OpenTSDBStore) LabelValues(
	ctx context.Context,
	req *storepb.LabelValuesRequest) (*storepb.LabelValuesResponse, error) {
	level.Debug(store.logger).Log("msg", "LabelValues", "Label", req.Label)
	if store.enableMetricSuggestions && req.Label == "__name__" {
		var pNames []string
		store.metricsNamesLock.RLock()
		for _, item := range store.metricNames {
			pNames = append(pNames, strings.Replace(item, ".", ":", -1))
		}
		store.metricsNamesLock.RUnlock()
		return &storepb.LabelValuesResponse{
			Values: pNames,
		}, nil
	}
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (store *OpenTSDBStore) loadAllMetricNames(ctx context.Context) error {
	metricNames, err := store.suggestAsList(ctx, "metrics")
	if err != nil {
		return err
	}
	store.internalMetrics.numberOfOpenTSDBMetrics.With(prometheus.Labels{
		"type": "retrieved",
	}).Set(float64(len(metricNames)))

	metricNames, _, err = store.checkMetricNames(metricNames, false)
	if err != nil {
		return err
	}

	store.metricsNamesLock.Lock()
	store.metricNames = metricNames
	store.internalMetrics.numberOfOpenTSDBMetrics.With(prometheus.Labels{
		"type": "served",
	}).Set(float64(len(store.metricNames)))
	store.metricsNamesLock.Unlock()
	return nil
}

func (store *OpenTSDBStore) getMatchingMetricNames(matcher storepb.LabelMatcher) ([]string, error) {
	if matcher.Name != "__name__" {
		return nil, errors.New("getMatchingMetricNames must be called on __name__ matcher")
	}
	if matcher.Type == storepb.LabelMatcher_EQ {
		value := strings.Replace(matcher.Value, ":", ".", -1)
		return []string{value}, nil
	} else if matcher.Type == storepb.LabelMatcher_NEQ {
		// we can support this, but we should not.
		return nil, errors.New("NEQ is not supported for __name__ label")
	} else if matcher.Type == storepb.LabelMatcher_NRE {
		return nil, errors.New("NRE is not supported")
	} else if matcher.Type == storepb.LabelMatcher_RE {
		// TODO: Regexp matchers working on the actual name seems like the least
		// surprising behaviour. Actually document this.
		rx, err := regexp.Compile(matcher.Value)
		if err != nil {
			return nil, err
		}
		var matchingMetrics []string
		// TODO: parallelize this
		store.metricsNamesLock.RLock()
		for _, v := range store.metricNames {
			if rx.MatchString(v) {
				matchingMetrics = append(matchingMetrics, v)
			}
		}
		store.metricsNamesLock.RUnlock()
		return matchingMetrics, nil
	}
	return nil, errors.New("unknown matcher type")
}

func (store *OpenTSDBStore) composeOpenTSDBQuery(req *storepb.SeriesRequest) (opentsdb.QueryParam /*warnings*/, []error, error) {
	var tagFilters []opentsdb.Filter
	var metricNames []string
	var err error
	for _, matcher := range req.Matchers {
		if matcher.Name == "__name__" {
			metricNames, err = store.getMatchingMetricNames(matcher)
			if err != nil {
				level.Info(store.logger).Log("err", err)
				return opentsdb.QueryParam{}, nil, err
			}
			continue
		}
		f, err := convertPromQLMatcherToFilter(matcher)
		if err != nil {
			level.Info(store.logger).Log("err", err)
			return opentsdb.QueryParam{}, nil, err
		}
		tagFilters = append(tagFilters, f)
	}
	if len(metricNames) == 0 {
		// although promQL supports queries without metric names
		// we do not want to do it at the moment.
		err := errors.New("missing __name__")
		level.Info(store.logger).Log("err", err)
		return opentsdb.QueryParam{}, nil, err
	}
	var warnings []error
	metricNames, warnings, err = store.checkMetricNames(metricNames, true)
	if err != nil {
		level.Info(store.logger).Log("err", err)
		return opentsdb.QueryParam{}, nil, err
	}
	if len(metricNames) == 0 {
		return opentsdb.QueryParam{}, nil, nil
	}

	subQueries := make([]opentsdb.SubQuery, len(metricNames))
	for i, mn := range metricNames {
		subQueries[i] = opentsdb.SubQuery{
			Aggregator: "none",
			Metric:     mn,
			Fiters:     tagFilters,
		}
	}
	query := opentsdb.QueryParam{
		Start:        req.MinTime,
		End:          req.MaxTime,
		Queries:      subQueries,
		MsResolution: true,
	}
	level.Debug(store.logger).Log("tsdb-query", query.String())
	return query, warnings, nil
}

func (store *OpenTSDBStore) checkMetricNames(metricNames []string, fullBlock bool) (allowed []string, warnings []error, err error) {
	var maybeWarn []error
	for _, name := range metricNames {
		if store.blockedMetricNames != nil && store.blockedMetricNames.MatchString(name) {
			if fullBlock {
				return nil, nil, fmt.Errorf("Metric %q is blocked on Geras", name)
			}
			continue
		} else if !store.allowedMetricNames.MatchString(name) {
			maybeWarn = append(maybeWarn, fmt.Errorf("%q is not allowed via Geras", name))
			continue
		}
		allowed = append(allowed, name)
	}
	if len(maybeWarn) > 0 && len(maybeWarn) != len(metricNames) {
		// Oddness where things are partially allowed (if nothing is allowed then
		// this likely is a Prometheus only query and that's fine). This could be
		// for various reasons (e.g. {__name__=~"up|tsd\.something"}, or where a
		// regexp __name__ was used and some metrics in tsdb aren't allowed).  This
		// is after the __name__ expansion and could be quite long, truncate it.
		if len(maybeWarn) > 5 {
			warnings = maybeWarn[:5]
		} else {
			warnings = maybeWarn
		}
	}
	return allowed, warnings, nil
}

func convertOpenTSDBResultsToSeriesResponse(respI opentsdb.QueryRespItem) (*storepb.SeriesResponse, error) {
	samples := make([]prompb.Sample, 0)
	for _, dp := range respI.GetDataPoints() {
		samples = append(samples, prompb.Sample{Timestamp: dp.Timestamp, Value: dp.Value.(float64)})
	}
	seriesLabels := make([]storepb.Label, len(respI.Tags))
	i := 0
	for k, v := range respI.Tags {
		seriesLabels[i] = storepb.Label{Name: k, Value: v}
		i++
	}
	seriesLabels = append(seriesLabels, storepb.Label{Name: "__name__", Value: respI.Metric})
	enc, cb, err := encodeChunk(samples)
	if err != nil {
		return nil, err
	}
	var minTime, maxTime int64
	if len(samples) != 0 {
		minTime = samples[0].Timestamp
		maxTime = samples[len(samples)-1].Timestamp
	}
	res := storepb.NewSeriesResponse(&storepb.Series{
		Labels: seriesLabels,
		Chunks: []storepb.AggrChunk{{
			MinTime: minTime,
			MaxTime: maxTime,
			Raw:     &storepb.Chunk{Type: enc, Data: cb},
		}},
	})
	return res, nil
}

func convertPromQLMatcherToFilter(matcher storepb.LabelMatcher) (opentsdb.Filter, error) {
	f := opentsdb.Filter{
		GroupBy: true,
		Tagk:    matcher.Name,
	}
	switch matcher.Type {
	case storepb.LabelMatcher_EQ:
		f.Type = "literal_or"
		f.FilterExp = matcher.Value
	case storepb.LabelMatcher_NEQ:
		f.Type = "not_literal_or"
		f.FilterExp = matcher.Value
	case storepb.LabelMatcher_NRE:
		return opentsdb.Filter{}, errors.New("LabelMatcher_NRE is not supported")
	case storepb.LabelMatcher_RE:
		f.Type = "regexp"
		f.FilterExp = matcher.Value
	}
	return f, nil
}

func encodeChunk(ss []prompb.Sample) (storepb.Chunk_Encoding, []byte, error) {
	c := chunkenc.NewXORChunk()
	a, err := c.Appender()
	if err != nil {
		return 0, nil, err
	}
	for _, s := range ss {
		a.Append(int64(s.Timestamp), float64(s.Value))
	}
	return storepb.Chunk_XOR, c.Bytes(), nil
}
