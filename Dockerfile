# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/server ./cmd/server
RUN mkdir -p /data && chown 65532:65532 /data

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /bin/server /server
COPY --from=build --chown=65532:65532 /data /data

WORKDIR /data

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/server"]
