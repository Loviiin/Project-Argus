FROM golang:1.26-alpine AS builder

WORKDIR /workspaces/Project-Argus
RUN apk add --no-cache git
COPY . .

WORKDIR /workspaces/Project-Argus/services/parser
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin cmd/main.go

FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/bin .
ENV CONFIG_PATH=/workspaces/Project-Argus/config/config.yaml
CMD ["./bin"]
