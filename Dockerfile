# build stage
FROM golang:1.15-alpine AS build-env
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
LABEL org.opencontainers.image.description="Geras: Thanos Store API for OpenTSDB"
LABEL org.opencontainers.image.source="https://github.com/G-Research/geras"
LABEL org.opencontainers.image.vendor="G-Research"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.version=${GERAS_VERSION}
LABEL org.opencontainers.image.revision=${GIT_REVISION}

WORKDIR /bin
COPY --from=build-env /go/bin/geras .
COPY --from=build-env /go/bin/grpc-health-probe .
USER 1000
ENTRYPOINT ["./geras"]
