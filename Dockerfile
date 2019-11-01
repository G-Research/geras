# build stage
FROM golang:1-alpine AS build-env
ADD . /src
WORKDIR /src
RUN apk add git
ARG GRPC_HEALTH_PROBE_VERSION=v0.3.0
RUN go get github.com/grpc-ecosystem/grpc-health-probe@${GRPC_HEALTH_PROBE_VERSION}

ARG GERAS_VERSION="development"
ARG BUILD_USER="docker"
ARG GIT_REVISION="unknown"
ARG GIT_BRANCH="unknown"

RUN if [[ "${GIT_REVISION}" = "unknown" ]]; then \
  GIT_REVISION="$(git rev-parse HEAD)"; \
  GIT_BRANCH="$(git rev-parse --abbrev-ref HEAD)"; \
fi; \
go install -ldflags ' \
  -X github.com/prometheus/common/version.Version='"${GERAS_VERSION}"' \
  -X github.com/prometheus/common/version.Revision='"${GIT_REVISION}"' \
  -X github.com/prometheus/common/version.Branch='"${GIT_BRANCH}"' \
  -X github.com/prometheus/common/version.BuildUser='"${BUILD_USER}"' \
  -X github.com/prometheus/common/version.BuildDate='"$(date +%Y%m%d-%H:%M:%S)" \
  ./cmd/geras

# final stage
FROM alpine
WORKDIR /bin
COPY --from=build-env /go/bin/geras .
COPY --from=build-env /go/bin/grpc-health-probe .
USER 1000
ENTRYPOINT ["./geras"]
