# Stage 1: Build the Go application
FROM golang:1.23-alpine AS builder

ENV APP_HOME=/mednet/q
WORKDIR $APP_HOME

# 1) Copy go.mod and go.sum first, download deps (caches nicely)
COPY go.mod go.sum ./
RUN go mod download

# 2) Now copy the rest of your code
COPY . .

# 3) Build your binary
RUN go build -o relay cmd/server/main.go

# Stage 2: Create the final minimal image
FROM alpine:latest
RUN apk update && apk add --no-cache curl

ENV APP_HOME=/mednet/q
WORKDIR $APP_HOME

# Copy binary + env
COPY --from=builder /mednet/q/relay .

EXPOSE 8080
EXPOSE 2525
CMD ["./relay"]
