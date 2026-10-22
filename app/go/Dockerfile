FROM golang:1.25 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/txn-api .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/txn-api /txn-api
EXPOSE 8000
ENTRYPOINT ["/txn-api"]
