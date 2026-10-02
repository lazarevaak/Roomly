FROM golang:1.27.1-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/server ./backend

FROM alpine:3.22

WORKDIR /app

COPY --from=builder /out/server ./server
COPY --from=builder /src/web ./web
COPY --from=builder /src/migrations ./migrations

USER 10001:10001

EXPOSE 8080

ENTRYPOINT ["./server"]
