# build stage
FROM golang:alpine AS build-env
ADD . /src
WORKDIR /src
RUN apk add git
ARG GRPC_HEALTH_PROBE_VERSION=v0.3.0
RUN go get github.com/grpc-ecosystem/grpc-health-probe@${GRPC_HEALTH_PROBE_VERSION}

# Normally built on CircleCI, you can get a dev image with version info with:
# docker build --build-arg GIT_REVISION="$(git rev-parse HEAD)" \
#   --build-arg GIT_BRANCH="$(git rev-parse --abbrev-ref HEAD)" .
ARG GERAS_VERSION="development"
ARG GIT_REVISION="unknown"
ARG GIT_BRANCH="unknown"

ENV GO111MODULE=on
RUN go install -mod=vendor -ldflags '-extldflags "-static" \
  -X github.com/prometheus/common/version.Version='"${GERAS_VERSION}"' \
  -X github.com/prometheus/common/version.Revision='"${GIT_REVISION}"' \
  -X github.com/prometheus/common/version.Branch='"${GIT_BRANCH}"' \
  -X github.com/prometheus/common/version.BuildDate='"$(date +%Y%m%d-%H:%M:%S)" \
  ./cmd/geras

# final stage
FROM alpine
WORKDIR /bin
COPY --from=build-env /go/bin/geras .
COPY --from=build-env /go/bin/grpc-health-probe .
USER 1000
ENTRYPOINT ["./geras"]
