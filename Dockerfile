FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o proxy-pac ./cmd/proxy-pac

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/proxy-pac .
COPY config.yaml.example /etc/proxy-pac.yaml
# PAC file should be mounted or copied
EXPOSE 8080 1080 9090
CMD ["./proxy-pac", "-conf", "/etc/proxy-pac.yaml"]
