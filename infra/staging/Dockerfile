FROM alpine:3.22
ARG VERSION
LABEL org.opencontainers.image.revision=$VERSION
RUN adduser -D -u 10001 pinelink
WORKDIR /app
COPY --chmod=755 pinelinkd /app/pinelinkd
COPY migrations /app/migrations
USER pinelink
ENV PINELINK_MIGRATIONS_DIR=/app/migrations
ENTRYPOINT ["/app/pinelinkd"]
