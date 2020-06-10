package store

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/thanos-io/thanos/pkg/store/storepb"

	opentsdb "github.com/G-Research/opentsdb-goclient/client"
)

func TestComposeOpenTSDBQuery(t *testing.T) {
	testCases := []struct {
		req                            storepb.SeriesRequest
		tsdbQ                          *opentsdb.QueryParam
		knownMetrics                   []string
		err                            error
		allowedMetrics, blockedMetrics *regexp.Regexp
		storeLabels []storepb.Label
	}{
		{
			knownMetrics: []string{"test.metric"},
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "key",
						Value: "value",
					},
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric",
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_SUM},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Metric:     "test.metric",
						Filters: []opentsdb.Filter{
							{
								Type:      "literal_or",
								Tagk:      "key",
								FilterExp: "value",
								GroupBy:   true,
							},
						},
					},
				},
			},
		},
		{
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric",
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_MAX},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Metric:     "test.metric",
						Filters:    []opentsdb.Filter{},
					},
				},
			},
		},
		{
			knownMetrics: []string{"test.metric2"},
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "host",
						Value: "x|y",
					},
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric2",
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_MIN},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Metric:     "test.metric2",
						Filters: []opentsdb.Filter{
							{
								Type:      "regexp",
								Tagk:      "host",
								FilterExp: `^(?:x\|y)$`,
								GroupBy:   true,
							},
						},
					},
				},
			},
		},
		{
			knownMetrics: []string{"test.metric2"},
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_RE,
						Name:  "host",
						Value: "foo.+",
					},
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric2",
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_MIN},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Metric:     "test.metric2",
						Filters: []opentsdb.Filter{
							{
								Type:      "regexp",
								Tagk:      "host",
								FilterExp: "^(?:foo.+)$",
								GroupBy:   true,
							},
						},
					},
				},
			},
		},
		{
			knownMetrics: []string{"test.metric2"},
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "host",
						Value: "*",
					},
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric2",
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_MIN},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Metric:     "test.metric2",
						Filters: []opentsdb.Filter{
							{
								Type:      "literal_or",
								Tagk:      "host",
								FilterExp: "*",
								GroupBy:   true,
							},
						},
					},
				},
			},
		},
		{
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric2",
					},
					{
						Type:  storepb.LabelMatcher_NRE,
						Name:  "host",
						Value: ".*",
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_MIN},
				PartialResponseDisabled: false,
			},
			err: errors.New("NRE (!~) is not supported for general regexps, only fixed alternatives like '(a|b)'"),
		},
		{
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_NRE,
						Name:  "__name__",
						Value: "test.metric2",
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_MIN},
				PartialResponseDisabled: false,
			},
			err: errors.New("NRE (!~) is not supported for __name__"),
		},
		{
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_RE,
						Name:  "host",
						Value: ".*",
					},
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric2",
					},
					{
						Type:  storepb.LabelMatcher_NEQ,
						Name:  "key",
						Value: "v",
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_MIN},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Metric:     "test.metric2",
						Filters: []opentsdb.Filter{
							{
								Type:      "wildcard",
								Tagk:      "host",
								FilterExp: "*",
								GroupBy:   true,
							},
							{
								Type:      "not_literal_or",
								Tagk:      "key",
								FilterExp: "v",
								GroupBy:   true,
							},
						},
					},
				},
			},
		},
		{
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_RE,
						Name:  "host",
						Value: "foo.*",
					},
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric2",
					},
					{
						Type:  storepb.LabelMatcher_NEQ,
						Name:  "key",
						Value: "v",
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_MIN},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Metric:     "test.metric2",
						Filters: []opentsdb.Filter{
							{
								Type:      "wildcard",
								Tagk:      "host",
								FilterExp: "foo*",
								GroupBy:   true,
							},
							{
								Type:      "not_literal_or",
								Tagk:      "key",
								FilterExp: "v",
								GroupBy:   true,
							},
						},
					},
				},
			},
		},
		{
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_NRE,
						Name:  "host",
						Value: "(aa|bb)",
					},
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric2",
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_MIN},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Metric:     "test.metric2",
						Filters: []opentsdb.Filter{
							{
								Type:      "not_literal_or",
								Tagk:      "host",
								FilterExp: "aa|bb",
								GroupBy:   true,
							},
						},
					},
				},
			},
		},
		{
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_RE,
						Name:  "host",
						Value: ".*",
					},
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test:metric2:sub:subsub",
					},
					{
						Type:  storepb.LabelMatcher_NEQ,
						Name:  "key",
						Value: "v",
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_MIN},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Metric:     "test.metric2.sub.subsub",
						Filters: []opentsdb.Filter{
							{
								Type:      "wildcard",
								Tagk:      "host",
								FilterExp: "*",
								GroupBy:   true,
							},
							{
								Type:      "not_literal_or",
								Tagk:      "key",
								FilterExp: "v",
								GroupBy:   true,
							},
						},
					},
				},
			},
		},
		{
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_RE,
						Name:  "host",
						Value: "(?:a|b|c)",
					},
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric",
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_MIN},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Metric:     "test.metric",
						Filters: []opentsdb.Filter{
							{
								Type:      "literal_or",
								Tagk:      "host",
								FilterExp: "a|b|c",
								GroupBy:   true,
							},
						},
					},
				},
			},
		},
		{
			knownMetrics: []string{"a", "b", "c"},
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_RE,
						Name:  "__name__",
						Value: ".*",
					},
					{
						Type:  storepb.LabelMatcher_NEQ,
						Name:  "key",
						Value: "v",
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_MIN},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Metric:     "a",
						Filters: []opentsdb.Filter{
							{
								Type:      "not_literal_or",
								Tagk:      "key",
								FilterExp: "v",
								GroupBy:   true,
							},
						},
					},
					{
						Aggregator: "none",
						Metric:     "b",
						Filters: []opentsdb.Filter{
							{
								Type:      "not_literal_or",
								Tagk:      "key",
								FilterExp: "v",
								GroupBy:   true,
							},
						},
					},
					{
						Aggregator: "none",
						Metric:     "c",
						Filters: []opentsdb.Filter{
							{
								Type:      "not_literal_or",
								Tagk:      "key",
								FilterExp: "v",
								GroupBy:   true,
							},
						},
					},
				},
			},
		},
		{
			knownMetrics:   []string{"test.metric", "other.metric"},
			allowedMetrics: regexp.MustCompile(`test\..*`),
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_RE,
						Name:  "__name__",
						Value: `(other|test)\.metric`,
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_SUM},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Metric:     "test.metric",
					},
				},
			},
		},
		{
			knownMetrics:   []string{},
			allowedMetrics: regexp.MustCompile(`test\..*`),
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: `prometheus_metric`,
					},
					{
						Type:  storepb.LabelMatcher_NRE,
						Name:  "test",
						Value: `x|y.*|[a-b]`,
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_SUM},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{},
		},
		{
			knownMetrics:   []string{"test.metric"},
			allowedMetrics: regexp.MustCompile(`^\w+\.`),
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "up",
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_SUM},
				PartialResponseDisabled: false,
			},
			// All metric names filtered out
			tsdbQ: &opentsdb.QueryParam{},
		},
		{
			knownMetrics:   []string{"bad.metric"},
			blockedMetrics: regexp.MustCompile(`bad`),
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "bad.metric",
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_SUM},
				PartialResponseDisabled: false,
			},
			err: errors.New(`Metric "bad.metric" is blocked on Geras`),
		},
		{
			knownMetrics:   []string{"bad.metric"},
			blockedMetrics: regexp.MustCompile(`bad\.`),
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "bad:metric",
					},
				},
				MaxResolutionWindow:     0,
				Aggregates:              []storepb.Aggr{storepb.Aggr_SUM},
				PartialResponseDisabled: false,
			},
			err: errors.New(`Metric "bad.metric" is blocked on Geras`),
		},
		{
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric",
					},
				},
				MaxResolutionWindow:     60000,
				Aggregates:              []storepb.Aggr{storepb.Aggr_RAW},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Metric:     "test.metric",
						Filters:    []opentsdb.Filter{},
					},
				},
			},
		},
		{
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric",
					},
				},
				MaxResolutionWindow:     60000,
				Aggregates:              []storepb.Aggr{storepb.Aggr_COUNT},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Downsample: "60s-count",
						Metric:     "test.metric",
						Filters:    []opentsdb.Filter{},
					},
				},
			},
		},
		{
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric",
					},
				},
				MaxResolutionWindow:     120000,
				Aggregates:              []storepb.Aggr{storepb.Aggr_MAX},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Downsample: "120s-max",
						Metric:     "test.metric",
						Filters:    []opentsdb.Filter{},
					},
				},
			},
		},
		{
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric",
					},
				},
				MaxResolutionWindow:     3600000,
				Aggregates:              []storepb.Aggr{storepb.Aggr_MIN},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Downsample: "3600s-min",
						Metric:     "test.metric",
						Filters:    []opentsdb.Filter{},
					},
				},
			},
		},
		{
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric",
					},
				},
				MaxResolutionWindow:     15000,
				Aggregates:              []storepb.Aggr{storepb.Aggr_SUM},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Downsample: "15s-sum",
						Metric:     "test.metric",
						Filters:    []opentsdb.Filter{},
					},
				},
			},
		},
		{
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric",
					},
				},
				MaxResolutionWindow:     60000,
				Aggregates:              []storepb.Aggr{storepb.Aggr_COUNTER},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Downsample: "60s-avg",
						Metric:     "test.metric",
						Filters:    []opentsdb.Filter{},
					},
				},
			},
		},
		{
			req: storepb.SeriesRequest{
				MinTime: 0,
				MaxTime: 100,
				Matchers: []storepb.LabelMatcher{
					{
						Type:  storepb.LabelMatcher_EQ,
						Name:  "__name__",
						Value: "test.metric",
					},
				},
				MaxResolutionWindow:     60000,
				Aggregates:              []storepb.Aggr{storepb.Aggr_COUNT, storepb.Aggr_SUM, storepb.Aggr_MIN, storepb.Aggr_MAX, storepb.Aggr_COUNTER},
				PartialResponseDisabled: false,
			},
			tsdbQ: &opentsdb.QueryParam{
				Start: 0,
				End:   100,
				Queries: []opentsdb.SubQuery{
					{
						Aggregator: "none",
						Downsample: "60s-count",
						Metric:     "test.metric",
						Filters:    []opentsdb.Filter{},
					},
					{
						Aggregator: "none",
						Downsample: "60s-sum",
						Metric:     "test.metric",
						Filters:    []opentsdb.Filter{},
					},
					{
						Aggregator: "none",
						Downsample: "60s-min",
						Metric:     "test.metric",
						Filters:    []opentsdb.Filter{},
					},
					{
						Aggregator: "none",
						Downsample: "60s-max",
						Metric:     "test.metric",
						Filters:    []opentsdb.Filter{},
					},
					{
						Aggregator: "none",
						Downsample: "60s-avg",
						Metric:     "test.metric",
						Filters:    []opentsdb.Filter{},
					},
				},
			},
		},
	}

	for i, test := range testCases {
		allowedMetrics := regexp.MustCompile(".*")
		if test.allowedMetrics != nil {
			allowedMetrics = test.allowedMetrics
		}
		store := NewOpenTSDBStore(
			log.NewJSONLogger(os.Stdout), nil, nil, time.Duration(0), 1*time.Minute, test.storeLabels, allowedMetrics, test.blockedMetrics, false, false, "foo")
		store.metricNames = test.knownMetrics
		p, _, err := store.composeOpenTSDBQuery(&test.req)
		if test.err != nil {
			if test.err.Error() != err.Error() {
				t.Errorf("%d: not expected error, got %v, want %v", i, err, test.err)
			}
			continue
		}
		if err != nil {
			t.Error(err)
		}
		if len(p.Queries) != len(test.tsdbQ.Queries) {
			t.Errorf("%d: expected %d queries, got %d (%v, %v)\ninput: %#v", i, len(test.tsdbQ.Queries), len(p.Queries), p.Queries, test.tsdbQ.Queries, test.req)
		}
		if len(test.tsdbQ.Queries) == 0 {
			continue
		}
		// test the requested ranges
		if test.tsdbQ.Start.(int) != int(p.Start.(int64)) ||
			test.tsdbQ.End.(int) != int(p.End.(int64)) {
			t.Errorf("%d: requested range is not equal to sent range (%d - %d) != (%d - %d)",
				i, p.Start, p.End, test.tsdbQ.Start, test.tsdbQ.End)
		}
		if len(p.Queries) != len(test.tsdbQ.Queries) {
			t.Errorf("%d: number of subqueries does not match", i)
		}
		for _, referenceQ := range test.tsdbQ.Queries {
			match := false
			for _, subQ := range p.Queries {
				// check metric name
				if referenceQ.Metric != subQ.Metric {
					continue
				}
				match = true
				// test aggregator
				if subQ.Aggregator != referenceQ.Aggregator {
					t.Errorf("\taggregator does not match")
				}
				// test filters
				if len(referenceQ.Filters) != len(subQ.Filters) {
					t.Errorf("\tfilter length does not match")
				}
				filters := map[string]opentsdb.Filter{}
				for _, f := range subQ.Filters {
					filters[f.Tagk] = f
				}
				for _, want := range referenceQ.Filters {
					got, ok := filters[want.Tagk]
					if !ok {
						t.Errorf("%d: filter does not exist", i)
						continue
					}
					if got.Type != want.Type {
						t.Errorf("got %v, want %v", got.Type, want.Type)
					}
					if got.FilterExp != want.FilterExp {
						t.Errorf("got %v, want %v", got.FilterExp, want.FilterExp)
					}
				}
			}
			if !match {
				t.Errorf("%d: there is no matching subquery for %v", i, referenceQ)
			}
		}
	}
}

func newDps(in map[string]interface{}) (out opentsdb.DataPoints) {
	enc, err := json.Marshal(in)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(enc, &out)
	if err != nil {
		panic(err)
	}
	return
}

func TestConvertOpenTSDBResultsToSeriesResponse(t *testing.T) {
	testCases := []struct {
		input              opentsdb.QueryRespItem
		expectedOutput     *storepb.SeriesResponse
		expectedChunkTypes []storepb.Aggr
	}{
		{
			input: opentsdb.QueryRespItem{
				Metric: "metric",
				Tags:   map[string]string{},
				Dps:    newDps(map[string]interface{}{}),
			},
			expectedOutput: storepb.NewSeriesResponse(&storepb.Series{
				Labels: []storepb.Label{{Name: "__name__", Value: "metric"}},
				Chunks: []storepb.AggrChunk{},
			}),
			expectedChunkTypes: []storepb.Aggr{},
		},
		{
			input: opentsdb.QueryRespItem{
				Metric: "metric",
				Tags:   map[string]string{"a": "b"},
				Dps: newDps(map[string]interface{}{
					"1": 1.0,
					"2": 1.5,
					"3": 2.0,
				}),
			},
			expectedOutput: storepb.NewSeriesResponse(&storepb.Series{
				Labels: []storepb.Label{{Name: "__name__", Value: "metric"}, {Name: "a", Value: "b"}},
				Chunks: []storepb.AggrChunk{{MinTime: 1, MaxTime: 3}},
			}),
			expectedChunkTypes: []storepb.Aggr{storepb.Aggr_RAW},
		},
		{
			input: opentsdb.QueryRespItem{
				Metric: "metric2",
				Tags:   map[string]string{"a": "b", "host": "test"},
				Dps: newDps(map[string]interface{}{
					"10": 1.0,
					"12": 1.5,
					"13": 2.0,
				}),
			},
			expectedOutput: storepb.NewSeriesResponse(&storepb.Series{
				Labels: []storepb.Label{
					{Name: "__name__", Value: "metric2"},
					{Name: "a", Value: "b"},
					{Name: "host", Value: "test"}},
				Chunks: []storepb.AggrChunk{{MinTime: 10, MaxTime: 13}},
			}),
			expectedChunkTypes: []storepb.Aggr{storepb.Aggr_RAW},
		},
		{
			input: opentsdb.QueryRespItem{
				Metric: "metric1",
				Tags:   map[string]string{},
				Dps: newDps(map[string]interface{}{
					"10": 1.0,
					"12": 1.5,
					"13": 2.0,
				}),
				Query: opentsdb.SubQuery{
					Aggregator: "none",
					Metric:     "metric",
					Rate:       false,
				},
			},
			expectedOutput: storepb.NewSeriesResponse(&storepb.Series{
				Labels: []storepb.Label{{Name: "__name__", Value: "metric1"}},
				Chunks: []storepb.AggrChunk{{MinTime: 10, MaxTime: 13}},
			}),
			expectedChunkTypes: []storepb.Aggr{storepb.Aggr_RAW},
		},
		{
			input: opentsdb.QueryRespItem{
				Metric: "metric1",
				Tags:   map[string]string{},
				Dps: newDps(map[string]interface{}{
					"10": 1.0,
					"12": 1.5,
					"13": 2.0,
				}),
				Query: opentsdb.SubQuery{
					Aggregator: "none",
					Metric:     "metric",
					Rate:       false,
					Downsample: "60s-count",
				},
			},
			expectedOutput: storepb.NewSeriesResponse(&storepb.Series{
				Labels: []storepb.Label{{Name: "__name__", Value: "metric1"}},
				Chunks: []storepb.AggrChunk{{MinTime: 10, MaxTime: 13}},
			}),
			expectedChunkTypes: []storepb.Aggr{storepb.Aggr_COUNT},
		},
		{
			input: opentsdb.QueryRespItem{
				Metric: "metric1",
				Tags:   map[string]string{},
				Dps: newDps(map[string]interface{}{
					"10": 1.0,
					"12": 1.5,
					"13": 2.0,
				}),
				Query: opentsdb.SubQuery{
					Aggregator: "none",
					Metric:     "metric",
					Rate:       false,
					Downsample: "60s-sum",
				},
			},
			expectedOutput: storepb.NewSeriesResponse(&storepb.Series{
				Labels: []storepb.Label{{Name: "__name__", Value: "metric1"}},
				Chunks: []storepb.AggrChunk{{MinTime: 10, MaxTime: 13}},
			}),
			expectedChunkTypes: []storepb.Aggr{storepb.Aggr_SUM},
		},
		{
			input: opentsdb.QueryRespItem{
				Metric: "metric1",
				Tags:   map[string]string{},
				Dps: newDps(map[string]interface{}{
					"10": 1.0,
					"12": 1.5,
					"13": 2.0,
				}),
				Query: opentsdb.SubQuery{
					Aggregator: "none",
					Metric:     "metric",
					Rate:       false,
					Downsample: "60s-min",
				},
			},
			expectedOutput: storepb.NewSeriesResponse(&storepb.Series{
				Labels: []storepb.Label{{Name: "__name__", Value: "metric1"}},
				Chunks: []storepb.AggrChunk{{MinTime: 10, MaxTime: 13}},
			}),
			expectedChunkTypes: []storepb.Aggr{storepb.Aggr_MIN},
		},
		{
			input: opentsdb.QueryRespItem{
				Metric: "metric1",
				Tags:   map[string]string{},
				Dps: newDps(map[string]interface{}{
					"10": 1.0,
					"12": 1.5,
					"13": 2.0,
				}),
				Query: opentsdb.SubQuery{
					Aggregator: "none",
					Metric:     "metric",
					Rate:       false,
					Downsample: "60s-max",
				},
			},
			expectedOutput: storepb.NewSeriesResponse(&storepb.Series{
				Labels: []storepb.Label{{Name: "__name__", Value: "metric1"}},
				Chunks: []storepb.AggrChunk{{MinTime: 10, MaxTime: 13}},
			}),
			expectedChunkTypes: []storepb.Aggr{storepb.Aggr_MAX},
		},
		{
			input: opentsdb.QueryRespItem{
				Metric: "metric1",
				Tags:   map[string]string{},
				Dps: newDps(map[string]interface{}{
					"10": 1.0,
					"12": 1.5,
					"13": 2.0,
				}),
				Query: opentsdb.SubQuery{
					Aggregator: "none",
					Metric:     "metric",
					Rate:       false,
					Downsample: "60s-avg",
				},
			},
			expectedOutput: storepb.NewSeriesResponse(&storepb.Series{
				Labels: []storepb.Label{{Name: "__name__", Value: "metric1"}},
				Chunks: []storepb.AggrChunk{{MinTime: 10, MaxTime: 13}},
			}),
			expectedChunkTypes: []storepb.Aggr{storepb.Aggr_COUNTER},
		},
	}
	for _, test := range testCases {
		store := NewOpenTSDBStore(
			log.NewJSONLogger(os.Stdout), nil, nil, time.Duration(0), 1*time.Minute, []storepb.Label{}, nil, nil, false, false, "foo")
		converted, _, err := store.convertOpenTSDBResultsToSeriesResponse(&test.input)
		if err != nil {
			t.Errorf("unexpected error: %s", err.Error())
		}
		if len(converted.GetSeries().Labels) == len(test.expectedOutput.GetSeries().Labels) {
			// Labels must be alphabetically sorted
			for i, label := range converted.GetSeries().Labels {
				want := test.expectedOutput.GetSeries().Labels[i]
				if label.Name != want.Name || label.Value != want.Value {
					t.Errorf("%d: got %v, want %v", i, label, want)
				}
			}
		} else {
			t.Errorf("number of tags does not match")
		}
		if len(test.expectedOutput.GetSeries().Chunks) != len(converted.GetSeries().Chunks) {
			t.Error("number of chunks does not match")
		}
		for ci, chunk := range test.expectedOutput.GetSeries().Chunks {
			convertedChunk := converted.GetSeries().Chunks[ci]
			if chunk.MinTime != convertedChunk.MinTime {
				t.Errorf("chunk %d min time is not the expected: %d", ci, chunk.MinTime)
			}
			if chunk.MaxTime != convertedChunk.MaxTime {
				t.Errorf("chunk %d max time is not the expected: %d != %d ", ci, chunk.MaxTime, converted.GetSeries().Chunks[ci].MaxTime)
			}
			expectedChunkType := test.expectedChunkTypes[ci]
			switch expectedChunkType {
			case storepb.Aggr_RAW:
				if convertedChunk.Raw == nil {
					t.Errorf("chunk %d raw content not set", ci)
				}
				break
			case storepb.Aggr_COUNT:
				if convertedChunk.Count == nil {
					t.Errorf("chunk %d raw content not set", ci)
				}
				break
			case storepb.Aggr_SUM:
				if convertedChunk.Sum == nil {
					t.Errorf("chunk %d raw content not set", ci)
				}
				break
			case storepb.Aggr_MIN:
				if convertedChunk.Min == nil {
					t.Errorf("chunk %d raw content not set", ci)
				}
				break
			case storepb.Aggr_MAX:
				if convertedChunk.Max == nil {
					t.Errorf("chunk %d raw content not set", ci)
				}
				break
			case storepb.Aggr_COUNTER:
				if convertedChunk.Counter == nil {
					t.Errorf("chunk %d raw content not set", ci)
				}
				break
			default:
				t.Errorf("Unknown chunk type %d expected for chunk %d", expectedChunkType, ci)
			}
		}
	}
}

func TestGetMatchingMetricNames(t *testing.T) {
	testCases := []struct {
		input          storepb.LabelMatcher
		expectedOutput []string
	}{
		{
			input: storepb.LabelMatcher{
				Name:  "tagk",
				Type:  storepb.LabelMatcher_EQ,
				Value: "tagv",
			},
			expectedOutput: nil,
		},
		{
			input: storepb.LabelMatcher{
				Name:  "__name__",
				Type:  storepb.LabelMatcher_NEQ,
				Value: "value",
			},
			expectedOutput: nil,
		},
		{
			input: storepb.LabelMatcher{
				Name:  "__name__",
				Type:  storepb.LabelMatcher_NRE,
				Value: "value",
			},
			expectedOutput: nil,
		},
		{
			input: storepb.LabelMatcher{
				Name:  "__name__",
				Type:  storepb.LabelMatcher_EQ,
				Value: "metric.name",
			},
			expectedOutput: []string{
				"metric.name",
			},
		},
		{
			input: storepb.LabelMatcher{
				Name:  "__name__",
				Type:  storepb.LabelMatcher_RE,
				Value: "tsd\\..*",
			},
			expectedOutput: []string{
				"tsd.rpc.errors",
				"tsd.rpc.exceptions",
				"tsd.rpc.forbidden",
				"tsd.rpc.received",
				"tsd.rpc.unauthorized",
			},
		},
		{
			input: storepb.LabelMatcher{
				Name:  "__name__",
				Type:  storepb.LabelMatcher_RE,
				Value: "cpu\\.[a-z]?i.*",
			},
			expectedOutput: []string{
				"cpu.idle",
				"cpu.irq",
				"cpu.nice",
			},
		},
	}

	for _, test := range testCases {
		store := &OpenTSDBStore{
			metricNames: []string{
				"cpu.idle",
				"cpu.irq",
				"cpu.nice",
				"cpu.sys",
				"cpu.usr",
				"tsd.rpc.errors",
				"tsd.rpc.exceptions",
				"tsd.rpc.forbidden",
				"tsd.rpc.received",
				"tsd.rpc.unauthorized",
			},
		}
		output, err := store.getMatchingMetricNames(test.input)

		if test.expectedOutput != nil {
			if err != nil {
				t.Errorf("Unexpected error: %v", err.Error())
			}
			if output == nil {
				t.Error("Expected output but not got any")
			} else if len(output) != len(test.expectedOutput) {
				t.Error("Number of metrics does not match")
			} else {
				for i, expected := range test.expectedOutput {
					if output[i] != expected {
						t.Errorf("Metric %v doesn't match expected: %v", output[i], expected)
					}
				}
			}
		} else {
			if output != nil {
				t.Errorf("Unexpected output: %v", output)
			}
			if err == nil {
				t.Error("Expected error but not got one")
			}
		}
	}

}
