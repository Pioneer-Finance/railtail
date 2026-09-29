FROM golang:1.26.8 AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 go build -trimpath -o railtail -ldflags="-w -s" ./.

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /app/railtail /usr/local/bin/railtail

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/railtail"]
