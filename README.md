# Cassandra Updater

[![Build](https://github.com/MinaFoundation/cassandra-updater/actions/workflows/build.yml/badge.svg)](https://github.com/MinaFoundation/cassandra-updater/actions/workflows/build.yml)

Takes a range from submissions, and updates rows with some dummy data...

## Build
```
$ nix-shell
$ make
```


## Configuration

```
export AWS_KEYSPACE=bpu_integration_dev
export SSL_CERTFILE=/path/to/certfile.crt
export CASSANDRA_PORT=9142
export CASSANDRA_HOST=cassandra.us-west-2.amazonaws.com
export CASSANDRA_USERNAME=****
export CASSANDRA_PASSWORD=****
```
## Run

```
$ ./result/bin/cassandra-updater "2024-03-04 09:38:54.0+0000" "2024-03-04 09:45:55.0+0000"
```

## Docker
```
$ nix-shell
$ TAG=1.0 make docker
```
```
docker run --rm \
  -e AWS_KEYSPACE \
  -e SSL_CERTFILE=/etc/certfile.crt \
  -e CASSANDRA_PORT \
  -e CASSANDRA_HOST \
  -e CASSANDRA_USERNAME \
  -e CASSANDRA_PASSWORD \
  -v /home/piotr/code/mf/uptime-service-validation/uptime_service_validation/database/aws_keyspaces/cert/sf-class2-root.crt:/etc/certfile.crt \
  673156464838.dkr.ecr.us-west-2.amazonaws.com/cassandra-updater:1.0 \
  "2024-03-04 09:38:54.0+0000" "2024-03-04 09:45:55.0+0000"
```
