FROM scratch
ARG OCI_REVISION=unknown
ARG OCI_VERSION=0.1.0-rc.1
ARG OCI_CREATED=unknown
ARG OCI_SOURCE=https://github.com/mhilton7/WiiBridge
LABEL org.opencontainers.image.revision="${OCI_REVISION}" \
      org.opencontainers.image.version="${OCI_VERSION}" \
      org.opencontainers.image.created="${OCI_CREATED}" \
      org.opencontainers.image.source="${OCI_SOURCE}"
COPY build/container-root/wiibridge-host /wiibridge-host
USER 568:568
EXPOSE 8445 10809
ENTRYPOINT ["/wiibridge-host"]
