FROM golang:1.24.3

WORKDIR /app
COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .
RUN go build -o /app/main

EXPOSE 8081 60000 60001 60002 60003 60004
CMD ["/app/main"]