# Geras
[![CircleCI](https://circleci.com/gh/G-Research/geras/tree/master.svg?style=svg)](https://circleci.com/gh/G-Research/geras/tree/master)

The goal of this project is to make it possible to run PromQL queries on OpenTSDB using the same interface.

This component is not part of Thanos project, but it is designed to work with Thanos. This decision has been
made here: https://github.com/improbable-eng/thanos/issues/768

## Solution

Since Thanos's StoreAPI is designed for unified data access and it's not too Prometheus specific, Geras can
implement it on top of OpenTSDB providing a unified view over Prometheus and OpenTSDB.

## Build

```
go get github.com/G-Research/geras/cmd/geras

```

After the build you will have a self-contained binary (`geras`). It writes logs to `stdout`.

## Usage

```
  -grpc-listen string
        service will expose the store api on this address (default "localhost:19000")
  -log.format string
        Log format. One of [logfmt, json] (default "logfmt")
  -log.level string
        Log filtering level. One of [debug, info, warn, error] (default "error")
  -metrics-refresh-interval duration
        Time between metric name refreshes. Use negative duration to disable refreshes. (default 15m0s)
  -opentsdb-address string
        host:port
```

## Limitations

* PromQL supports queries without `__name__`. This is not allowed in geras and it will raise an error.
* Geras loads the metric names from OpenTSDB and keeps them in memory for queries like `__name__=~""`.
* Have to use `:` instead of `.` in metric names (PromQL limitation).
