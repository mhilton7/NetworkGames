FROM scratch
COPY build/container-root/wiibridge-host /wiibridge-host
USER 568:568
EXPOSE 8445 10809
ENTRYPOINT ["/wiibridge-host"]
