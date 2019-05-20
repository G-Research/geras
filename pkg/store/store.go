package store

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"

	opentsdb "github.com/bluebreezecf/opentsdb-goclient/client"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/improbable-eng/thanos/pkg/store/prompb"
	"github.com/improbable-eng/thanos/pkg/store/storepb"
	"github.com/prometheus/tsdb/chunkenc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// the original OpenTSDBClient has 24 methods, this interface contains only the
// subset of them which are used
// and its easier to write a mock for this one.
type MinimalOpenTSDBClient interface {
	Query(param opentsdb.QueryParam) (*opentsdb.QueryResponse, error)
	Suggest(param opentsdb.SuggestParam) (*opentsdb.SuggestResponse, error)
}

type OpenTSDBStore struct {
	logger                log.Logger
	openTSDBClient        MinimalOpenTSDBClient
	metricNames           []string
	metricsNamesLock      sync.RWMutex
	metricRefreshInterval time.Duration
}

func NewOpenTSDBStore(logger log.Logger, client MinimalOpenTSDBClient, interval time.Duration) *OpenTSDBStore {
	store := &OpenTSDBStore{
		logger:                log.With(logger, "component", "opentsdb"),
		openTSDBClient:        client,
		metricRefreshInterval: interval,
	}
	err := store.loadAllMetricNames()
	if err != nil {
		level.Info(store.logger).Log("err", err)
	}
	if store.metricRefreshInterval >= 0 {
		go func() {
			for {
				err := store.loadAllMetricNames()
				if err != nil {
					level.Info(store.logger).Log("err", err)
				} else {
					level.Debug(logger).Log("msg", "metric names have been refreshed")
				}
				time.Sleep(store.metricRefreshInterval)
			}
		}()
	}
	return store
}

func (store *OpenTSDBStore) Info(
	ctx context.Context,
	req *storepb.InfoRequest) (*storepb.InfoResponse, error) {
	res := storepb.InfoResponse{
		MinTime: 0,
		MaxTime: math.MaxInt64,
		Labels:  []storepb.Label{},
	}
	level.Debug(store.logger).Log("msg", "Info")
	return &res, nil
}

func (store *OpenTSDBStore) Series(
	req *storepb.SeriesRequest,
	server storepb.Store_SeriesServer) error {
	level.Debug(store.logger).Log("msg", "Series")
	query, err := store.composeOpenTSDBQuery(req)
	if err != nil {
		level.Error(store.logger).Log("err", err)
		return err
	}
	result, err := store.openTSDBClient.Query(query)
	if err != nil {
		level.Error(store.logger).Log("err", err)
		return err
	}
	for _, respI := range result.QueryRespCnts {
		res, err := convertOpenTSDBResultsToSeriesResponse(respI)
		if err != nil {
			level.Error(store.logger).Log("err", err)
			return err
		}
		if err := server.Send(res); err != nil {
			level.Error(store.logger).Log("err", err)
			return err
		}
	}
	return nil
}

func (store *OpenTSDBStore) LabelNames(
	ctx context.Context,
	req *storepb.LabelNamesRequest) (*storepb.LabelNamesResponse, error) {
	labelNames, err := store.openTSDBClient.Suggest(
		opentsdb.SuggestParam{
			Type:         "tagk",
			Q:            "",
			MaxResultNum: math.MaxInt32,
		})
	if err != nil {
		return nil, err
	}
	return &storepb.LabelNamesResponse{
		Names: labelNames.ResultInfo,
	}, nil
}

func (store *OpenTSDBStore) LabelValues(
	ctx context.Context,
	req *storepb.LabelValuesRequest) (*storepb.LabelValuesResponse, error) {
	level.Debug(store.logger).Log("msg", "LabelValues", "Label", req.Label)
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (store *OpenTSDBStore) loadAllMetricNames() error {
	resp, err := store.openTSDBClient.Suggest(opentsdb.SuggestParam{
		Type:         "metrics",
		Q:            "",            // no restriction
		MaxResultNum: math.MaxInt32, // all of the metric names
	})
	if err != nil {
		return err
	}
	store.metricsNamesLock.Lock()
	store.metricNames = resp.ResultInfo
	store.metricsNamesLock.Unlock()
	return nil
}

func (store *OpenTSDBStore) getMatchingMetricNames(matcher storepb.LabelMatcher) ([]string, error) {
	if matcher.Name != "__name__" {
		return nil, errors.New("getMatchingMetricNames must be called on __name__ matcher")
	}
	if matcher.Type == storepb.LabelMatcher_EQ {
		return []string{matcher.Value}, nil
	} else if matcher.Type == storepb.LabelMatcher_NEQ {
		// we can support this, but we should not.
		return nil, errors.New("NEQ is not supported for __name__ label")
	} else if matcher.Type == storepb.LabelMatcher_NRE {
		return nil, errors.New("NRE is not supported")
	} else if matcher.Type == storepb.LabelMatcher_RE {
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

func (store *OpenTSDBStore) composeOpenTSDBQuery(req *storepb.SeriesRequest) (opentsdb.QueryParam, error) {
	var tagFilters []opentsdb.Filter
	var metricNames []string
	var err error
	for _, matcher := range req.Matchers {
		if matcher.Name == "__name__" {
			metricNames, err = store.getMatchingMetricNames(matcher)
			if err != nil {
				level.Info(store.logger).Log("err", err)
				return opentsdb.QueryParam{}, err
			}
			continue
		}
		f, err := convertPromQLMatcherToFilter(matcher)
		if err != nil {
			level.Info(store.logger).Log("err", err)
			return opentsdb.QueryParam{}, err
		}
		tagFilters = append(tagFilters, f)
	}
	if len(metricNames) == 0 {
		// although promQL supports queries without metric names
		// we do not want to do it at the moment.
		err := errors.New("missing __name__")
		level.Info(store.logger).Log("err", err)
		return opentsdb.QueryParam{}, err
	}
	subQueries := make([]opentsdb.SubQuery, len(metricNames))
	for i, mn := range metricNames {
		mn = strings.Replace(mn, ":", ".", -1)
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
	return query, nil
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
