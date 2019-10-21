// Binary faketsdb is a very simple fake instance of opentsdb, for integration
// testing of Geras.
//
// Fake timeseries will be queryable with the following format: "test.a.0".
// Where: 'a' is a letter between 'a' and 'z', and 0 is a number between 0 and
// --number-of-metrics - 1.
//
// Over the given time range the datapoints for each series will have an
// interval in seconds specified by the number in the query. (Except 0, which is
// 500ms).
//
// The letter 'a' has an additional tag (x=a).
// The letter 'b' has two additional tags: (x=b, y=y and y=z).
// The letter 'c' has the same two additional tags, and the y=z series has its values doubled.
// Other letters are untagged.
//
// Suggestions are only returned for the number of metrics names given, but
// queries can be made against e.g. test.b.0 which isn't returned as a
// suggestion.
//
// Curl test:
// curl -d '{"msResolution":true,"start":1546300800000,"end":1546300800000,"queries":[{"metric":"test.c.2","aggregator":"none"}]}' http://localhost:4242/api/query
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/G-Research/opentsdb-goclient/client"
)

// Fake *values* are invented relative to this time in unix epoch ms (i.e. datapoint value == datapoint_time - OUR_EPOCH).
// It happens to be 01/01/2019 00:00, but that doesn't really matter.
const OUR_EPOCH = 1546300800 * 1000

var fakeMetrics = []string{}

var (
	flagNumberMetrics = flag.Int("number-of-metrics", 1000000, "How many metrics to invent")
	flagListen        = flag.String("listen", ":4242", "Where HTTP service should listen")
)

func main() {
	flag.Parse()

	for i := 0; i < *flagNumberMetrics; i++ {
		n := 'a' + (i % 26)
		fakeMetrics = append(fakeMetrics, fmt.Sprintf("test.%c.%d", n, i))
	}

	http.HandleFunc("/api/suggest", jsonMarshal(bodyReader(suggest)))
	http.HandleFunc("/api/version", jsonMarshal(version))
	http.HandleFunc("/api/query", jsonMarshal(bodyReader(query)))
	http.HandleFunc("/api/query/last", jsonMarshal(bodyReader(queryLast)))
	log.Fatal(http.ListenAndServe(*flagListen, nil))
}

func jsonMarshal(f func(r *http.Request) interface{}) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL)
		w.Header().Set("Content-Type", "application/json")
		result := f(r)
		b, err := json.Marshal(result)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(b)
	}
}

func bodyReader(f func(r *http.Request, body []byte) interface{}) func(r *http.Request) interface{} {
	return func(r *http.Request) interface{} {
		if r.Method == "POST" {
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				log.Printf("bodyReader: %v", err)
				return nil
			}
			r.Body.Close()
			log.Printf("body: %s", string(body))
			return f(r, body)
		} else {
			return f(r, nil)
		}
	}
}

func version(r *http.Request) interface{} {
	return map[string]string{
		"version": "faketsdb go",
	}
}

func suggest(r *http.Request, body []byte) interface{} {
	return fakeMetrics
}

func query(r *http.Request, body []byte) interface{} {
	var param client.QueryParam
	err := json.Unmarshal(body, &param)
	if err != nil {
		return makeError(err.Error())
	}

	if !param.MsResolution {
		return makeError("msResolution required")
	}

	var results []client.QueryRespItem
	for _, q := range param.Queries {
		if q.Aggregator != "none" {
			return makeError("only none aggregator supported")
		}
		if len(q.Fiters) != 0 {
			return makeError("filter not yet supported")
		}

		r := getResults(int(param.Start.(float64)), int(param.End.(float64)), q)
		if r == nil {
			err := makeError(fmt.Sprintf("No such name for 'metrics': '%s'", q.Metric))
			err["error"].(map[string]interface{})["code"] = 400
			return err
		}
		results = append(results, r...)
	}
	return results
}

func getResults(start, end int, q client.SubQuery) []client.QueryRespItem {
	var a rune
	var n int
	_, err := fmt.Sscanf(q.Metric, "test.%c.%d", &a, &n)
	if err != nil || n >= *flagNumberMetrics {
		log.Printf("Dropped query for %q, %v", err)
		return nil
	}

	interval := n * 1000
	if interval == 0 {
		interval = 500
	}

	dp := make(map[string]interface{})
	// Invent datapoints, can have points before start
	for i := start - (start % interval); i <= end; i += interval {
		dp[fmt.Sprintf("%d", i)] = i - OUR_EPOCH
	}

	var results []client.QueryRespItem

	switch a {
	case 'a':
		// Extra tag
		results = append(results, client.QueryRespItem{
			Metric: q.Metric,
			Tags: map[string]string{
				"x": "a",
			},
			AggregatedTags: []string{},
			Dps:            dp,
		})
	case 'b':
		// Extra tags
		for _, tags := range []map[string]string{
			{"x": "a", "y": "y"},
			{"x": "a", "y": "z"},
		} {
			results = append(results, client.QueryRespItem{
				Metric:         q.Metric,
				Tags:           tags,
				AggregatedTags: []string{},
				Dps:            dp,
			})
		}
	case 'c':
		// Extra tags, different values (i.e. two different timeseries)
		dps := []map[string]interface{}{
			dp,
			mutateDatapoints(dp, func(x int) int { return x * 2 }),
		}
		for i, tags := range []map[string]string{
			{"x": "a", "y": "y"},
			{"x": "a", "y": "z"},
		} {
			results = append(results, client.QueryRespItem{
				Metric:         q.Metric,
				Tags:           tags,
				AggregatedTags: []string{},
				Dps:            dps[i],
			})
		}
	default:
		results = append(results, client.QueryRespItem{
			Metric:         q.Metric,
			AggregatedTags: []string{},
			Tags:           map[string]string{},
			Dps:            dp,
		})
	}

	log.Printf("Query for %q, returning %d results (%d datapoints)", q.Metric, len(results), len(dp))
	return results
}

func mutateDatapoints(datapoints map[string]interface{}, op func(int) int) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range datapoints {
		out[k] = op(v.(int))
	}
	return out
}

func queryLast(r *http.Request, body []byte) interface{} {
	return []string{}
}

func makeError(msg string) map[string]interface{} {
	log.Printf("error: %s", msg)
	return map[string]interface{}{
		"error": map[string]interface{}{
			"message": msg,
		},
	}
}
