FROM scratch
COPY build/container-root/networkgames-host /networkgames-host
USER 568:568
EXPOSE 8443 10809
ENTRYPOINT ["/networkgames-host"]
