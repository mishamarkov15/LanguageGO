FROM golang:1.26-alpine AS builder
WORKDIR /src/gateway
COPY gateway/go.mod gateway/go.sum ./
COPY ledger/go.mod ledger/go.sum /src/ledger/
RUN go mod download
COPY ledger/ /src/ledger/
COPY gateway/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gateway .

FROM alpine:3.23
WORKDIR /app
COPY --from=builder /out/gateway ./gateway
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/gateway"]
