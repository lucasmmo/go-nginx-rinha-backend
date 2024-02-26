FROM golang:1.21 as base

WORKDIR /app
COPY go.* ./
ADD main.go ./
RUN GO111MODULE=auto CGO_ENABLED=0 GOOS=linux go build main.go

FROM alpine

WORKDIR /app
COPY --from=base /app/main ./
EXPOSE 8080
ENV GIN_MODE release
CMD [ "./main" ]
