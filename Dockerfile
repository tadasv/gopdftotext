FROM alpine:latest

RUN apk add --no-cache poppler poppler-utils
WORKDIR /app

COPY gopdftotext /app/
CMD ["/app/gopdftotext"]
