FROM golang:1.26-alpine AS builder

WORKDIR /workspaces/Project-Argus
RUN apk add --no-cache git

# Cache: copy all module files first for Go Workspace
COPY go.work go.work.sum ./
COPY pkg/go.mod pkg/go.sum ./pkg/
COPY services/discovery/go.mod services/discovery/go.sum ./services/discovery/
COPY services/parser/go.mod services/parser/go.sum ./services/parser/
COPY services/scraper/go.mod services/scraper/go.sum ./services/scraper/
RUN go mod download

# Copy source code
COPY pkg/ ./pkg/
COPY services/parser/ ./services/parser/

WORKDIR /workspaces/Project-Argus/services/parser
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin cmd/main.go

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/bin .
ENV CONFIG_PATH=/workspaces/Project-Argus/config/config.yaml
CMD ["./bin"]
