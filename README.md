# Geras
[![CircleCI](https://circleci.com/gh/G-Research/geras/tree/master.svg?style=svg)](https://circleci.com/gh/G-Research/geras/tree/master)

The goal of this project is to make it possible to run PromQL queries on OpenTSDB.

We have achieved this by providing an implementation of the [Thanos](https://github.com/improbable-eng/thanos) StoreAPI. 

Since Thanos's StoreAPI is designed for unified data access and is not too Prometheus specific, Geras is able to provide an implementation which proxies onto the OpenTSDB HTTP API, giving the ability to query OpenTSDB using PromQL, and even enabling unified queries over Prometheus and OpenTSDB.

## Build

```
go get github.com/G-Research/geras/cmd/geras

```

After the build you will have a self-contained binary (`geras`). It writes logs to `stdout`.

## Usage

```
  -grpc-listen string
        Service will expose the Store API on this address (default "localhost:19000")
  -label value
        Label to expose on the Store API, of the form '<key>=<value>'. May be repeated.
  -log.format string
        Log format. One of [logfmt, json] (default "logfmt")
  -log.level string
        Log filtering level. One of [debug, info, warn, error] (default "error")
  -metrics-refresh-interval duration
        Time between metric name refreshes. Use negative duration to disable refreshes. (default 15m0s)
  -opentsdb-address string
        http[s]://<host>:<port>
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
```

When specifying multiple labels, you will need to repeat the argument name, e.g:

```
./geras -label label1=value1 -label label2=value2
``` 

## Limitations

* PromQL supports queries without `__name__`. This is not allowed in geras and it will raise an error.
* Geras periodically loads metric names from OpenTSDB and keeps them in memory to support queries like `{__name__=~""}`.
* Thanos' primary timeseries backend is Prometheus, which doesn't support `.` in metric names. However OpenTSDB metrics generally use `.` as a seperator within names. In order to query names containing a `.` you will need to either:
  * Replace all `.` with `:` in your query
    
    OR
  * Use the magic `__name__` label to specify the metric name, e.g. `{__name__="cpu.percent"}`
