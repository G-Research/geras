package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/thanos-io/thanos/pkg/store/storepb"
	"golang.org/x/net/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	opentsdb "github.com/G-Research/opentsdb-goclient/client"

	"github.com/G-Research/geras/pkg/regexputil"
)

type OpenTSDBStore struct {
	logger                                 log.Logger
	openTSDBClient                         opentsdb.ClientContext
	internalMetrics                        internalMetrics
	metricNames                            []string
	metricsNamesLock                       sync.RWMutex
	metricRefreshInterval                  time.Duration
	metricRefreshTimeout                   time.Duration
	allowedMetricNames, blockedMetricNames *regexp.Regexp
	enableMetricSuggestions                bool
	enableMetricNameRewriting              bool
	storeLabels                            []storepb.Label
	storeLabelsMap                         map[string]string
	healthcheckMetric                      string
}

var (
	aggregateToDownsample                  = map[storepb.Aggr]string{
		storepb.Aggr_COUNT:   "count",
		storepb.Aggr_SUM:     "sum",
		storepb.Aggr_MIN:     "min",
		storepb.Aggr_MAX:     "max",
		storepb.Aggr_COUNTER: "avg",
	}
	downsampleToAggregate                  map[string]storepb.Aggr
)

func init() {
	downsampleToAggregate = map[string]storepb.Aggr{}
	for a, d := range aggregateToDownsample {
		if _, exists := downsampleToAggregate[d]; exists {
			panic(fmt.Sprintf("Invalid aggregate/downsample mapping - not reversible for downsample function %s", d))
		}
		downsampleToAggregate[d] = a
	}
}

func NewOpenTSDBStore(logger log.Logger, client opentsdb.ClientContext, reg prometheus.Registerer, refreshInterval, refreshTimeout time.Duration, storeLabels []storepb.Label, allowedMetricNames, blockedMetricNames *regexp.Regexp, enableMetricSuggestions, enableMetricNameRewriting bool, healthcheckMetric string) *OpenTSDBStore {
	// Extract the store labels into a map for faster access later
	storeLabelsMap := map[string]string{}
	for _, l := range storeLabels {
		storeLabelsMap[l.Name] = l.Value
	}
	store := &OpenTSDBStore{
		logger:                    log.With(logger, "component", "opentsdb"),
		openTSDBClient:            client,
		internalMetrics:           newInternalMetrics(reg),
		metricRefreshInterval:     refreshInterval,
		metricRefreshTimeout:      refreshTimeout,
		enableMetricSuggestions:   enableMetricSuggestions,
		enableMetricNameRewriting: enableMetricNameRewriting,
		storeLabels:               storeLabels,
		storeLabelsMap:            storeLabelsMap,
		allowedMetricNames:        allowedMetricNames,
		blockedMetricNames:        blockedMetricNames,
		healthcheckMetric:         healthcheckMetric,
	}
	if client != nil {
		store.updateMetrics(context.Background(), logger)
	}
	return store
}

type internalMetrics struct {
	numberOfOpenTSDBMetrics     *prometheus.GaugeVec
	lastUpdateOfOpenTSDBMetrics prometheus.Gauge
	openTSDBLatency             *prometheus.HistogramVec
	servedDatapoints            prometheus.Counter
	servedSeries                prometheus.Counter
}

func newInternalMetrics(reg prometheus.Registerer) internalMetrics {
	m := internalMetrics{
		openTSDBLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{Name: "geras_opentsdb_request_latency_seconds",
				Buckets: []float64{
					0.01, 0.05, 0.1, 0.5, 1, 5, 10, 20, 50,
				}},
			[]string{"endpoint", "status"}),
		servedDatapoints: prometheus.NewCounter(prometheus.CounterOpts{Name: "geras_served_datapoints_total"}),
		servedSeries:     prometheus.NewCounter(prometheus.CounterOpts{Name: "geras_served_series_total"}),
		numberOfOpenTSDBMetrics: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "geras_cached_metrics"},
			[]string{"type"}),
		lastUpdateOfOpenTSDBMetrics: prometheus.NewGauge(prometheus.GaugeOpts{Name: "geras_metrics_cache_update_time"}),
	}
	if reg != nil {
		reg.MustRegister(m.openTSDBLatency)
		reg.MustRegister(m.servedDatapoints)
		reg.MustRegister(m.servedSeries)
		reg.MustRegister(m.numberOfOpenTSDBMetrics)
		reg.MustRegister(m.lastUpdateOfOpenTSDBMetrics)
	}
	return m
}

func (store *OpenTSDBStore) updateMetrics(ctx context.Context, logger log.Logger) {
	events := trace.NewEventLog("store.updateMetrics", "")

	fetch := func() {
		events.Printf("Refresh metrics")
		tr := trace.New("store.updateMetrics", "fetch")
		defer tr.Finish()
		ctx, cancel := context.WithTimeout(ctx, store.metricRefreshTimeout)
		defer cancel()
		err := store.loadAllMetricNames(trace.NewContext(ctx, tr))
		if err != nil {
			level.Info(store.logger).Log("err", err)
			events.Errorf("error: %v", err)
		} else {
			store.internalMetrics.lastUpdateOfOpenTSDBMetrics.Set(float64(time.Now().Unix()))
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

func (store *OpenTSDBStore) Info(
	ctx context.Context,
	req *storepb.InfoRequest) (*storepb.InfoResponse, error) {
	res := storepb.InfoResponse{
		MinTime: 0,
		MaxTime: math.MaxInt64,
		Labels:  store.storeLabels,
		LabelSets: []storepb.LabelSet{{Labels:store.storeLabels}},
	}
	err := store.timedTSDBOp("query", func() error {
		now := time.Now().Unix()
		q := opentsdb.QueryParam{
			Start:        now,
			End:          now + 1,
			MsResolution: true,
			Queries: []opentsdb.SubQuery{{
				Metric:     store.healthcheckMetric,
				Aggregator: "sum",
			}},
		}
		results, err := store.openTSDBClient.WithContext(ctx).Query(q)
		if err != nil {
			return err
		}
		if results.ErrorMsg != nil {
			return results.ErrorMsg
		}
		return nil
	})
	return &res, err
}

func (store OpenTSDBStore) Check(ctx context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (store OpenTSDBStore) Watch(req *healthpb.HealthCheckRequest, srv healthpb.Health_WatchServer) error {
	return status.Errorf(codes.Unimplemented, "method Watch not implemented")
}

func (store *OpenTSDBStore) Series(
	req *storepb.SeriesRequest,
	server storepb.Store_SeriesServer) error {
	ctx := server.Context()
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("PromQL: %v", dumpPromQL(req))
	}
	query, warnings, err := store.composeOpenTSDBQuery(req)
	if err != nil {
		level.Error(store.logger).Log("err", err)
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

	err = store.timedTSDBOp("query", func() error {
		outCh := make(chan *opentsdb.QueryRespItem, 5)
		err := store.openTSDBClient.WithContext(ctx).QueryStream(query, outCh)
		if err != nil {
			qerr, ok := err.(opentsdb.QueryError)
			if ok {
				if code, ok := qerr["code"].(float64); ok && code == 400 {
					msg, ok := qerr["message"].(string)
					if !ok || !strings.Contains(msg, "No such name for ") {
						level.Info(store.logger).Log("msg", "Ignoring 400 error", "err", err)
					}
					// Ignore all 400 errors, regardless of the reason (but the logs
					// should say if it's not a non-existent metric).
					return nil
				}
			}
			return err
		}
		overallCount := 0
		seriesCount := 0
		for respI := range outCh {
			if respI.Error != nil {
				return respI.Error
			}
			res, count, err := store.convertOpenTSDBResultsToSeriesResponse(respI)
			if err != nil {
				return err
			}
			if err := server.Send(res); err != nil {
				return err
			}
			store.internalMetrics.servedDatapoints.Add(float64(count))
			overallCount += count
			store.internalMetrics.servedSeries.Add(1)
			seriesCount++
		}
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("sent: datapoints:%d series:%d", overallCount, seriesCount)
		}
		return nil
	})
	if err != nil {
		level.Error(store.logger).Log("err", err)
		return err
	}
	return nil
}

func (store *OpenTSDBStore) timedTSDBOp(endpoint string, f func() error) error {
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
	return err
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
	err := store.timedTSDBOp("suggest_"+t, func() error {
		var err error
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
	if req.Label == "__name__" {
		if !store.enableMetricSuggestions {
			// An error for this breaks Thanos query UI; return an empty list instead.
			return &storepb.LabelValuesResponse{}, nil
		}
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
	sort.Strings(metricNames)

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
		return nil, errors.New("NEQ (!=) is not supported for __name__")
	} else if matcher.Type == storepb.LabelMatcher_NRE {
		return nil, errors.New("NRE (!~) is not supported for __name__")
	} else if matcher.Type == storepb.LabelMatcher_RE {
		rx, err := regexp.Compile("^(?:" + matcher.Value + ")$")
		if err != nil {
			return nil, err
		}
		var matchingMetrics []string
		store.metricsNamesLock.RLock()
		literalPrefix, complete := rx.LiteralPrefix()
		if literalPrefix != "" {
			firstPossibleIndex := sort.SearchStrings(store.metricNames, literalPrefix)
			if complete {
				if firstPossibleIndex < len(store.metricNames) && store.metricNames[firstPossibleIndex] == literalPrefix {
					matchingMetrics = append(matchingMetrics, literalPrefix)
				}
			} else {
				for i := firstPossibleIndex; i < len(store.metricNames); i++ {
					v := store.metricNames[i]
					if rx.MatchString(v) {
						matchingMetrics = append(matchingMetrics, v)
					} else {
						if !strings.HasPrefix(v, literalPrefix) {
							break
						}
					}
				}
			}
		} else {
			for _, v := range store.metricNames {
				if rx.MatchString(v) {
					matchingMetrics = append(matchingMetrics, v)
				}
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
	// Find __name__ matcher first so that checkMetricNames can check if the
	// metric is whitelisted on Geras. This avoids sending errors about other
	// labels if __name__ is later skipped.
	for _, matcher := range req.Matchers {
		if matcher.Name == "__name__" {
			metricNames, err = store.getMatchingMetricNames(matcher)
			if err != nil {
				level.Info(store.logger).Log("err", err)
				return opentsdb.QueryParam{}, nil, err
			}
			break
		}
	}
	var warnings []error
	metricNames, warnings, err = store.checkMetricNames(metricNames, true)
	if err != nil {
		level.Info(store.logger).Log("err", err)
		return opentsdb.QueryParam{}, nil, err
	}
	if len(metricNames) == 0 {
		// although promQL supports queries without metric names we do not want to
		// do it at the moment, but don't send an error because it's fine to do
		// queries that join metrics on Thanos and Geras. e.g.:
		// {__name__="some.opentsdb.metric",label="x"} or absent({label="x"}) * 0
		return opentsdb.QueryParam{}, nil, nil
	}

	for _, matcher := range req.Matchers {
		if matcher.Name == "__name__" {
			continue
		}
		f, err := convertPromQLMatcherToFilter(matcher)
		if err != nil {
			level.Info(store.logger).Log("err", err)
			return opentsdb.QueryParam{}, nil, err
		}
		tagFilters = append(tagFilters, f)
	}

	aggregationCount := 0
	needRawAggregation := true
	var downsampleSecs int64
	if req.MaxResolutionWindow != 0 {
		needRawAggregation = false
		for _, agg := range req.Aggregates {
			switch agg {
			case storepb.Aggr_RAW:
				needRawAggregation = true
				break
			case storepb.Aggr_COUNT:
				fallthrough
			case storepb.Aggr_SUM:
				fallthrough
			case storepb.Aggr_MIN:
				fallthrough
			case storepb.Aggr_MAX:
				fallthrough
			case storepb.Aggr_COUNTER:
				aggregationCount++
				break
			default:
				level.Info(store.logger).Log("err", fmt.Sprintf("Unrecognised series aggregator: %v", agg))
				needRawAggregation = true
				break
			}
		}
		downsampleSecs = req.MaxResolutionWindow / 1000
	}
	if needRawAggregation {
		aggregationCount++
	}
	subQueries := make([]opentsdb.SubQuery, len(metricNames)*aggregationCount)
	for i, mn := range metricNames {
		aggregationIndex := 0
		if req.MaxResolutionWindow != 0 {
			for _, agg := range req.Aggregates {
				addAgg := true
				var downsample string
				if ds, exists := aggregateToDownsample[agg]; exists {
					downsample = ds
				} else {
					addAgg = false
				}
				if addAgg {
					subQueries[(i*aggregationCount)+aggregationIndex] = opentsdb.SubQuery{
						Aggregator: "none",
						Downsample: fmt.Sprintf("%vs-%s", downsampleSecs, downsample),
						Metric:     mn,
						Filters:    tagFilters,
					}
					aggregationIndex++
				}
			}
		}
		if needRawAggregation {
			subQueries[(i*aggregationCount)+aggregationIndex] = opentsdb.SubQuery{
				Aggregator: "none",
				Metric:     mn,
				Filters:    tagFilters,
			}
		}
	}
	query := opentsdb.QueryParam{
		Start:        req.MinTime,
		End:          req.MaxTime,
		Queries:      subQueries,
		MsResolution: true,
		ShowQuery:    true,
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

func (store *OpenTSDBStore) convertOpenTSDBResultsToSeriesResponse(respI *opentsdb.QueryRespItem) (*storepb.SeriesResponse, int, error) {
	name := respI.Metric
	if store.enableMetricNameRewriting {
		name = strings.ReplaceAll(name, ".", ":")
	}
	seriesLabels := make([]storepb.Label, 1+len(respI.Tags)+len(store.storeLabels))
	i := 0
	keys := make([]string, len(seriesLabels))
	keys[i] = "__name__"
	i++
	for k := range respI.Tags {
		keys[i] = k
		i++
	}
	for k := range store.storeLabelsMap {
		keys[i] = k
		i++
	}
	sort.Strings(keys)

	i = 0
	for _, k := range keys {
		if k == "__name__" {
			seriesLabels[i] = storepb.Label{Name: k, Value: name}
		} else if v, ok := store.storeLabelsMap[k]; ok {
		  seriesLabels[i] = storepb.Label{Name: k, Value: v}
		} else {
			seriesLabels[i] = storepb.Label{Name: k, Value: respI.Tags[k]}
		}
		i++
	}

	downsampleFunction := "none"
	if hyphenIndex := strings.Index(respI.Query.Downsample, "-"); hyphenIndex >= 0 {
		downsampleFunction = respI.Query.Downsample[hyphenIndex+1:]
	}
	var aggregate storepb.Aggr
	if v, exists := downsampleToAggregate[downsampleFunction]; exists {
		aggregate = v
	} else {
		aggregate = storepb.Aggr_RAW
	}

	// Turn datapoints into chunks (Prometheus's tsdb encoding)
	dps := respI.GetDataPoints()
	chunks := []storepb.AggrChunk{}
	for i := 0; i < len(dps); {
		c := chunkenc.NewXORChunk()
		a, err := c.Appender()
		if err != nil {
			return nil, 0, err
		}
		var minTime int64
		// Maximum 120 datapoints in a chunk -- this is a Thanos recommendation, see
		// https://app.slack.com/client/T08PSQ7BQ/CL25937SP/thread/CL25937SP-1572162942.034700
		// (on https://slack.cncf.io).
		for ; i < len(dps) && (minTime == 0 || i%120 != 0); i++ {
			dp := dps[i]
			if minTime == 0 {
				minTime = int64(dp.Timestamp)
			}
			a.Append(int64(dp.Timestamp), dp.Value.(float64))
		}
		chunk := &storepb.Chunk{Type: storepb.Chunk_XOR, Data: c.Bytes()}
		aggrChunk := storepb.AggrChunk{
			MinTime: minTime,
			MaxTime: int64(dps[i-1].Timestamp),
		}
		switch aggregate {
		case storepb.Aggr_COUNT:
			aggrChunk.Count = chunk
		case storepb.Aggr_SUM:
			aggrChunk.Sum = chunk
		case storepb.Aggr_MIN:
			aggrChunk.Min = chunk
		case storepb.Aggr_MAX:
			aggrChunk.Max = chunk
		case storepb.Aggr_COUNTER:
			aggrChunk.Counter = chunk
		case storepb.Aggr_RAW:
			fallthrough
		default:
			aggrChunk.Raw = chunk
		}
		chunks = append(chunks, aggrChunk)
	}
	return storepb.NewSeriesResponse(&storepb.Series{
		Labels: seriesLabels,
		Chunks: chunks,
	}), len(dps), nil
}

func convertPromQLMatcherToFilter(matcher storepb.LabelMatcher) (opentsdb.Filter, error) {
	f := opentsdb.Filter{
		GroupBy: true,
		Tagk:    matcher.Name,
	}
	switch matcher.Type {
	case storepb.LabelMatcher_EQ:
		if !strings.Contains(matcher.Value, "|") {
			f.Type = "literal_or"
			f.FilterExp = matcher.Value
		} else {
			// "|" is meaningful in OpenTSDB matches and there's no way to escape.
			// It's unlikely to be used in queries, but to avoid odd behaviour we turn
			// this into a regexp.
			f.Type = "regexp"
			f.FilterExp = "^(?:" + regexp.QuoteMeta(matcher.Value) + ")$"
		}
	case storepb.LabelMatcher_NEQ:
		f.Type = "not_literal_or"
		f.FilterExp = matcher.Value
	case storepb.LabelMatcher_NRE:
		rx, err := regexputil.Parse(matcher.Value)
		if err != nil {
			return opentsdb.Filter{}, err
		}
		items, ok := rx.List()
		if !ok {
			return opentsdb.Filter{}, errors.New("NRE (!~) is not supported for general regexps, only fixed alternatives like '(a|b)'")
		}
		f.Type = "not_literal_or"
		f.FilterExp = strings.Join(items, "|")
	case storepb.LabelMatcher_RE:
		rx, err := regexputil.Parse(matcher.Value)
		if err != nil {
			return opentsdb.Filter{}, err
		}
		if items, ok := rx.List(); ok {
			f.Type = "literal_or"
			f.FilterExp = strings.Join(items, "|")
		} else if wildcard, ok := rx.Wildcard(); ok {
			f.Type = "wildcard"
			f.FilterExp = wildcard
		} else {
			f.Type = "regexp"
			f.FilterExp = "^(?:" + matcher.Value + ")$"
		}
	}
	return f, nil
}

func dumpPromQL(req *storepb.SeriesRequest) string {
	b := strings.Builder{}
	for i, m := range req.Matchers {
		if i != 0 {
			b.WriteRune(',')
		}
		t := "="
		switch m.Type {
		case storepb.LabelMatcher_NEQ:
			t = "!="
		case storepb.LabelMatcher_RE:
			t = "=~"
		case storepb.LabelMatcher_NRE:
			t = "!~"
		}
		fmt.Fprintf(&b, "%s%s%q", m.Name, t, m.Value)
	}
	return fmt.Sprintf("{%v}", &b)
}
