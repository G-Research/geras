# build stage
FROM golang:alpine AS build-env
ADD . /src
WORKDIR /src
ENV GOOS=linux
RUN go build -mod=vendor -ldflags '-extldflags "-static"' -o geras ./cmd/geras/main.go

# final stage
FROM alpine
WORKDIR /bin
COPY --from=build-env /src/geras /bin/
ARG GRPC_HEALTH_PROBE_REPO_URL=https://github.com/grpc-ecosystem/grpc-health-probe
RUN GRPC_HEALTH_PROBE_VERSION=v0.3.0 && \
    wget -qO/bin/grpc_health_probe ${GRPC_HEALTH_PROBE_REPO_URL}/releases/download/${GRPC_HEALTH_PROBE_VERSION}/grpc_health_probe-linux-amd64 && \
    chmod +x /bin/grpc_health_probe
ENTRYPOINT ["./geras"]
