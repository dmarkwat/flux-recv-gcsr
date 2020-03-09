FROM golang:1.13.8
WORKDIR /build
ADD . .
RUN go test && GOOS=linux GOARCH=amd64 go build -ldflags="-w -s"

FROM gcr.io/distroless/base
COPY --from=0 /build/flux-recv-gcsr /flux-recv-gcsr
ENTRYPOINT ["/flux-recv-gcsr"]
