# build stage
FROM golang:alpine AS build-env
RUN apk --no-cache add build-base git bzr mercurial gcc
ADD . /src
WORKDIR /src
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=386
RUN go build -mod=vendor -ldflags '-extldflags "-static"' -o geras ./cmd/geras/main.go

# final stage
FROM alpine
WORKDIR /app
COPY --from=build-env /src/geras /app/
ENTRYPOINT ["./geras"]
