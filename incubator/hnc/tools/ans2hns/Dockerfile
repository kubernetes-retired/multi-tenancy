FROM golang:1.15
ENV CGO_ENABLED=0
WORKDIR /go/src/
COPY . .
RUN go build -v -o /usr/local/bin/function ./

FROM gcr.io/distroless/static:nonroot
COPY --from=0 /usr/local/bin/function /usr/local/bin/function
USER nonroot:nonroot
CMD ["function"]
