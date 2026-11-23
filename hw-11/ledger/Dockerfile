FROM golang:1.26-alpine AS builder
WORKDIR /src/ledger
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ledger ./cmd/ledger

FROM alpine:3.23
WORKDIR /app
COPY --from=builder /out/ledger ./ledger
USER 65532:65532
ENTRYPOINT ["/app/ledger"]
