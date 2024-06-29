FROM golang:1.22.3

WORKDIR /app

COPY . ./
RUN go mod download

WORKDIR /app/cmd

RUN CGO_ENABLED=0 GOOS=linux go build -o /kaas-api main.go

EXPOSE 2024

CMD [ "/kaas-api" ]
