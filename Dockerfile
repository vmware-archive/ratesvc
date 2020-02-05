# syntax = docker/dockerfile:experimental

FROM golang:1.13 as builder
WORKDIR /go/src/github.com/kubeapps/ratesvc
COPY go.mod go.sum ./
COPY response response
COPY testutil testutil
COPY *.go ./
# With the trick below, Go's build cache is kept between builds.
# https://github.com/golang/go/issues/27719#issuecomment-514747274
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  CGO_ENABLED=0 go build -installsuffix cgo .
RUN curl https://s3.amazonaws.com/rds-downloads/rds-combined-ca-bundle.pem >> /etc/ssl/certs/ca-certificates.crt

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/src/github.com/kubeapps/ratesvc/ratesvc /ratesvc
EXPOSE 8080
CMD ["/ratesvc"]
