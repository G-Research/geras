# build stage
FROM golang:alpine AS build-env
ADD . /src
WORKDIR /src
ENV GOOS=linux
RUN go build -mod=vendor -ldflags '-extldflags "-static"' -o geras ./cmd/geras/main.go

# final stage
FROM alpine
WORKDIR /app
COPY --from=build-env /src/geras /app/
ENTRYPOINT ["./geras"]
