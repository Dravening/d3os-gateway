FROM golang:1.20 as builder
#FROM registry.cn-hangzhou.aliyuncs.com/draven_yyz/golang:1.20 as builder
WORKDIR /app
COPY . .
RUN go env -w GOPROXY=https://goproxy.cn,direct; go mod download

RUN CGO_ENABLED=0  GOOS=linux GOARCH=amd64 go build -o d3os-gateway main.go

FROM alpine as runner
COPY --from=builder /app/d3os-gateway .
COPY --from=builder /app/kubeconfig .
ENTRYPOINT [ "./d3os-gateway" ]
CMD [ "ingress" ]