FROM golang:1.26-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/modguard \
    ./cmd/modguard

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/modguard /modguard
EXPOSE 8080
ENTRYPOINT ["/modguard"]
