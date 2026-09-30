#!/bin/bash
 
SERVER_PORT=8080
PROXY_PORT=7331
 
echo "Iniciando modo dev en http://localhost:$PROXY_PORT"
 
templ generate --watch \
  --proxy="http://localhost:$SERVER_PORT" \
  --proxyport=$PROXY_PORT \
  --open-browser=true \
  --watch-pattern="(.+\.go$)|(.+\.templ$)|(.+\.css$)|(.+\.js$)" \
  --cmd="go run ."
 