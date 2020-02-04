FROM quay.io/deis/go-dev:v1.5.0 as builder
COPY . /go/src/github.com/kubeapps/ratesvc
WORKDIR /go/src/github.com/kubeapps/ratesvc
RUN dep ensure
RUN CGO_ENABLED=0 go build -a -installsuffix cgo
RUN curl https://s3.amazonaws.com/rds-downloads/rds-combined-ca-bundle.pem >> /etc/ssl/certs/ca-certificates.crt

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/src/github.com/kubeapps/ratesvc/ratesvc /ratesvc
EXPOSE 8080
CMD ["/ratesvc"]
