# Submission Updater

[![Build](https://github.com/MinaFoundation/cassandra-updater/actions/workflows/build.yml/badge.svg)](https://github.com/MinaFoundation/cassandra-updater/actions/workflows/build.yml)

This is a wrapper over the [Stateless verifier tool](https://github.com/MinaProtocol/mina/tree/develop/src/app/delegation_verify) that is responsible for communication with Cassandra database. It will select a range of submissions from Cassandra, feed `stateless_verifier` with it, collect results and update submissions with gathered data. In order to work as expected the program requires `DELEGATION_VERIFY_BIN_PATH` env variable to be set.

## Build
```
$ nix-shell
$ make
```


## Configuration

**1. Runtime Configuration**:

  - `DELEGATION_VERIFY_BIN_PATH` - path to [Stateless verifier tool](https://github.com/MinaProtocol/mina/tree/develop/src/app/delegation_verify) binary.

**2. AWS Keyspaces/Cassandra Configuration**:

  **Mandatory/common env vars:**
  - `AWS_KEYSPACE` - Your Keyspace name.
  - `SSL_CERTFILE` - The path to your SSL certificate.

  **Depending on way of connecting:**

  _Service level connection:_
  - `CASSANDRA_HOST` - Cassandra host (e.g. cassandra.us-west-2.amazonaws.com).
  - `CASSANDRA_PORT` - Cassandra port (e.g. 9142).
  - `CASSANDRA_USERNAME` - Cassandra service user.
  - `CASSANDRA_PASSWORD` - Cassandra service password.

  _AWS access key / web identity token:_
  - `AWS_REGION` - The AWS region (same as used for S3).
  - `AWS_WEB_IDENTITY_TOKEN_FILE` - AWS web identity token file.
  - `AWS_ROLE_SESSION_NAME` - AWS role session name.
  - `AWS_ROLE_ARN` - AWS role arn.
  - `AWS_ACCESS_KEY_ID` - Your AWS Access Key ID. No need to set if `AWS_WEB_IDENTITY_TOKEN_FILE`, `AWS_ROLE_SESSION_NAME` and `AWS_ROLE_ARN` are set.
  - `AWS_SECRET_ACCESS_KEY` - Your AWS Secret Access Key. No need to set if `AWS_WEB_IDENTITY_TOKEN_FILE`, `AWS_ROLE_SESSION_NAME` and `AWS_ROLE_ARN` are set.

**3. AWS S3 Configuration**:

  - `AWS_S3_BUCKET` - AWS S3 Bucket where blocks and submissions are stored.
  - `NETWORK_NAME` - Network name (in case block does not exist in Cassandra we attempt to download it from AWS S3 from `AWS_S3_BUCKET`\\`NETWORK_NAME`\blocks)
  - `AWS_REGION` - The AWS region where your S3 bucket is located. While this is automatically retrieved, it can also be explicitly set through environment variables or AWS configuration files.
  - `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` - Your AWS credentials. These are automatically retrieved from your environment or AWS configuration files but should be securely stored and accessible in your deployment environment.
 
## Run

```
$ ./result/bin/cassandra-updater "2024-03-04 09:38:54.0+0000" "2024-03-04 09:45:55.0+0000"
```

## Docker

We can build docker image containing both `submission-updater` and [Stateless verifier tool](https://github.com/MinaProtocol/mina/tree/develop/src/app/delegation_verify). For that we need to feed build with `DUNE_PROFILE` and `MINA_BRANCH` env variables. `DUNE_PROFILE` is the profile in which the tool will be built (typically `devnet`). `MINA_BRANCH` indicates which branch of [Mina](https://github.com/MinaProtocol/mina) repository we want to build the tool from.

The docker image already has `DELEGATION_VERIFY_BIN_PATH` and `SSL_CERTFILE` already set, so we only need to feed it with remaining config variables required for connecting to AWS Keyspaces / Cassandra.

**Build**:

```
$ nix-shell
$ TAG=1.0 \
  DUNE_PROFILE=devnet \
  MINA_BRANCH=delegation_verify_over_stdin_rc_base \
  make docker-delegation-verify
```

**Run**:

```
docker run --rm \
  -e AWS_KEYSPACE \
  -e AWS_REGION \
  -e AWS_ACCESS_KEY_ID \
  -e AWS_SECRET_ACCESS_KEY \
  673156464838.dkr.ecr.us-west-2.amazonaws.com/delegation-verify:1.0 \
  "2024-03-15 13:12:12.0+0000" "2024-03-15 13:12:13.0+0000"
```
