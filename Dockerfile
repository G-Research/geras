# build stage
FROM golang:alpine AS build-env
ADD . /src
WORKDIR /src
RUN apk add git
RUN go get github.com/grpc-ecosystem/grpc-health-probe@v0.3.0
ENV GO111MODULE=on
RUN go build -mod=vendor -ldflags '-extldflags "-static"' -o geras ./cmd/geras/main.go

# final stage
FROM alpine
WORKDIR /bin
COPY --from=build-env /src/geras /bin/
COPY --from=build-env /go/bin/grpc-health-probe /bin/
USER 1000
ENTRYPOINT ["./geras"]
