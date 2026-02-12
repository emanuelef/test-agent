# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o agent ./cmd/agent

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /build/agent .

# Set environment variables with sensible defaults
ENV OLLAMA_HOST=http://127.0.0.1:11434
ENV OLLAMA_MODEL=llama3.1
ENV FORECAST_DAYS=15

# Run the agent
CMD ["./agent"]
