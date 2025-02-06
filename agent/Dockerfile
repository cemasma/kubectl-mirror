FROM golang:1.23.4-bookworm AS builder

WORKDIR /app

COPY ./ ./

RUN go mod download

RUN apt update
RUN apt install libpcap-dev --assume-yes

RUN go build -o ./main

FROM golang:1.23.4-bookworm AS release

WORKDIR /app

RUN apt update
RUN apt install libpcap-dev --assume-yes

COPY --from=builder /app/main ./main
EXPOSE 8080
ENTRYPOINT ["./main"]