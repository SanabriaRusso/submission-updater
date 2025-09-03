#!/usr/bin/env bash

set -e

if [[ "$OUT" == "" ]]; then
  OUT="$PWD/result"
fi

case "$1" in
  test)
    cd src
    $GO test
    ;;
  docker-delegation-verify)
    if [[ "$TAG" == "" ]]; then
      echo "Specify TAG env variable."
      exit 1
    fi
    if [[ "$DUNE_PROFILE" == "" ]]; then
      echo "Specify DUNE_PROFILE env variable. (e.g. devnet)"
      exit 1
    fi
    if [[ "$MINA_BRANCH" == "" ]]; then
      echo "Specify MINA_BRANCH env variable. (The branch to build the delegation-verify binary from)."
      exit 1
    fi
    # set default image name for GitHub Container Registry if IMAGE_NAME is not set
    IMAGE_NAME=${IMAGE_NAME:-ghcr.io/sanabriarusso/submission-updater}
    docker build --build-arg "MINA_BRANCH=$MINA_BRANCH" --build-arg "DUNE_PROFILE=$DUNE_PROFILE" -f dockerfiles/Dockerfile-delegation-verify -t "$IMAGE_NAME:$TAG" .
    ;;
  docker-standalone)
    if [[ "$TAG" == "" ]]; then
      echo "Specify TAG env variable."
      exit 1
    fi
    # set default image name for GitHub Container Registry if IMAGE_NAME is not set
    IMAGE_NAME=${IMAGE_NAME:-ghcr.io/sanabriarusso/submission-updater}
    docker build -f dockerfiles/Dockerfile-standalone -t "$IMAGE_NAME:$TAG" .
    ;;
  "")
    cd src
    $GO build -o "$OUT/bin/submission-updater"
    ;;
  *)
    echo "unknown command $1"
    exit 2
    ;;
esac
