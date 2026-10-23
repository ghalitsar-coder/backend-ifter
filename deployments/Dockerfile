FROM golang:1.26.10-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN mkdir -p /out/storage/imports

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=build --chown=nonroot:nonroot /out/api /api
COPY --from=build --chown=nonroot:nonroot /out/storage /app/storage
EXPOSE 8080
ENTRYPOINT ["/api"]
