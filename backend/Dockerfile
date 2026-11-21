FROM golang:1.27-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates \
	&& addgroup -S appgroup \
	&& adduser -S appuser -G appgroup

WORKDIR /app
COPY --from=builder /out/server /app/server
RUN chown -R appuser:appgroup /app

USER appuser
EXPOSE 8080
CMD ["./server"]