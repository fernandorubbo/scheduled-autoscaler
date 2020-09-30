FROM golang:1.14-alpine as builder
WORKDIR /go/src/custom-metric-extporter
COPY . .
RUN go build -o /export .

FROM alpine as release
COPY --from=builder /export /export
CMD [ "/export"]
