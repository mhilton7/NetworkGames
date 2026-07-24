#!/bin/bash
set -euo pipefail

command=${1:?init-ca, issue-server, or issue-client}
directory=${2:?certificate directory}
name=${3:-}
mkdir -p "$directory"
umask 077
case "$command" in
  init-ca)
    test ! -e "$directory/ca.key"
    openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 \
      -out "$directory/ca.key"
    openssl req -x509 -new -sha256 -days 3650 -key "$directory/ca.key" \
      -subj "/CN=NetworkGames private CA" -out "$directory/ca.crt"
    ;;
  issue-server)
    test -n "$name"
    openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 \
      -out "$directory/server.key.new"
    openssl req -new -key "$directory/server.key.new" -subj "/CN=$name" \
      -out "$directory/server.csr"
    printf 'subjectAltName=DNS:%s\nextendedKeyUsage=serverAuth\n' "$name" \
      > "$directory/server.ext"
    openssl x509 -req -sha256 -days 397 -in "$directory/server.csr" \
      -CA "$directory/ca.crt" -CAkey "$directory/ca.key" -CAcreateserial \
      -extfile "$directory/server.ext" -out "$directory/server.crt.new"
    mv "$directory/server.key.new" "$directory/server.key"
    mv "$directory/server.crt.new" "$directory/server.crt"
    rm -f "$directory/server.csr" "$directory/server.ext"
    ;;
  issue-client)
    test -n "$name"
    case "$name" in *[!A-Za-z0-9._-]*) exit 64;; esac
    openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 \
      -out "$directory/$name.key"
    openssl req -new -key "$directory/$name.key" -subj "/CN=$name" \
      -out "$directory/$name.csr"
    printf 'extendedKeyUsage=clientAuth\n' > "$directory/$name.ext"
    openssl x509 -req -sha256 -days 397 -in "$directory/$name.csr" \
      -CA "$directory/ca.crt" -CAkey "$directory/ca.key" -CAcreateserial \
      -extfile "$directory/$name.ext" -out "$directory/$name.crt"
    rm -f "$directory/$name.csr" "$directory/$name.ext"
    ;;
  *) exit 64 ;;
esac
