# syntax=docker/dockerfile:1

FROM golang:1.24-alpine

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -o ./virtual-call-center

EXPOSE 5080/udp
EXPOSE 443/tcp

CMD ["/app/virtual-call-center"]