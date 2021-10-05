FROM go:1.17-alpine

WORKDIR /usr/src/app

COPY . .

RUN go build .

CMD [ "./temp-email" ]