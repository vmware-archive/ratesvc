FROM quay.io/deis/go-dev:v1.5.0 as builder
COPY . /go/src/github.com/kubeapps/ratesvc
WORKDIR /go/src/github.com/kubeapps/ratesvc
RUN dep ensure
RUN CGO_ENABLED=0 go build -a -installsuffix cgo

FROM scratch
COPY --from=builder /go/src/github.com/kubeapps/ratesvc/ratesvc /ratesvc
EXPOSE 8080
CMD ["/ratesvc"]
