FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/offers ./cmd/offers
FROM alpine:3.22.2
RUN adduser -D -H app
USER app
COPY --from=build /out/offers /offers
ENTRYPOINT ["/offers"]
