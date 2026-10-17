FROM golang:1.27.2-bookworm
WORKDIR /workspace
ENV GOTOOLCHAIN=local
RUN mkdir /tmp/air-build && cd /tmp/air-build \
 && go mod init rj.dev/air-build \
 && go get github.com/air-verse/air@v1.67.4 golang.org/x/text@v0.41.0 \
 && go build -trimpath -o /go/bin/air github.com/air-verse/air \
 && rm -rf /tmp/air-build
COPY cmd/healthcheck/main.go /tmp/healthcheck.go
RUN go build -trimpath -o /usr/local/bin/rj-healthcheck /tmp/healthcheck.go
EXPOSE 8080
CMD ["air", "-c", ".air.toml"]
