# docker-exec-ws

## Intro
Websocket server that serves the results of docker exec

## Usage
server
```
./app [-host :8080] [-docker_host 127.0.0.1:2375] [-cols 100] [-rows 28]
```

client
```
http://<ws_host>/?id=<container_id>
```
