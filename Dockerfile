# Build Geth in a stock Go builder container
FROM golang:1.10-alpine as construction

RUN apk add --no-cache make gcc musl-dev linux-headers

ADD . /truechain-engineering-code
RUN cd /truechain-engineering-code && make getrue

# Pull Geth into a second stage deploy alpine container
FROM alpine:latest

RUN apk add --no-cache ca-certificates
COPY --from=construction /truechain-engineering-code/build/bin/getrue /usr/local/bin/
CMD ["getrue"]

EXPOSE 8545 8545 30303 30303/udp 10080 10080/tcp
ENTRYPOINT ["getrue"]
