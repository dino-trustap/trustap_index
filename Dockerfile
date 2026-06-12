# We use Alpine Linux to keep the size of the image small.
FROM alpine:3.22.2

WORKDIR /app

RUN \
    apk add ca-certificates

COPY \
    api \
    /app/

USER 1001

EXPOSE 80

ENTRYPOINT ["/app/api", ":80"]









