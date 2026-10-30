FROM golang:1.27.3-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /api ./cmd/api

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && addgroup -S documents && adduser -S -G documents documents
WORKDIR /app
COPY --from=build /api /app/api
USER documents
EXPOSE 8085
ENTRYPOINT ["/app/api"]
CMD ["serve"]
