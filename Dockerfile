FROM golang AS builder
WORKDIR /go/src/github.com/jamesqin-cn/docker-exec-wx/
ADD . .
RUN go get -v && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM alpine
MAINTAINER QinWuquan <jamesqin@vip.qq.com>
COPY --from=builder /go/src/github.com/jamesqin-cn/docker-exec-wx/app /bin/
EXPOSE 8080
ENTRYPOINT ["app"]
CMD ["-host :8080", "-docker_host 127.0.0.1:2375", "-cols 100", "-rows 28"]
