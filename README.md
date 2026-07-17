<p align="left"><img height="250" src=logo/svg/geras_Logo-Color.svg alt="Geras-logo"/></p>

[![CI - Docker](https://github.com/G-Research/geras/actions/workflows/docker.yaml/badge.svg?style=flat&branch=master&event=push)](https://github.com/G-Research/geras/actions/workflows/docker.yaml?query=branch%3Amaster+event%3Apush)
[![CI - Test](https://github.com/G-Research/geras/actions/workflows/test.yaml/badge.svg?style=flat&branch=master&event=push)](https://github.com/G-Research/geras/actions/workflows/test.yaml?query=branch%3Amaster+event%3Apush)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)


Geras provides a [Thanos](https://thanos.io) Store API for the [OpenTSDB](http://opentsdb.net) HTTP API.
This makes it possible to query OpenTSDB via PromQL, through Thanos.

Since Thanos's StoreAPI is designed for unified data access and is not too Prometheus specific,
Geras is able to provide an implementation which proxies onto the OpenTSDB HTTP API, giving the
ability to query OpenTSDB using PromQL, and even enabling unified queries (including joins) over
Prometheus and OpenTSDB.

## Build

```
go get github.com/G-Research/geras/cmd/geras
```

After the build you will have a self-contained binary (`geras`). It writes logs to `stdout`.

A Dockerfile is also provided (see [docker-compose.yaml](test/docker-compose.yaml) for an example of using it).

## Deployment

At a high level:

* Run Geras somewhere and point it to OpenTSDB: `-opentsdb-address opentsdb:4242`;
* Configure a Thanos query instance with `--store=geras:19000` (i.e. the gRPC listen address).

Geras additionally listens on a HTTP port for Prometheus `/metrics` queries and some debug details
(using x/net/trace, see for example `/debug/requests` and `/debug/events`.

## Usage

```
  -grpc-listen string
        Service will expose the Store API on this address (default "localhost:19000")
  -http-listen string
        Where to serve HTTP debugging endpoints (like /metrics) (default "localhost:19001")
  -trace-enabled
        Enable tracing of requests, which is shown at /debug/requests (default true)
  -trace-dumpbody
        Include TSDB request and response bodies in traces (can be expensive) (default false)
  -label value
        Label to expose on the Store API, of the form '<key>=<value>'. May be repeated.
  -log.format string
        Log format. One of [logfmt, json] (default "logfmt")
  -log.level string
        Log filtering level. One of [debug, info, warn, error] (default "error")
  -healthcheck-metric
        A metric to query as a readiness health check (default "tsd.rpc.recieved")
  -metrics-refresh-interval duration
        Time between metric name refreshes. Use negative duration to disable refreshes. (default 15m0s)
  -metrics-refresh-timeout
        Timeout for metric refreshes (default 2m0s)
  -metrics-suggestions
        Enable metric suggestions (can be expensive) (default true)
  -opentsdb-address string
        <host>:<port>
  -metrics-allowed-regexp regexp
        A regular expression specifying the allowed metrics. Default is `.*`,
        i.e. everything. A good value if your metric names all match OpenTSDB
        style of `service.metric.name` could be `^\w+\..*$`. Disallowed metrics
        are simply not queried and non error is returned -- the purpose is to
        not send traffic to OpenTSDB when the metric source is Prometheus.
  -metrics-blocked-regexp regexp
        A regular expression of metrics to block. Default is empty and means to
        not block anything. The expected use of this is to block problematic
        queries as a fast mitigation therefore an error is returned when a
        metric is blocked.
  -metrics-name-response-rewriting
        Rewrite '.' to a defined character and other bad characters to '_' in all responses (Prometheus
        remote_read won't accept these, while Thanos will) (default true)
  -period-character-replace
		Rewrite '.' to a defined charater that Prometheus will handle better. (default ':')
```

When specifying multiple labels, you will need to repeat the argument name, e.g:

```
./geras -label label1=value1 -label label2=value2
```

## Limitations

* PromQL supports queries without `__name__`. This is not possible in OpenTSDB and no results will be returned if the query doesn't match on a metric name.
* Geras periodically loads metric names from OpenTSDB and keeps them in memory to support queries like `{__name__=~"regexp"}`.
* Thanos' primary timeseries backend is Prometheus, which doesn't support unquoted dots in metric names. However OpenTSDB metrics generally use `.` as a seperator within names. In order to query names containing a `.` you will need to either:
  * Replace all `.` with another character (we like `:`).
  * Use the `__name__` label to specify the metric name, e.g. `{__name__="cpu.percent"}`
  * Also watch out for `-` (dashes) in your metric names

## Security

Please see our [security policy](https://github.com/G-Research/geras/blob/master/SECURITY.md) for details on reporting security vulnerabilities.
