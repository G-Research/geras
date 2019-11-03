FROM golang:1-alpine
ADD . /faketsdb
WORKDIR /faketsdb
ENV GO111MODULE=on
RUN go install .
ENTRYPOINT ["/go/bin/faketsdb"]
